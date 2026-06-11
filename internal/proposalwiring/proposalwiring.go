package proposalwiring

import (
	"context"
	"fmt"
	"log"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/ingest"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// Options carries the per-run inputs that are not part of the tenant Config:
// the opportunity Store the service reads/writes, the base filesystem path for
// the proposal document store, and the three independent live/offline mode
// toggles. The zero value wires the fully offline service (stub Outline/Writer,
// deterministic Final Review, no ingestion).
type Options struct {
	// Store is the opportunity/proposal store the service persists through.
	Store store.Store

	// BasePath is the directory under which the proposal document store lives.
	BasePath string

	// LiveWriter drafts with the live Gemini writer (and live Gemini outline
	// planner) instead of the offline stubs. Requires cfg.GCP.ProjectID.
	LiveWriter bool

	// LiveReview runs the live Gemini compliance pass in Final Review on top of
	// the deterministic checks. Requires cfg.GCP.ProjectID.
	LiveReview bool

	// LiveIngest fetches and extracts the solicitation documents at draft time
	// (HTTP fetch -> GCS store -> Document AI / stdlib DOCX extraction). Requires
	// cfg.GCP.ProjectID, cfg.Ingest.GCSBucket, and cfg.Ingest.DocumentAIProcessor.
	LiveIngest bool
}

// New wires the REAL Zone 2 agents behind the shared gated lifecycle (epic
// #153) and returns a ready-to-serve *proposal.Service. It is the single place
// the Outline, Writer, Final Review, and (optional) ingestion agents are
// assembled, so cmd/dashboard and a future cmd/api build the service the same
// way.
//
// The three live toggles in opts switch each agent independently:
//   - Outline: cached Google Docs client always; live Gemini section planner
//     when opts.LiveWriter is set, deterministic planner otherwise.
//   - Writer: live Gemini drafting when opts.LiveWriter is set, stub otherwise.
//   - Final Review: live Gemini compliance pass when opts.LiveReview is set,
//     deterministic checks only otherwise.
//   - Ingestion: HTTP fetch + GCS + Document AI/DOCX extraction when
//     opts.LiveIngest is set, skipped otherwise.
//
// With every toggle off (the Options zero value) the service is fully offline:
// no GCP or network calls are made during construction or operation.
func New(ctx context.Context, cfg *config.Config, opts Options) (*proposal.Service, error) {
	docs, err := document.NewStore(opts.BasePath)
	if err != nil {
		return nil, fmt.Errorf("document store: %w", err)
	}
	docsClient, err := googledocs.NewClient(ctx, googledocs.Config{UseCached: true})
	if err != nil {
		return nil, fmt.Errorf("docs client: %w", err)
	}

	// One company profile feeds both the Hunter/Scorer and the Writer's grounding
	// (WS-A3). The Writer consumes the flattened scorer view, derived from the
	// single profile via scorer.FromProfile rather than a separate hand-maintained
	// scorer JSON.
	//
	// Resolve the profile at runtime (WS-A6): an existing deployment with a real
	// profile at the configured path behaves identically; a fresh image with no
	// real profile grounds the Writer on the generic example template plus an
	// explicit logged warning. ResolveProfile reports which source was used.
	// TODO(WS-C): the Store/GCS-backed, onboarding-written profile plugs in inside
	// profile.ResolveProfile (ahead of the local-file check), not here.
	scorerProfile := &scorer.CapabilityProfile{}
	if cfg.Profile.WriterPath != "" {
		companyProfile, profileSource, err := profile.ResolveProfile(cfg.Profile.WriterPath)
		if err != nil {
			return nil, fmt.Errorf("load profile: %w", err)
		}
		log.Printf("Company profile source: %s", profileSource)
		derived := scorer.FromProfile(companyProfile)
		scorerProfile = &derived
	}

	// The live agents share one Vertex AI region. The Gemini 3.x family —
	// gemini-3.1-pro-preview (drafting) and gemini-3.5-flash (outline structure) —
	// is served only from the global endpoint, so that is the default
	// (config.GCP.AgentRegion: GCP_REGION with a "global" default).
	region := cfg.GCP.AgentRegion

	ol := outline.New(docsClient) // deterministic section planner (offline default)
	w := writer.New()             // stub writer (offline default)
	if opts.LiveWriter {
		projectID := cfg.GCP.ProjectID
		if projectID == "" {
			return nil, fmt.Errorf("live agents require GCP_PROJECT_ID (use offline mode for credential-less UI dev)")
		}
		// Outline plans the section structure with gemini-3.5-flash; the Writer
		// persona "Thomas" drafts the prose with gemini-3.1-pro-preview while the
		// Claude/Opus 4.8 Vertex quota is pending (swap GEMINI_MODEL when it lands).
		planner, err := outline.NewGeminiSectionPlanner(ctx,
			projectID, region, cfg.GCP.OutlineModel)
		if err != nil {
			return nil, fmt.Errorf("gemini outline planner: %w", err)
		}
		ol = outline.NewWithPlanner(docsClient, planner)

		gen, err := writer.NewGeminiGenerator(ctx,
			projectID, region, cfg.GCP.WriterModel)
		if err != nil {
			return nil, fmt.Errorf("gemini generator: %w", err)
		}
		w = writer.NewWithGenerator(gen)
		log.Printf("Outline: LIVE gemini-3.5-flash planner; Technical Writer %q: LIVE gemini-3.1-pro-preview drafting (project %s)", "Thomas", projectID)
	} else {
		log.Printf("Outline + Technical Writer: OFFLINE stub mode — live Gemini agents are the default")
	}

	review := finalreview.New()
	if opts.LiveReview {
		projectID := cfg.GCP.ProjectID
		if projectID == "" {
			return nil, fmt.Errorf("live agents require GCP_PROJECT_ID (use offline mode for credential-less UI dev)")
		}
		// The reviewer model is configured INDEPENDENTLY of the Writer's GEMINI_MODEL.
		// The Final Review verifier bake-off found gemini-2.5-pro is the best Gemini
		// compliance verifier (83% defect recall) while gemini-3.1-pro is the worst
		// (17%) — so the gate must not silently inherit whatever the Writer is set to.
		// FINALREVIEW_MODEL lets it stay on the validated model (and swap to a Claude
		// model once Anthropic-on-Vertex quota lands), regardless of GEMINI_MODEL.
		// The reviewer uses config.GCP.Region (GCP_REGION with a "us-east4" default),
		// distinct from the agents' "global" AgentRegion above.
		reviewModel := cfg.GCP.FinalReviewModel
		checker, err := finalreview.NewGeminiComplianceChecker(ctx,
			projectID, cfg.GCP.Region, reviewModel)
		if err != nil {
			return nil, fmt.Errorf("gemini compliance checker: %w", err)
		}
		review = finalreview.NewWithComplianceChecker(checker)
		log.Printf("Final Review: LIVE Gemini compliance pass enabled (project %s, model %s)", projectID, reviewModel)
	} else {
		log.Printf("Final Review: OFFLINE deterministic checks only — live Gemini compliance is the default")
	}

	// Document ingestion is opt-in via the live-ingest option. A true nil interface
	// (not a typed-nil) is essential so proposal.Service's `Ingest == nil` check skips it.
	var ingestor proposal.Ingestor
	if opts.LiveIngest {
		projectID := cfg.GCP.ProjectID
		bucket := cfg.Ingest.GCSBucket
		processorID := cfg.Ingest.DocumentAIProcessor
		if projectID == "" || bucket == "" || processorID == "" {
			return nil, fmt.Errorf("live ingestion requires GCP_PROJECT_ID, GCS_SOLICITATIONS_BUCKET, and DOCUMENTAI_PROCESSOR_ID (set the live-ingest option)")
		}
		gcs, _, err := ingest.NewGCSStore(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("gcs store: %w", err)
		}
		docAI, _, err := ingest.NewDocumentAIExtractor(ctx, projectID, cfg.Ingest.DocumentAILocation, processorID, bucket)
		if err != nil {
			return nil, fmt.Errorf("document ai extractor: %w", err)
		}
		ingestor = ingest.New(ingest.NewHTTPFetcher(nil, 0), gcs, ingest.NewRoutingExtractor(docAI))
		log.Printf("Document ingestion: LIVE (bucket %s, Document AI processor %s)", bucket, processorID)
	} else {
		log.Printf("Document ingestion: off (enable the live-ingest option to fetch + extract solicitation documents)")
	}

	return proposal.NewService(&proposal.Deps{
		Opportunities: opts.Store,
		Documents:     docs,
		Outline:       ol,
		Writer:        w,
		Review:        review,
		Profile:       scorerProfile,
		Ingest:        ingestor,
	}), nil
}
