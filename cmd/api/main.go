// Package main implements the JSON API server for Kaimi.
//
// It serves the programmatic surface over the same opportunity store, dashboard
// read layer, and proposal action service that cmd/dashboard renders. Today the
// API exposes only GET /healthz (WS-B1); the read, select, and OAuth endpoints
// land in later tickets without changing this lifecycle.
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
	"github.com/Mawar2/Kaimi/internal/httpapi"
	"github.com/Mawar2/Kaimi/internal/proposalwiring"
	"github.com/Mawar2/Kaimi/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Mirror cmd/dashboard's flag surface so the two binaries share an operator
	// model. -offline forces the credential-less stub agents for local/UI dev;
	// otherwise the live Gemini writer/review are the default.
	storePath := flag.String("store", "", "Path to the JSON store directory")
	liveWriter := flag.Bool("live-writer", true, "Draft with the live Gemini writer (default true; -offline disables; needs GCP_PROJECT_ID)")
	liveReview := flag.Bool("live-review", true, "Run the live Gemini compliance pass in Final Review (default true; -offline disables; needs GCP_PROJECT_ID)")
	liveIngest := flag.Bool("live-ingest", false, "Ingest solicitation documents (needs GCP_PROJECT_ID, GCS_SOLICITATIONS_BUCKET, DOCUMENTAI_PROCESSOR_ID)")
	offline := flag.Bool("offline", false, "Force all agents to stub/deterministic mode for credential-less local/UI development (no GCP calls)")
	profilePath := flag.String("profile", "config/profile.json", "Company profile JSON/YAML for grounding the writer")
	host := flag.String("host", "", "Interface to bind; use 0.0.0.0 in containers/Cloud Run (overrides API_HOST)")
	port := flag.Int("port", 0, "Port to serve on (overrides API_PORT; $PORT still wins for Cloud Run)")
	flag.Parse()

	// The HTTP/server layer config (bind address) is resolved independently of the
	// app-wide tenant config, then optionally overridden by flags the operator set.
	apiCfg, err := httpapi.LoadConfig()
	if err != nil {
		return fmt.Errorf("load API config: %w", err)
	}
	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	if set["host"] {
		apiCfg.Host = *host
	}
	if set["port"] {
		apiCfg.Port = *port
	}

	// Resolve the tenant configuration (GCP project/region, model names, ingest
	// targets, writer profile path) through internal/config. Only flags the
	// operator explicitly set are forwarded so env/file values are not shadowed.
	cfgFlags := &config.Flags{}
	if set["profile"] {
		cfgFlags.WriterProfilePath = profilePath
	}
	cfg, err := config.Load(cfgFlags)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Live agents are the default; -offline forces the credential-less stub path.
	lw, lr := *liveWriter, *liveReview
	if *offline {
		lw, lr = false, false
	}

	if *storePath == "" {
		return fmt.Errorf("--store path is required")
	}

	// Build the same JSON store the dashboard uses; reads and actions share it.
	s, err := store.NewJSONStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	// Assemble the Zone-2 proposal service through the shared wiring so the API
	// builds it exactly the way cmd/dashboard does.
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

	srv := httpapi.New(httpapi.Deps{
		Dashboard: dashboard.NewService(s),
		Proposals: proposals,
	})

	addr := net.JoinHostPort(apiCfg.Host, fmt.Sprintf("%d", apiCfg.Port))
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Starting Kaimi API on http://%s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("listen: %s\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down API server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}
	// Let in-flight agent stages persist their final status.
	proposals.Wait()

	log.Println("API server exiting")
	return nil
}
