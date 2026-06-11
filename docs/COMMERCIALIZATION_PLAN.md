# Kaimi → Commercial Product: Shared Cloud Backend, Auth, Onboarding & Self-Serve Deploy

**Last updated:** 2026-06-11
**Status:** Founding roadmap for the private `Mawar2/kaimi-platform` repo. Approved by Malik 2026-06-11.

## Context

Kaimi is production BD infrastructure built *for BlueMeta*, but the desktop app was discovered to run on **mock data**, and multiple outside businesses (Ey3 Technologies committed; a large contractor interested) want to use it. The goal is to turn a BlueMeta-internal tool into a **product multiple federal-BD businesses can run**, across web + desktop, on shared cloud services with authentication and per-customer isolation.

The decisive constraint: **BlueMeta is itself a federal contractor competing for the same SAM.gov opportunities as its customers.** A pooled SaaS where BlueMeta hosts competitors' bid/no-bid and capability data is a fatal trust problem. This drove the locked decisions below.

### Locked decisions
- **Isolation:** Isolated per customer — each customer is a *separate deployment* (their own/your-managed GCP project + their own Google Workspace). No pooled row-level multi-tenancy. "Tenancy" = one configurable tenant identity per deployment.
- **Distribution:** Build the foundation so **both** GCP Marketplace and direct SaaS work; pick the primary channel after pilots convert.
- **Scope (this phase):** Foundation build — de-BlueMeta into runtime config, add Google Workspace auth, build the shared API both web + desktop consume (kill mock data), and Terraform-ize deploy.
- **Pilots:** **Self-serve deploy from day one** → install/onboarding polish is a first-class priority *now*.
- **Onboarding:** All BlueMeta-specifics are **replaced in-product via an onboarding flow**, not by hand-editing files. The company profile becomes runtime data, not a baked artifact.
- **Repository:** This commercialization work lives in this **private repo `Mawar2/kaimi-platform`**, *not* in the public hackathon repo (`Mawar2/Kaimi`). **Full git history preserved.** The public repo is kept as an **upstream remote** to cherry-pick core pipeline/agent improvements forward; commercialization code (auth, tenancy, onboarding, Terraform) lives only here.

### Infrastructure judgment (Docker/K8s/Terraform)
- **Docker:** keep (pipeline already containerized).
- **Kubernetes:** **no** — Cloud Run is the right serverless target for a batch pipeline + a lightweight API. K8s adds ops burden with no benefit at this scale.
- **Terraform:** **adopt** — it becomes both the per-customer deploy unit *and* the future Marketplace artifact, replacing the procedural `scripts/setup-gcp.*`.

## Ground-truth architecture (verified against code)

