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
	"strconv"
	"syscall"
	"time"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/httpapi"
	"github.com/Mawar2/Kaimi/internal/profile"
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
	insecureNoAuth := flag.Bool("insecure-no-auth", false, "DEV-ONLY / INSECURE: serve the /api/v1 API WITHOUT authentication when OAuth is not configured. Without this flag the server REFUSES to start unconfigured (fail closed). Also honored via KAIMI_INSECURE_NO_AUTH=true. NEVER set in production.")
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

	// Runtime company-profile store (WS-C1), rooted at the SAME store base path so
	// the tenant profile persists alongside the opportunity queue. It backs the
	// GET/PUT /api/v1/profile onboarding endpoints.
	profileStore, err := profile.NewJSONProfileStore(*storePath)
	if err != nil {
		return fmt.Errorf("failed to initialize profile store: %w", err)
	}

	// Resolve customer-Drive connect (WS-C2). It is OPTIONAL and independent of
	// sign-in OAuth: with no DRIVE_OAUTH_* env set the connect endpoints are omitted
	// (they answer 503) and proposal Docs use the default service-account/cached
	// path. When configured, a deployment can connect the CUSTOMER's own Drive so
	// generated Docs land in their Workspace. The token/target are persisted under
	// the SAME store base path as the profile/opportunity stores.
	driveOAuthCfg, driveEnabled, err := httpapi.LoadDriveOAuthConfig()
	if err != nil {
		return fmt.Errorf("load Drive OAuth config: %w", err)
	}
	var driveHandler *httpapi.DriveHandler
	var customerDriveOAuth *drivetoken.OAuthClient
	if driveEnabled {
		driveTokenStore, err := drivetoken.NewJSONTokenStore(*storePath)
		if err != nil {
			return fmt.Errorf("failed to initialize drive token store: %w", err)
		}
		driveTargetStore, err := drivetoken.NewJSONTargetStore(*storePath)
		if err != nil {
			return fmt.Errorf("failed to initialize drive target store: %w", err)
		}
		driveHandler, err = httpapi.NewDriveHandler(driveOAuthCfg, driveTokenStore, driveTargetStore)
		if err != nil {
			return fmt.Errorf("build drive handler: %w", err)
		}
		// Same client credentials let the proposal pipeline refresh the stored token
		// so Docs land in the customer's Drive once connected.
		customerDriveOAuth = &drivetoken.OAuthClient{
			ClientID:     driveOAuthCfg.ClientID,
			ClientSecret: driveOAuthCfg.ClientSecret,
			RedirectURL:  driveOAuthCfg.RedirectURL,
		}
		log.Printf("Customer-Drive connect enabled (/api/v1/integrations/drive/*)")
	} else {
		log.Printf("Customer-Drive connect disabled (no DRIVE_OAUTH_* config); proposal Docs use the default Drive client")
	}

	// Assemble the Zone-2 proposal service through the shared wiring so the API
	// builds it exactly the way cmd/dashboard does.
	proposals, err := proposalwiring.New(context.Background(), &cfg, proposalwiring.Options{
		Store:              s,
		BasePath:           *storePath,
		LiveWriter:         lw,
		LiveReview:         lr,
		LiveIngest:         *liveIngest,
		CustomerDriveOAuth: customerDriveOAuth,
	})
	if err != nil {
		return fmt.Errorf("failed to wire proposal service: %w", err)
	}

	// Resolve Workspace OAuth sign-in (WS-B4). It is OPTIONAL: with no OAUTH_*/
	// SESSION_SECRET env set, auth is disabled and the /auth/* routes are omitted so
	// the offline/dev mode still runs. PRODUCTION must set them (Secret Manager →
	// env in Cloud Run). When enabled, the auth handler also backs the WS-B5
	// RequireSession middleware via ParseSession.
	oauthCfg, oauthEnabled, err := httpapi.LoadOAuthConfig()
	if err != nil {
		return fmt.Errorf("load OAuth config: %w", err)
	}
	var auth *httpapi.AuthHandler
	if oauthEnabled {
		auth, err = httpapi.NewAuthHandler(&oauthCfg)
		if err != nil {
			return fmt.Errorf("build auth handler: %w", err)
		}
		log.Printf("Workspace OAuth enabled for domain %q", oauthCfg.AllowedDomain)
	} else {
		log.Printf("Workspace OAuth disabled (no OAUTH_* config); /auth/* routes omitted")
	}

	// Decide whether running WITHOUT auth is permitted. This is the fail-closed gate:
	// an unconfigured server only starts when the operator EXPLICITLY opts in to the
	// insecure path, either with -insecure-no-auth or KAIMI_INSECURE_NO_AUTH=true.
	// When OAuth is configured this is irrelevant (OAuth always wins in Routes()).
	// A malformed env value (anything strconv.ParseBool rejects) is treated as false
	// so a typo'd env var stays on the safe, fail-closed side.
	envInsecure, _ := strconv.ParseBool(os.Getenv("KAIMI_INSECURE_NO_AUTH"))
	allowInsecure := *insecureNoAuth || envInsecure
	if !oauthEnabled && !allowInsecure {
		log.Fatal("Workspace OAuth is not configured and -insecure-no-auth was not set: refusing to start an unauthenticated API. " +
			"Configure OAUTH_*/SESSION_SECRET for production, or pass -insecure-no-auth (or KAIMI_INSECURE_NO_AUTH=true) for local dev only.")
	}

	// WS-C3a: build the same SSR dashboard cmd/dashboard renders, over the SAME store
	// and proposal service, and serve it from this authed server so there is ONE
	// authed, same-origin web server. The HTML pages render real Store data (no mock);
	// the post-login redirect default ("/") lands here. WithProposals enables the
	// Zone-2 surfaces (select/workspace/gate); WithTenantName sets the sidebar label.
	dashboardSvc := dashboard.NewService(s)
	dashboardOpts := []dashboard.Option{
		dashboard.WithProposals(proposals),
		dashboard.WithTenantName(cfg.Tenant.DisplayName),
		// WS-C3 onboarding: the in-product setup flow reuses the same runtime profile
		// store the JSON API does, so /onboarding pre-fills + persists the company
		// profile and the Triage screen surfaces a first-run "Complete onboarding" link.
		dashboard.WithProfileStore(profileStore),
	}
	// Inject the signed-in identity + per-session CSRF token so the onboarding form
	// shows who is signed in and is CSRF-protected. The dashboard cannot read the
	// httpapi session directly (import cycle), so cmd/api adapts AuthHandler's
	// DashboardIdentity into a dashboard.IdentityFunc here. With OAuth disabled
	// (offline/dev), no identity is wired and onboarding relies on SameSite=Lax alone.
	if auth != nil {
		dashboardOpts = append(dashboardOpts, dashboard.WithIdentity(
			func(ctx context.Context) (dashboard.Identity, bool) {
				email, csrf, ok := auth.DashboardIdentity(ctx)
				return dashboard.Identity{Email: email, CSRFToken: csrf}, ok
			}))
	}
	// Show the WS-C2 Drive connection state on the onboarding page when customer-Drive
	// connect is configured. Read straight from the drivetoken stores (no httpapi/
	// dashboard cycle); when Drive connect is disabled the page shows the
	// "not available in this deployment" treatment.
	if driveEnabled {
		driveTokenStore, err := drivetoken.NewJSONTokenStore(*storePath)
		if err != nil {
			return fmt.Errorf("failed to initialize drive token store for onboarding: %w", err)
		}
		driveTargetStore, err := drivetoken.NewJSONTargetStore(*storePath)
		if err != nil {
			return fmt.Errorf("failed to initialize drive target store for onboarding: %w", err)
		}
		dashboardOpts = append(dashboardOpts, dashboard.WithDriveStatus(
			dashboard.DriveStatusFromStores(driveTokenStore, driveTargetStore)))
	}
	dashboardHTML := dashboard.NewHandler(dashboardSvc, dashboardOpts...)

	srv := httpapi.New(httpapi.Deps{
		Dashboard:           dashboardSvc,
		DashboardHTML:       dashboardHTML,
		Proposals:           proposals,
		ProfileStore:        profileStore,
		Auth:                auth,
		Drive:               driveHandler,
		AllowInsecureNoAuth: allowInsecure,
		// CORS allow-list from CORS_ALLOWED_ORIGINS (empty by default → same-origin,
		// no-op). Set only when a browser SPA is served from a different origin.
		AllowedOrigins: apiCfg.AllowedOrigins,
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
