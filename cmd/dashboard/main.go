// Package main implements the dashboard server for Kaimi.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/ingest"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// newMux wires all dashboard routes to the shared branded handler (issue
// #147): the Triage overview at "/", the opportunity detail, and the Zone 2
// surfaces (proposals, workspace, gate actions — issue #156).
// "/opportunities" stays mounted as an alias because the overview's filter
// form submits there.
func newMux(h *dashboard.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.Handle("/opportunities", h)
	mux.Handle("/opportunities/", h)
	return mux
}

// newProposalService wires the REAL Zone 2 agents behind the shared gated
// lifecycle (epic #153): the Outline agent (cached docs client unless live
// Google credentials are configured elsewhere), the Technical Writer (stub
// by default; -live-writer drafts with Gemini over Vertex AI ADC), and the
// Final Review agent (deterministic checks by default; -live-review adds the
// Gemini compliance pass over Vertex AI ADC).
//
// With -live-ingest, the proposal service ingests the solicitation attachments
// (HTTP fetch → GCS store → Document AI / stdlib DOCX extraction) at draft time
// and threads their text into the Writer and the live Final Review compliance
// pass. Without it, ingestion is skipped and the pipeline behaves as before.
func newProposalService(s store.Store, basePath string, liveWriter, liveReview, liveIngest bool, profilePath string) (*proposal.Service, error) {
	docs, err := document.NewStore(basePath)
	if err != nil {
		return nil, fmt.Errorf("document store: %w", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		return nil, fmt.Errorf("docs client: %w", err)
	}

	profile := &scorer.CapabilityProfile{}
	if profilePath != "" {
		data, err := os.ReadFile(profilePath)
		if err != nil {
			return nil, fmt.Errorf("read profile: %w", err)
		}
		if err := json.Unmarshal(data, profile); err != nil {
			return nil, fmt.Errorf("parse profile: %w", err)
		}
	}

	// The live agents share one Vertex AI region. The Gemini 3.x family —
	// gemini-3.1-pro-preview (drafting) and gemini-3.5-flash (outline structure) —
	// is served only from the global endpoint, so that is the default.
	region := envOr("GCP_REGION", "global")

	ol := outline.New(docsClient) // deterministic section planner (offline default)
	w := writer.New()             // stub writer (offline default)
	if liveWriter {
		projectID := envOr("GCP_PROJECT_ID", "")
		if projectID == "" {
			return nil, fmt.Errorf("live agents require GCP_PROJECT_ID (or pass -offline for credential-less UI dev)")
		}
		// Outline plans the section structure with gemini-3.5-flash; the Writer
		// persona "Thomas" drafts the prose with gemini-3.1-pro-preview while the
		// Claude/Opus 4.8 Vertex quota is pending (swap GEMINI_MODEL when it lands).
		planner, err := outline.NewGeminiSectionPlanner(context.Background(),
			projectID, region, envOr("OUTLINE_MODEL", "gemini-3.5-flash"))
		if err != nil {
			return nil, fmt.Errorf("gemini outline planner: %w", err)
		}
		ol = outline.NewWithPlanner(docsClient, planner)

		gen, err := writer.NewGeminiGenerator(context.Background(),
			projectID, region, envOr("GEMINI_MODEL", "gemini-3.1-pro-preview"))
		if err != nil {
			return nil, fmt.Errorf("gemini generator: %w", err)
		}
		w = writer.NewWithGenerator(gen)
		log.Printf("Outline: LIVE gemini-3.5-flash planner; Technical Writer %q: LIVE gemini-3.1-pro-preview drafting (project %s)", "Thomas", projectID)
	} else {
		log.Printf("Outline + Technical Writer: OFFLINE stub mode (-offline) — live Gemini agents are the default")
	}

	review := finalreview.New()
	if liveReview {
		projectID := envOr("GCP_PROJECT_ID", "")
		if projectID == "" {
			return nil, fmt.Errorf("live agents require GCP_PROJECT_ID (or pass -offline for credential-less UI dev)")
		}
		// The reviewer model is configured INDEPENDENTLY of the Writer's GEMINI_MODEL.
		// The Final Review verifier bake-off found gemini-2.5-pro is the best Gemini
		// compliance verifier (83% defect recall) while gemini-3.1-pro is the worst
		// (17%) — so the gate must not silently inherit whatever the Writer is set to.
		// FINALREVIEW_MODEL lets it stay on the validated model (and swap to a Claude
		// model once Anthropic-on-Vertex quota lands), regardless of GEMINI_MODEL.
		reviewModel := envOr("FINALREVIEW_MODEL", "gemini-2.5-pro")
		checker, err := finalreview.NewGeminiComplianceChecker(context.Background(),
			projectID, envOr("GCP_REGION", "us-east4"), reviewModel)
		if err != nil {
			return nil, fmt.Errorf("gemini compliance checker: %w", err)
		}
		review = finalreview.NewWithComplianceChecker(checker)
		log.Printf("Final Review: LIVE Gemini compliance pass enabled (project %s, model %s)", projectID, reviewModel)
	} else {
		log.Printf("Final Review: OFFLINE deterministic checks only (-offline) — live Gemini compliance is the default")
	}

	// Document ingestion is opt-in via -live-ingest. A true nil interface (not a
	// typed-nil) is essential so proposal.Service's `Ingest == nil` check skips it.
	var ingestor proposal.Ingestor
	if liveIngest {
		projectID := envOr("GCP_PROJECT_ID", "")
		bucket := envOr("GCS_SOLICITATIONS_BUCKET", "")
		processorID := envOr("DOCUMENTAI_PROCESSOR_ID", "")
		if projectID == "" || bucket == "" || processorID == "" {
			return nil, fmt.Errorf("-live-ingest requires GCP_PROJECT_ID, GCS_SOLICITATIONS_BUCKET, and DOCUMENTAI_PROCESSOR_ID")
		}
		ctx := context.Background()
		gcs, _, err := ingest.NewGCSStore(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("gcs store: %w", err)
		}
		docAI, _, err := ingest.NewDocumentAIExtractor(ctx, projectID, envOr("DOCUMENTAI_LOCATION", "us"), processorID, bucket)
		if err != nil {
			return nil, fmt.Errorf("document ai extractor: %w", err)
		}
		ingestor = ingest.New(ingest.NewHTTPFetcher(nil, 0), gcs, ingest.NewRoutingExtractor(docAI))
		log.Printf("Document ingestion: LIVE (bucket %s, Document AI processor %s)", bucket, processorID)
	} else {
		log.Printf("Document ingestion: off (pass -live-ingest to fetch + extract solicitation documents)")
	}

	return proposal.NewService(&proposal.Deps{
		Opportunities: s,
		Documents:     docs,
		Outline:       ol,
		Writer:        w,
		Review:        review,
		Profile:       profile,
		Ingest:        ingestor,
	}), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run() error {
	port := flag.Int("port", 8900, "Port to serve on (honors $PORT when set, e.g. Cloud Run)")
	host := flag.String("host", envOr("HOST", "127.0.0.1"), "Interface to bind; use 0.0.0.0 in containers/Cloud Run")
	storePath := flag.String("store", "", "Path to the JSON store directory")
	liveWriter := flag.Bool("live-writer", true, "Draft with the live Gemini writer (default true; -offline disables; Vertex AI ADC; needs GCP_PROJECT_ID)")
	liveReview := flag.Bool("live-review", true, "Run the live Gemini compliance pass in Final Review (default true; -offline disables; Vertex AI ADC; needs GCP_PROJECT_ID)")
	liveIngest := flag.Bool("live-ingest", false, "Ingest solicitation documents (GCS + Document AI; needs GCP_PROJECT_ID, GCS_SOLICITATIONS_BUCKET, DOCUMENTAI_PROCESSOR_ID)")
	offline := flag.Bool("offline", false, "Force all agents to stub/deterministic mode for credential-less UI development (no GCP calls)")
	profilePath := flag.String("profile", "config/bluemeta_scorer_profile.json", "Capability profile JSON for grounding the writer (BlueMeta's real profile by default)")
	flag.Parse()

	// Cloud Run (and most container platforms) inject the listen port via $PORT.
	// Honor it over the flag default so the same binary works locally and hosted.
	if p := envOr("PORT", ""); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			*port = n
		}
	}

	// Live agents are the default; -offline forces the credential-less stub path
	// (Outline deterministic, Writer stub, Final Review deterministic checks only).
	lw, lr := *liveWriter, *liveReview
	if *offline {
		lw, lr = false, false
	}

	if *storePath == "" {
		return fmt.Errorf("--store path is required")
	}

	s, err := store.NewJSONStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	proposals, err := newProposalService(s, *storePath, lw, lr, *liveIngest, *profilePath)
	if err != nil {
		return fmt.Errorf("failed to wire proposal service: %w", err)
	}

	mux := newMux(dashboard.NewHandler(dashboard.NewService(s), dashboard.WithProposals(proposals)))

	addr := net.JoinHostPort(*host, fmt.Sprintf("%d", *port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Starting dashboard on http://%s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("listen: %s\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	// Let in-flight agent stages persist their final status.
	proposals.Wait()

	log.Println("Server exiting")
	return nil
}