- **Existing HTTP server:** `cmd/dashboard/main.go` runs a real `net/http` server (graceful shutdown, `$PORT`-aware) serving **server-rendered HTML** via `internal/dashboard/handler.go`. **No auth anywhere.**
- **Read data layer (reuse as-is):** `internal/dashboard.Service` — `NewService(store.Store)`, `List(ctx, ListOptions)`, `Get(ctx, id)`, `CountStages`, `DeriveStage`.
- **Zone-2 conductor (reuse as-is):** `internal/proposal.Service` — `Select` (proposal.go:107), `Approve`, `RequestChanges`, `Submit`, `UpdateSection`, `Document`. Wired in `cmd/dashboard/main.go:61 newProposalService`.
- **Store:** `internal/store` (Save/Get/List/Delete, JSON-file impl `json.go`, Firestore-swap-ready). Path `queue/{id}.json`.
- **Schema:** `internal/opportunity/opportunity.go` — has `Selected/SelectedAt/ProposalStatus`, **no tenant field**. Tolerant `omitempty` reads (safe to add fields).
- **Three profile representations** (must reconcile): `internal/capability` (rich YAML, **dead in live path**), `internal/profile` (live eligibility, loads `config/profile.json`), `internal/scorer.CapabilityProfile` (flattened scoring view).
- **Drive:** `internal/googledocs/client.go` writes to one `SharedDriveID` via service-account key/ADC. Hardcoded BlueMeta drive id lives only in `.env`, not `.go`.
- **Desktop:** React prototype on 100% mock data (`kaimi/project/Kaimi Desktop.html` + `app-data.js`); spec in `design-handoff/Kaimi-handoff/DESKTOP.md`; partial `internal/desktop.Backend` exists. Stack direction = Wails (epic #136 / ADR #137).
- **Deploy today:** procedural `scripts/setup-gcp.{sh,ps1}` (Artifact Registry, GCS `${proj}-queue`, Cloud Run **Job** `kaimi-pipeline`, Scheduler, Secret Manager). **No Cloud Run Service for the dashboard/API yet. No Terraform.** Region `us-east4`.

## The shape of the product

```
                         ┌─────────────────────────────────────────┐
   Customer's GCP project │  Cloud Run SERVICE: kaimi-api (NEW)       │
   + their Workspace      │   - Google Workspace OAuth (hd=their dom) │
                          │   - JSON /api/v1 over dashboard+proposal  │
   React web ───────────▶ │   - Onboarding endpoints (profile, Drive) │
   Wails desktop ───────▶ │  Cloud Run JOB: kaimi-pipeline (exists)   │
   (no more mock data)    │  GCS: queue + tenant profile (runtime)    │
                          │  Secret Manager: SAM key, OAuth secret    │
                          └─────────────────────────────────────────┘
            Provisioned by Terraform module (deploy/terraform). No BlueMeta data baked in.
```

---

## Workstreams & tickets

> Discipline (per WORKFLOW.md/CLAUDE.md): every item below becomes a GitHub Issue with acceptance criteria **approved before code**, TDD (test first), two-layer testing (mocked/cached unit + `//go:build live` E2E), legible Go, single-responsibility files (no `utils.go`), errors wrapped with `%w`, and **humans merge — agents never merge**.

### WS-0 — Private repository setup (do this first)
- **0a Create the private repo** — `gh repo create Mawar2/kaimi-platform --private`. ✅ done
- **0b Secret-scrub before pushing** — verify no secrets committed in history (`.env`, `kaimi-sa-key.json`, OAuth/SAM keys gitignored, not tracked). *AC: clean scan; no credentials in history.* ✅ done (clean)
- **0c Push full history** — `git remote add platform <url>`; push `main` with full history. Keep `origin` = public `Mawar2/Kaimi` as **upstream** for cherry-picks. *AC: private repo has complete codebase + history.* ✅ done
- **0d Commit this plan** — `docs/COMMERCIALIZATION_PLAN.md` (this document) as the founding roadmap. ◀ this PR
- **0e CI bootstrap** — port GitHub Actions into the private repo with its own secrets (`GCP_PROJECT_ID`, GCP auth via Workload Identity Federation preferred over JSON key, Gemini key). The de-`kaimi-seeker` generalization (A5/E2) lands here so CI is tenant-neutral from the start. *AC: CI green (test + lint + AI review).*
- **0f All subsequent WS-A…E work happens in this repo** on feature branches → draft PRs → human merge.

### WS-A — De-BlueMeta into runtime config (foundation; nothing breaks the live pipeline)
- **A1 `internal/config`** — one per-deployment `Config` (tenant id/display, GCP project/region/models, Drive target, SAM/OAuth secret refs, store paths). Precedence flags > env > file > default, reusing existing `envOr`/`getEnv` helpers. Thread into `cmd/pipeline` + `cmd/dashboard` `Deps` structs (no behavior change). *AC: pipeline + dashboard run identically; config unit-tested.*
- **A2 `Opportunity.TenantID`** — additive `json:"tenant_id,omitempty"`; pipeline stamps `cfg.Tenant.ID`. **No Store path change** (isolation is at the deployment boundary). *AC: legacy records load; new records carry tenant.*
- **A3 Unify profiles** — delete dead `internal/capability`; keep `internal/profile` as source of truth; add `profile.ToScorerProfile()` to derive the scorer view so one profile feeds Hunter + Scorer. *AC: golden-file parity with old scorer JSON.*
- **A4 Parameterize Writer prompt** — `internal/writer/writer.go:44` "for BlueMeta Technologies" → company name from profile. *AC: no "BlueMeta" literal for a non-BlueMeta profile.*
- **A5 Strip remaining hardcodings** — `cmd/spike` const `kaimi-seeker`; docstring examples in `cmd/eval`, `cmd/scorer`, `cmd/pipeline`, `internal/scorer`, `internal/claudevertex/doc.go`; `.github/workflows/ci.yml` (`kaimi-seeker` → `${{ secrets.GCP_PROJECT_ID }}`); `.env.example`; dashboard brand strings (`internal/dashboard/brand.go`, `handler.go:157` "BlueMeta BD"). Repoint BlueMeta-literal test fixtures to a neutral company. *AC: grep for `kaimi-seeker`/`BlueMeta` in non-test/non-tenant-data files returns nothing functional.*
- **A6 Profile-as-runtime-data** — load the active company profile from the Store/GCS (written by onboarding, WS-C), with `config/profile.example.yaml` as seed/fallback only. *AC: a fresh deployment boots with no real company data baked in.*

### WS-B — Shared JSON API + Google Workspace auth
- **B0 Extract service wiring** — move `newProposalService` (`cmd/dashboard/main.go:61`) into `internal/proposalwiring` so both `cmd/dashboard` and the new API reuse it. *AC: dashboard unchanged + tests pass.*
- **B1 `internal/httpapi` skeleton + `cmd/api`** — new JSON API binary (separate from the HTML `cmd/dashboard`), stdlib `net/http` (Go 1.25 `ServeMux` method+wildcard patterns; no router dep). Files: `doc.go`, `config.go`, `server.go`, `response.go`, `dto.go`. `GET /healthz`. *AC: `make build` produces `bin/api`; healthz 200.*
- **B2 Read endpoints** — `GET /api/v1/opportunities` (stage/score/sort filters, reusing `handler.go:468` parse semantics), `GET /api/v1/opportunities/{id}`, `GET /api/v1/stages/counts`. Export `dashboard.ValidOpportunityID`. *AC: contract tests over a `t.TempDir()` JSON store; 404 via `errors.Is(store.ErrNotFound)`.*
- **B3 Select + proposal status** — `POST /api/v1/opportunities/{id}/select` → `proposal.Service.Select` (202; 409 on already-selected/running); `GET /api/v1/proposals/{id}` composes `Get`+`Document`. *AC: select flips `Selected`; second select 409.*
- **B4 Google Workspace OAuth** — login/callback/logout (`auth.go`), HMAC-signed `HttpOnly; Secure; SameSite=Lax` session cookie (`session.go`, stdlib crypto). Use already-present `golang.org/x/oauth2/google` for code exchange and `google.golang.org/api/idtoken` to verify the ID token + enforce `hd == cfg.AllowedDomain` and `email_verified`. State/PKCE for CSRF. **Zero new modules.** *AC: wrong domain → 403; cookie flags asserted; no live call in unit tests (verifier seam injected).*
- **B5 Auth middleware** — `RequireSession` guards every `/api/v1/*` (401 JSON) and browser nav (redirect to login); `/healthz` + `/auth/*` exempt; add `GET /api/v1/me`. *AC: unauthenticated list call is 401.*
- **B6 `cmd/api` lifecycle + Dockerfile + Cloud Run** — mirror `cmd/dashboard/main.go` lifecycle; same-origin serving of the built SPA preferred (first-party cookie); `-offline` mode for UI dev. *AC: container runs in fresh project; `make build` target added.*
- **B7 `//go:build live` OAuth E2E** — real token exchange against a test Workspace, excluded from `make test`.

### WS-C — Onboarding flow (the heart of "any business can use it")
- **C1 Profile capture API** — `GET/PUT /api/v1/profile`: server validates and persists the tenant company profile (identity, UEI/CAGE, NAICS tiers, set-aside eligibility, competencies, past performance) to the Store/GCS. Replaces `config/bluemeta_profile.yaml` at runtime. *AC: a saved profile drives Hunter eligibility + Scorer + Writer with no redeploy.*
- **C2 Workspace/Drive connect** — onboarding OAuth consent against the customer's Workspace + a Drive picker; persist the chosen target Drive. Add a `TokenSource oauth2.TokenSource` branch to `googledocs.Config`/`newLiveClient` so proposals land in **the customer's own Drive**. *AC: live Doc created in customer Drive via OAuth; cached tests unchanged.*
- **C3 Onboarding UI** — React onboarding screens (`desktop-onboarding.jsx` exists): company profile form, Workspace connect, Drive selection, SAM API key entry, "first pipeline run" CTA. *AC: a brand-new business completes setup end-to-end without editing files.*
- **C4 First-run / empty states** — dashboard renders sensibly before any opportunities exist; guide the user to run the first Hunter pass.

### WS-D — Wire web + desktop to the live API (kill the mock data)
- **D1 Web frontend → API** — replace `window.KAIMI_*` mock data (`kaimi/project/.../app-data.js`) with `fetch('/api/v1/...', {credentials:'include'})`; 401 → `/auth/login`; render identity from `/api/v1/me`. *AC: web dashboard shows real store data.*
- **D2 Desktop → API (Wails)** — desktop drops mock data; uses **OAuth loopback flow** (ephemeral `127.0.0.1` redirect + system browser + PKCE), stores tokens in OS keychain (per `DESKTOP.md`), then calls the same hosted `/api/v1`. Reads may use local `internal/desktop.Backend` offline; **Select/agent runs are online-only**. *AC: desktop shows real data; select triggers a real Zone-2 run.* (Desktop stack/ADR is epic #136 — coordinate.)

### WS-E — Terraform self-serve deploy
- **E1 Terraform module `deploy/terraform/modules/kaimi`** mirroring `setup-gcp.sh` + **adding the Cloud Run Service** for the API: project-services, runtime service account (ADC, **no JSON key**), Artifact Registry, GCS buckets (`${proj}-queue`, `${proj}-solicitations`), Cloud Run Job (pipeline) + Service (api), Scheduler, Secret Manager (`samgov-api-key`, `oauth-client-secret`), least-privilege IAM (`secretmanager.admin`→`secretAccessor`). Inputs = the per-customer config. `envs/example` + `terraform.tfvars.example` + README zero-to-running flow. *AC: `terraform apply` into a clean project yields a running pipeline Job + API Service.*
- **E2 CI generalization** — `deploy-to-gcp` drops hardcoded `kaimi-seeker`, builds/pushes **both** images, applies Terraform; `terraform validate`/`fmt`/`plan` lint job. Deprecate (don't delete) `setup-gcp.{sh,ps1}` until Terraform is proven. *AC: deploy uses secrets + Terraform, no hardcoded project.*

---

## Recommended sequencing

1. **WS-0 first** — stand up the private repo (secret-scrub → push history → commit this plan → CI bootstrap).
2. **Tickets** — create the epic + WS-A/B/C issues with acceptance criteria for approval (hard gate).
3. **WS-A (A1→A6)** then **WS-B0** — pure refactors that keep the deployed pipeline byte-compatible.
4. **WS-B1→B6** — the API + auth (the shared backend both clients need).
5. **WS-C1→C4** — onboarding (what makes it a multi-business product).
6. **WS-D** — point web + desktop at the live API.
7. **WS-E** — Terraform self-serve deploy + CI cutover (can start in parallel after A1).

A natural **first PR slice** for momentum: A1 (`internal/config`) + B0 (extract wiring) + B1 (`cmd/api` skeleton + healthz) — all additive, low-risk, and they unblock everything downstream.

## Critical files
- `cmd/dashboard/main.go` — server lifecycle + `newProposalService` to extract (B0)
- `internal/dashboard/view.go`, `handler.go` — read `Service` to wrap; query-parse + id-validation to reuse (B2)
- `internal/proposal/proposal.go` — Zone-2 conductor to expose over JSON (B3)
- `internal/profile/profile.go` (+ delete `internal/capability/profile.go`) — profile unification (A3)
- `internal/writer/writer.go` — de-hardcode company name (A4)
- `internal/googledocs/client.go` — OAuth `TokenSource` branch for customer Drive (C2)
- `internal/opportunity/opportunity.go` — `TenantID` field (A2)
- `internal/claudevertex/generator.go` — existing `golang.org/x/oauth2` usage to follow (B4)
- `scripts/setup-gcp.sh` — source of truth to port to Terraform (E1)
- `.github/workflows/ci.yml` — de-`kaimi-seeker` + two-image deploy (A5, E2)

## Verification
- **Unit/contract (every commit):** `make test` — config precedence; profile golden-file parity; `Opportunity` legacy-record round-trip; API routes via `httptest` over a `t.TempDir()` store with faked proposal service + injected OAuth verifier (no network); session sign/verify + `hd` enforcement; cookie flags.
- **Lint/format:** `make lint`, `gofmt`; `terraform validate`/`fmt -check`/`plan` in CI.
- **Live/E2E (separate, opt-in):** `//go:build live` real Workspace OAuth (B7) + real Doc creation in a test Drive (C2); `internal/e2e` pipeline run.
- **Manual end-to-end (the real proof):** `terraform apply` into a clean throwaway GCP project + test Workspace → complete onboarding as a *non-BlueMeta* company → run a Hunter pass → see scored opps in web + desktop → select one → Zone-2 drafts a proposal into the test company's own Drive. No BlueMeta identity anywhere.

## Out of scope / forward markers
- Pooled multi-tenant SaaS, Firestore swap, billing/metering, FedRAMP — `// TODO(phase-N)`; not built ahead of an approved ticket.
- Desktop stack/ADR (#136/#137) is its own epic; this plan provides the API it consumes.
- GCP Marketplace listing packaging (the Terraform module is built Marketplace-ready; the listing submission itself is a later, channel-decision-gated step).
