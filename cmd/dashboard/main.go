// Package main implements the dashboard server for Kaimi.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/proposalwiring"
	"github.com/Mawar2/Kaimi/internal/store"
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run() error {
	// Flag defaults shown as the env-or-default value so --help reflects the
	// effective default; config.Load applies the canonical precedence and an
	// unset flag falls through to env/file/default.
	port := flag.Int("port", 8900, "Port to serve on (honors $PORT when set, e.g. Cloud Run)")
	host := flag.String("host", envOr("HOST", "127.0.0.1"), "Interface to bind; use 0.0.0.0 in containers/Cloud Run")
	storePath := flag.String("store", "", "Path to the JSON store directory")
	liveWriter := flag.Bool("live-writer", true, "Draft with the live Gemini writer (default true; -offline disables; Vertex AI ADC; needs GCP_PROJECT_ID)")
	liveReview := flag.Bool("live-review", true, "Run the live Gemini compliance pass in Final Review (default true; -offline disables; Vertex AI ADC; needs GCP_PROJECT_ID)")
	liveIngest := flag.Bool("live-ingest", false, "Ingest solicitation documents (GCS + Document AI; needs GCP_PROJECT_ID, GCS_SOLICITATIONS_BUCKET, DOCUMENTAI_PROCESSOR_ID)")
	offline := flag.Bool("offline", false, "Force all agents to stub/deterministic mode for credential-less UI development (no GCP calls)")
	profilePath := flag.String("profile", "config/profile.json", "Company profile JSON/YAML for grounding the writer (the Scorer view is derived from it)")
	flag.Parse()

	// Resolve the tenant configuration (GCP project/region, model names, ingest
	// targets, writer profile path) through internal/config. Only flags the
	// operator explicitly set are forwarded so env/file values are not shadowed
	// by flag defaults.
	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	cfgFlags := &config.Flags{}
	if set["host"] {
		cfgFlags.Host = host
	}
	if set["profile"] {
		cfgFlags.WriterProfilePath = profilePath
	}
	cfg, err := config.Load(cfgFlags)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Host precedence is flag > env > default, which config.Load already applied.
	*host = cfg.Server.Host

	// Port keeps its historic precedence: the -port flag default is 8900, but
	// $PORT (injected by Cloud Run and most container platforms) overrides it
	// unconditionally after flag parsing. config.Server.Port already resolves
	// $PORT over the default, so honor it whenever $PORT was set.
	if envOr("PORT", "") != "" {
		*port = cfg.Server.Port
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

	proposals, err := proposalwiring.New(context.Background(), &cfg, proposalwiring.Options{
		Store:      s,
		BasePath:   *storePath,
		LiveWriter: lw,
		LiveReview: lr,
		LiveIngest: *liveIngest,
	})
	if err != nil {
		return fmt.Errorf("failed to wire proposal service: %w", err)
	}

	mux := newMux(dashboard.NewHandler(dashboard.NewService(s),
		dashboard.WithProposals(proposals),
		dashboard.WithTenantName(cfg.Tenant.DisplayName)))

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
