// Package main implements the dashboard server for Kaimi.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

type server struct {
	svc  *dashboard.Service
	tmpl *template.Template
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 8900, "Port to serve on")
	storePath := flag.String("store", "", "Path to the JSON store directory")
	flag.Parse()

	if *storePath == "" {
		return fmt.Errorf("--store path is required")
	}

	s, err := store.NewJSONStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	svc := dashboard.NewService(s)

	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}

	srv := &server{
		svc:  svc,
		tmpl: tmpl,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleOverview)

	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", *port))
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

	log.Println("Server exiting")
	return nil
}

func (s *server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	rows, err := s.svc.List(r.Context(), dashboard.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	counts := make(map[dashboard.Stage]int)
	for i := range rows {
		counts[rows[i].Stage]++
	}

	data := struct {
		Stages []struct {
			Name dashboard.Stage
		}
		Counts map[dashboard.Stage]int
	}{
		Stages: []struct {
			Name dashboard.Stage
		}{
			{dashboard.StageHunted},
			{dashboard.StageScored},
			{dashboard.StageSelected},
			{dashboard.StageInProposal},
			{dashboard.StageAwaitingHumanReview},
			{dashboard.StageFinalized},
		},
		Counts: counts,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("Template error: %v", err)
	}
}
