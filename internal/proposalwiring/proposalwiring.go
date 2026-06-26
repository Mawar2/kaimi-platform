package proposalwiring

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/contextdoc"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/fallback"
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

	// CustomerDriveOAuth carries the OAuth client credentials used to refresh a
	// connected customer's Drive token (WS-C2). When set AND the customer has
	// connected their Drive (a token is stored at BasePath) AND a target Drive id
	// has been chosen, the Outline agent writes Docs into the CUSTOMER's own Drive
	// via that token source instead of the cached/service-account client. When unset
	// or not connected, the Docs client is unchanged (cached today). The ClientID/
	// ClientSecret must match the ones the connect flow used so the refresh token
	// stays valid; RedirectURL is required by oauth2.Config but unused for refresh.
	CustomerDriveOAuth *drivetoken.OAuthClient
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
	// Build the Google Docs client. WS-C2: when the deployment has connected the
	// CUSTOMER's own Drive (a token is stored at BasePath and a target Drive id is
	// set) and OAuth client credentials are supplied to refresh it, the Outline
	// agent writes Docs into the customer's Drive via that token source. Otherwise
	// the Docs client is unchanged (cached today). resolveDocsClient logs which path
	// it took and falls back to cached on any not-connected / config-missing case so
	// a partial setup never breaks the proposal pipeline.
	docsClient, err := resolveDocsClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("docs client: %w", err)
	}

	// One company profile feeds both the Hunter/Scorer and the Writer's grounding
	// (WS-A3). The Writer consumes the flattened scorer view, derived from the
	// single profile via scorer.FromProfile rather than a separate hand-maintained
	// scorer JSON.
	//
	// Resolve the profile at runtime (WS-A6 → WS-C1): a tenant-written profile in the
	// ProfileStore (onboarding via the API, no file edits) takes precedence; otherwise
	// an existing deployment with a real profile at the configured path behaves
	// identically; a fresh image with neither grounds the Writer on the generic
	// example template plus an explicit logged warning. ResolveProfileWithStore
	// reports which source was used.
	scorerProfile := &scorer.CapabilityProfile{}
	// profileProvider re-resolves the company profile FRESH per draft so a profile saved
	// via onboarding AFTER this long-lived service starts is honored (the proposal service
	// is built once at startup, so a static profile would otherwise pin a tenant to the
	// boot-time/fallback profile — e.g. proposals branded with the wrong company).
	var profileProvider func() *scorer.CapabilityProfile
	if cfg.Profile.WriterPath != "" {
		// The profile store roots at the SAME base path as the document/opportunity
		// stores so the tenant profile persists alongside them and the API and the
		// proposal pipeline resolve the identical profile.
		profileStore, err := profile.NewJSONProfileStore(opts.BasePath)
		if err != nil {
			return nil, fmt.Errorf("profile store: %w", err)
		}
		writerPath := cfg.Profile.WriterPath
		companyProfile, profileSource, err := profile.ResolveProfileWithStore(profileStore, writerPath)
		if err != nil {
			return nil, fmt.Errorf("load profile: %w", err)
		}
		log.Printf("Company profile source: %s", profileSource)
		derived := scorer.FromProfile(companyProfile)
		scorerProfile = &derived
		profileProvider = func() *scorer.CapabilityProfile {
			cp, _, perr := profile.ResolveProfileWithStore(profileStore, writerPath)
			if perr != nil {
				return nil // fall back to the startup-resolved scorerProfile
			}
			d := scorer.FromProfile(cp)
			return &d
		}
	}

	// contextDocsProvider re-resolves the client's onboarding context documents FRESH per
	// draft (same late-binding rationale as profileProvider): the proposal service is built
	// once at startup, but a client may upload capability statements / past-performance docs
	// during onboarding afterward, and the Writer must ground on them. The store roots at the
	// SAME base path as the profile/document stores so the API's onboarding uploads and this
	// pipeline read the identical documents. The extractor is only used on write (uploads go
	// through the API), so a PlainTextExtractor suffices for this read-only List path.
	var contextDocsProvider func() map[string]string
	if opts.BasePath != "" {
		ctxDocStore, err := contextdoc.NewJSONStore(opts.BasePath, contextdoc.PlainTextExtractor{})
		if err != nil {
			return nil, fmt.Errorf("context-doc store: %w", err)
		}
		contextDocsProvider = func() map[string]string {
			docs, lerr := ctxDocStore.List()
			if lerr != nil || len(docs) == 0 {
				return nil // best-effort grounding; never block drafting on context docs
			}
			out := make(map[string]string, len(docs))
			for _, d := range docs {
				if text := strings.TrimSpace(d.Text); text != "" {
					out[d.Name] = d.Text
				}
			}
			return out
		}
	}

	// The live agents share one Vertex AI region. The Gemini 3.x family —
	// gemini-3.1-pro-preview (drafting) and gemini-3.5-flash (outline structure) —
	// is served only from the global endpoint, so that is the default
	// (config.GCP.AgentRegion defaults to "global", independent of GCP_REGION;
	// overridable only via GCP_AGENT_REGION).
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
		// Real-model failover (upstream #245/#266): the primary stays exactly as before,
		// so the happy path is unchanged; a real-model backup only engages when the
		// primary fails transiently. Outline is the first agent every proposal hits, so a
		// single transient model error must not kill the chain.
		planner, err := outline.NewGeminiSectionPlanner(ctx,
			projectID, region, cfg.GCP.OutlineModel)
		if err != nil {
			return nil, fmt.Errorf("gemini outline planner (primary %s): %w", cfg.GCP.OutlineModel, err)
		}
		backupPlanner, err := outline.NewGeminiSectionPlanner(ctx,
			projectID, region, cfg.GCP.OutlineFallbackModel)
		if err != nil {
			return nil, fmt.Errorf("gemini outline planner (backup %s): %w", cfg.GCP.OutlineFallbackModel, err)
		}
		ol = outline.NewWithPlanner(docsClient, fallback.NewPlanner(planner, backupPlanner))

		gen, err := writer.NewGeminiGenerator(ctx,
			projectID, region, cfg.GCP.WriterModel)
		if err != nil {
			return nil, fmt.Errorf("gemini generator (primary %s): %w", cfg.GCP.WriterModel, err)
		}
		backupGen, err := writer.NewGeminiGenerator(ctx,
			projectID, region, cfg.GCP.WriterFallbackModel)
		if err != nil {
			return nil, fmt.Errorf("gemini generator (backup %s): %w", cfg.GCP.WriterFallbackModel, err)
		}
		w = writer.NewWithGenerator(fallback.NewGenerator(gen, backupGen))
		log.Printf("Outline: LIVE planner %s + fallback %s; Technical Writer %q: LIVE drafting %s + fallback %s (project %s)",
			cfg.GCP.OutlineModel, cfg.GCP.OutlineFallbackModel, "Thomas", cfg.GCP.WriterModel, cfg.GCP.WriterFallbackModel, projectID)
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
			return nil, fmt.Errorf("gemini compliance checker (primary %s): %w", reviewModel, err)
		}
		// Real-model failover (upstream #245): if the primary reviewer errors, fail over
		// to a real-model backup; if both fail, Final Review's own needs-human degrade
		// applies (its deterministic checks still stand). Primary unchanged.
		backupChecker, err := finalreview.NewGeminiComplianceChecker(ctx,
			projectID, cfg.GCP.Region, cfg.GCP.FinalReviewFallbackModel)
		if err != nil {
			return nil, fmt.Errorf("gemini compliance checker (backup %s): %w", cfg.GCP.FinalReviewFallbackModel, err)
		}
		review = finalreview.NewWithComplianceChecker(fallback.NewChecker(checker, backupChecker))
		log.Printf("Final Review: LIVE Gemini compliance pass + fallback (project %s, primary %s, backup %s)", projectID, reviewModel, cfg.GCP.FinalReviewFallbackModel)
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
		Opportunities:       opts.Store,
		Documents:           docs,
		Outline:             ol,
		Writer:              w,
		Review:              review,
		Profile:             scorerProfile,
		ProfileProvider:     profileProvider,
		ContextDocsProvider: contextDocsProvider,
		Ingest:              ingestor,
	}), nil
}

