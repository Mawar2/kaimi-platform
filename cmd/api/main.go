// Package main implements the JSON API server for Kaimi.
//
// It serves the programmatic surface over the same opportunity store, dashboard
// read layer, and proposal action service that cmd/dashboard renders. Today the
// API exposes only GET /healthz (WS-B1); the read, select, and OAuth endpoints
// land in later tickets without changing this lifecycle.
package main

import (
	"context"
	"crypto/rand"
	"errors"
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
	"github.com/Mawar2/Kaimi/internal/contextdoc"
	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/httpapi"
	"github.com/Mawar2/Kaimi/internal/ingest"
	"github.com/Mawar2/Kaimi/internal/productkey"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/proposalwiring"
	"github.com/Mawar2/Kaimi/internal/samsecret"
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
	insecureNoAuth := flag.Bool("insecure-no-auth", false, "DEV-ONLY / INSECURE: serve the /api/v1 API WITHOUT authentication when no gate is configured. Without this flag the server REFUSES to start unconfigured (fail closed). Also honored via KAIMI_INSECURE_NO_AUTH=true. NEVER set in production.")
	devSeedKey := flag.Bool("dev-seed-key", false, "DEV-ONLY: in product-key gate mode, mint one 14-day key in the (in-memory) registry at startup and log its magic link for local browser testing. Requires -insecure-no-auth.")
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

	// Decide whether running WITHOUT a gate is permitted. This is the fail-closed
	// backstop: an unconfigured server only starts when the operator EXPLICITLY opts in
	// to the insecure path (-insecure-no-auth or KAIMI_INSECURE_NO_AUTH=true). A
	// malformed env value (anything strconv.ParseBool rejects) is treated as false so a
	// typo'd env var stays on the safe, fail-closed side.
	envInsecure, _ := strconv.ParseBool(os.Getenv("KAIMI_INSECURE_NO_AUTH"))
	allowInsecure := *insecureNoAuth || envInsecure

	// Resolve the access gate. A deployment runs exactly one of:
	//   - product-key: the pilot gate (KAIMI_GATE_MODE=product-key). Workspace sign-in
	//     is OFF; Google OAuth is used only for the customer-Drive connect above.
	//   - workspace-oauth (default): the WS-B4 Google Workspace sign-in.
	gateMode, err := httpapi.LoadGateMode()
	if err != nil {
		return fmt.Errorf("load gate mode: %w", err)
	}

	var auth *httpapi.AuthHandler
	var productKeyGate *httpapi.ProductKeyGate
	switch gateMode {
	case httpapi.GateModeProductKey:
		reg, regDesc, rerr := buildProductKeyRegistry(context.Background(), cfg.GCP.ProjectID, allowInsecure)
		if rerr != nil {
			return rerr
		}
		// Close a Firestore-backed registry on shutdown; the in-memory one is a no-op.
		if c, ok := reg.(interface{ Close() error }); ok {
			defer func() { _ = c.Close() }()
		}
		secret, serr := resolveSessionSecret(allowInsecure)
		if serr != nil {
			return serr
		}
		// 12h session TTL; the key's own expiry caps it further (SetSessionBounded).
		productKeyGate, err = httpapi.NewProductKeyGate(reg, secret, 12*time.Hour, "/")
		if err != nil {
			return fmt.Errorf("build product-key gate: %w", err)
		}
		log.Printf("Access gate: PRODUCT-KEY mode (%s); Workspace sign-in disabled", regDesc)
		if *devSeedKey {
			if !allowInsecure {
				return fmt.Errorf("-dev-seed-key is a dev-only helper and requires -insecure-no-auth")
			}
			seedDevProductKey(context.Background(), reg, apiCfg)
		}
	default: // GateModeWorkspaceOAuth
		oauthCfg, oauthEnabled, oerr := httpapi.LoadOAuthConfig()
		if oerr != nil {
			return fmt.Errorf("load OAuth config: %w", oerr)
		}
		if oauthEnabled {
			auth, err = httpapi.NewAuthHandler(&oauthCfg)
			if err != nil {
				return fmt.Errorf("build auth handler: %w", err)
			}
			log.Printf("Workspace OAuth enabled for domain %q", oauthCfg.AllowedDomain)
		} else {
			log.Printf("Workspace OAuth disabled (no OAUTH_* config); /auth/* routes omitted")
		}
	}

	// Fail closed: a deployment must have SOME gate unless the operator explicitly
	// opted into the insecure dev path.
	gateConfigured := productKeyGate != nil || auth != nil
	if !gateConfigured && !allowInsecure {
		// Return (not log.Fatal) so deferred cleanup — e.g. the Firestore registry
		// Close above — still runs. run()'s caller prints the error and exits non-zero.
		return errors.New("no access gate configured and -insecure-no-auth was not set: refusing to start an unauthenticated API; " +
			"for the pilot gate set KAIMI_GATE_MODE=product-key with GCP_PROJECT_ID + SESSION_SECRET, " +
			"for Workspace sign-in set OAUTH_*/SESSION_SECRET, or pass -insecure-no-auth for local dev only")
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
		// Fail-closed mutation gate for the onboarding profile write: pass the SAME
		// allowInsecure opt-in the API/RequireSessionHTML use. In production
		// (allowInsecure == false) an onboarding POST with no resolvable session is
		// rejected rather than silently allowed; only an explicit -insecure-no-auth /
		// KAIMI_INSECURE_NO_AUTH dev run permits the unauthenticated (CSRF-free) write.
		dashboard.WithInsecureNoAuth(allowInsecure),
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
	} else if productKeyGate != nil {
		// Product-key mode: derive the onboarding form's CSRF token from the product-key
		// session (no Google identity, so email is empty). This keeps the onboarding PUT
		// CSRF-protected without Workspace OAuth — the gate is the access control, and the
		// token is bound to the session's key id over the same HMAC key.
		dashboardOpts = append(dashboardOpts, dashboard.WithIdentity(
			func(ctx context.Context) (dashboard.Identity, bool) {
				email, csrf, license, ok := productKeyGate.DashboardIdentity(ctx)
				return dashboard.Identity{Email: email, CSRFToken: csrf, LicenseKey: license}, ok
			}))
	}

	// SAM.gov key entry on the onboarding "Connect" step (per-tenant key → own quota).
	// When SAM_SECRET_NAME is set, wire a Secret Manager writer so a tester's key is
	// stored as a new version of the deployment's SAM secret (which the pipeline reads).
	// Without it, onboarding shows the "managed by your administrator" note. The runtime
	// SA needs roles/secretmanager.secretVersionAdder on that secret.
	if samSecretName := os.Getenv("SAM_SECRET_NAME"); samSecretName != "" && cfg.GCP.ProjectID != "" {
		samWriter, serr := samsecret.NewSecretManagerWriter(context.Background(), cfg.GCP.ProjectID, samSecretName)
		if serr != nil {
			return fmt.Errorf("build SAM key writer: %w", serr)
		}
		defer func() { _ = samWriter.Close() }()
		dashboardOpts = append(dashboardOpts, dashboard.WithSAMKeySaver(samWriter.Save))
		log.Printf("Onboarding SAM.gov key entry enabled (writes to Secret Manager secret %q)", samSecretName)
	}

	// Context-document uploads (onboarding "Connect" step) → their extracted text feeds
	// the capability map. Stored under the tenant store path. The RoutingExtractor handles
	// DOCX via the stdlib and routes everything else to its primary; with no Document AI
	// wired here the primary is a plain-text passthrough, so DOCX/text/markdown extract and
	// PDFs/images are stored with empty text (Document AI for those is a follow-on).
	ctxDocStore, err := contextdoc.NewJSONStore(*storePath, ingest.NewRoutingExtractor(contextdoc.PlainTextExtractor{}))
	if err != nil {
		return fmt.Errorf("failed to initialize context-doc store: %w", err)
	}
	dashboardOpts = append(dashboardOpts, dashboard.WithContextDocs(ctxDocStore))
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
		dashboardOpts = append(dashboardOpts,
			dashboard.WithDriveStatus(
				dashboard.DriveStatusFromStores(driveTokenStore, driveTargetStore)),
			// WS-C5b: let the onboarding/settings page change the Drive destination
			// without editing files. Back it with the SAME target store the JSON PUT
			// /api/v1/integrations/drive/target writes to, so the two surfaces persist to
			// one store and never diverge. driveTargetStore.Save rejects an empty id,
			// mirroring the PUT endpoint; the SSR handler resolves the "My Drive root"
			// choice to the "root" sentinel before saving.
			dashboard.WithDriveTargetSaver(
				func(driveID string) error {
					return driveTargetStore.Save(drivetoken.Target{DriveID: driveID})
				}))
	}
	dashboardHTML := dashboard.NewHandler(dashboardSvc, dashboardOpts...)

	srv := httpapi.New(httpapi.Deps{
		Dashboard:           dashboardSvc,
		DashboardHTML:       dashboardHTML,
		Proposals:           proposals,
		ProfileStore:        profileStore,
		Auth:                auth,
		ProductKey:          productKeyGate,
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

// buildProductKeyRegistry selects the product-key registry for the access gate. With a
// GCP project it uses the durable Firestore registry (production). With NO project it
// falls back to the in-memory registry, but ONLY when the insecure dev opt-in is set —
// otherwise it fails closed, because a non-durable registry behind a real access gate
// would silently lose every issued key on restart. It returns the registry and a short
// human description for the startup log.
func buildProductKeyRegistry(ctx context.Context, projectID string, allowInsecure bool) (productkey.Registry, string, error) {
	if projectID != "" {
		reg, err := productkey.NewFirestoreRegistry(ctx, projectID)
		if err != nil {
			return nil, "", fmt.Errorf("build Firestore key registry: %w", err)
		}
		return reg, "Firestore project " + projectID, nil
	}
	if !allowInsecure {
		return nil, "", fmt.Errorf("product-key gate needs GCP_PROJECT_ID for the Firestore key registry (or -insecure-no-auth for the in-memory dev registry): %w", httpapi.ErrMissingRequired)
	}
	log.Printf("WARNING: product-key gate using IN-MEMORY registry (no GCP_PROJECT_ID); keys do not persist across restarts. Dev only.")
	return productkey.NewMemoryRegistry(), "in-memory dev registry", nil
}

// resolveSessionSecret returns the HMAC key that signs product-key gate sessions. It
// requires SESSION_SECRET in any real deployment; only the explicit insecure dev opt-in
// permits an EPHEMERAL random secret (sessions then reset on restart). Failing closed
// here prevents a production gate from running with an unstable or absent signing key.
func resolveSessionSecret(allowInsecure bool) ([]byte, error) {
	if s := os.Getenv(envSessionSecret); s != "" {
		return []byte(s), nil
	}
	if !allowInsecure {
		return nil, fmt.Errorf("%s is required for the product-key gate: %w", envSessionSecret, httpapi.ErrMissingRequired)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate ephemeral session secret: %w", err)
	}
	log.Printf("WARNING: product-key gate using an EPHEMERAL session secret (no %s); all sessions reset on restart. Dev only.", envSessionSecret)
	return b, nil
}

// seedDevProductKey mints one 14-day key in the (in-memory dev) registry and logs its
// magic link, so a developer can click straight into the gated app locally. It is a
// dev-only convenience guarded by -dev-seed-key + -insecure-no-auth; it never runs in a
// real deployment.
func seedDevProductKey(ctx context.Context, reg productkey.Registry, apiCfg httpapi.Config) {
	rec, err := reg.Mint(ctx, "dev seed (local browser test)", 14*24*time.Hour)
	if err != nil {
		log.Printf("dev seed key: mint failed: %v", err)
		return
	}
	addr := net.JoinHostPort(apiCfg.Host, strconv.Itoa(apiCfg.Port))
	log.Printf("DEV SEED KEY: %s", rec.Key)
	log.Printf("DEV MAGIC LINK: http://%s/access?key=%s", addr, rec.Key)
}

// envSessionSecret is the env var holding the HMAC session-signing key. It mirrors the
// constant used by the Workspace-OAuth path (httpapi.LoadOAuthConfig reads the same
// SESSION_SECRET), so both gate modes sign sessions with the operator's one secret.
const envSessionSecret = "SESSION_SECRET"