// resolveDocsClient builds the Google Docs client (WS-C2). It returns a client that
// writes into the CUSTOMER's own Drive when ALL of the following hold:
//   - OAuth client credentials were supplied (opts.CustomerDriveOAuth), so a stored
//     refresh token can be refreshed;
//   - the customer has connected their Drive (a token is stored at opts.BasePath);
//   - a target Drive/folder id has been set (where created Docs land).
//
// Otherwise it returns the unchanged cached client. It NEVER returns an error for a
// "not connected / not configured" condition — those fall back to cached so a
// partial setup cannot break the proposal pipeline; only a malformed store
// directory surfaces an error. It never logs the token; only which path it took.
func resolveDocsClient(ctx context.Context, opts Options) (googledocs.Client, error) {
	cached := func() (googledocs.Client, error) {
		return googledocs.NewClient(ctx, googledocs.Config{UseCached: true})
	}

	// No credentials to refresh with → cannot use the customer token; stay cached.
	if opts.CustomerDriveOAuth == nil || opts.BasePath == "" {
		return cached()
	}

	tokenStore, err := drivetoken.NewJSONTokenStore(opts.BasePath)
	if err != nil {
		return nil, fmt.Errorf("drive token store: %w", err)
	}
	oc := drivetoken.NewOAuthConfig(
		opts.CustomerDriveOAuth.ClientID,
		opts.CustomerDriveOAuth.ClientSecret,
		opts.CustomerDriveOAuth.RedirectURL,
	)
	ts, err := drivetoken.TokenSourceFromStore(ctx, tokenStore, oc)
	if err != nil {
		// Not connected (or token unreadable): fall back to the cached client.
		log.Printf("Customer Drive: not connected; using cached Docs client (%v)", err)
		return cached()
	}

	// A token exists; a target Drive id must also be set for created Docs to have a
	// parent. Without one, fall back rather than guess a destination.
	targetStore, err := drivetoken.NewJSONTargetStore(opts.BasePath)
	if err != nil {
		return nil, fmt.Errorf("drive target store: %w", err)
	}
	target, err := targetStore.Load()
	if err != nil || target.DriveID == "" {
		log.Printf("Customer Drive: connected but no target Drive id set; using cached Docs client")
		return cached()
	}

	log.Printf("Customer Drive: LIVE — proposal Docs will be created in the customer's Drive (target %s)", target.DriveID)
	return googledocs.NewClient(ctx, googledocs.Config{
		TokenSource:   ts,
		DestinationID: target.DriveID,
	})
}
