# GOAL: Pilot Access & Cost Control (RELEASE 003)

**Owner:** autonomous /loop · **Created:** 2026-06-17 · **Repo:** Mawar2/kaimi-platform
**Builds on:** RELEASE 002 (upstream parity). **Revert points:** tag `release-001`, tag `release-002`.

## Objective
Let BlueMeta run outside testers (Ey3 first) on BlueMeta-hosted infrastructure for round 1,
with **time-limited product-key access** and **per-tester cost visibility** — without
multi-tenancy (each tester is isolated).

## Locked decisions (Malik, 2026-06-17)
- **Isolation = own GCP project per tester** (under BlueMeta's billing). Each tester = an
  isolated Kaimi deploy (own project, own profile/queue, proposals → their own Drive),
  using BlueMeta's SAM.gov + Vertex keys for round 1.
- **Access = Product Key** `KAIMI-XXXX-XXXX-XXXX`, time-limited (per-key `--days`, e.g. 7/14),
  the access gate. Google sign-in is used ONLY for the Drive connect (proposals output).
- **Key registry = Firestore** (instant revoke, expiry, audit).
- **Cost monitoring = per-project GCP Billing + budget alert** (source of truth, all-in incl.
  Vertex) **PLUS per-key Vertex token metering** in-app (granular AI spend).

## Cost-attribution rationale
Vertex AI is billed per-project, so per-tester cost is clean ONLY with a project per tester →
GCP Billing filtered by project = exact all-in cost; a per-project budget alert emails (and can
cap) at a threshold. The in-app token meter adds per-key/per-draft granularity on top.

## Phases (each: TDD, go gate, independent review, browser-validate UI, cite evidence)

### P0 — Cut RELEASE 002 first (parity is done; lock it in)
- Full `go test ./...` green except cmd/desktop + lint 0 + gofmt on platform/main.
- Tag+push `release-002` + GitHub Release (upstream parity: SAM ncode fix, agent failover,
  Vera/finalreview, zone2view Zone-2 view, gate QA + gap flagging).
- Rebuild+redeploy the **bm dogfood** (us-east4-docker.pkg.dev/kaimi-seeker/bm-kaimi/api:latest →
  gcloud run services update bm-kaimi-api) + live smoke. (bm stays Workspace-OAuth; the
  product-key gate ships with the per-tester deploys, not the bm dogfood.)

### P1 — Product-key core (internal/productkey + CLI)
- `internal/productkey`: generate `KAIMI-XXXX-XXXX-XXXX` (crypto/rand, unambiguous alphabet);
  `Registry` interface + Firestore impl: Mint(label, ttl) → key+record, Lookup(key), Revoke(key),
  List(). Record: {key, label/tester, issued_at, expires_at, revoked}. Validate = exists && now<expires && !revoked.
- `cmd/kaimi-key`: `mint --tester "Ey3" --days 14`, `revoke <key>`, `list`. ADC to Firestore.
- TDD with a fake registry; Firestore impl behind the interface. SECURITY review.

### P2 — Access gate (httpapi) — SECURITY-SENSITIVE
- Product-key entry page (GET) + POST validate → on success issue the existing HMAC session
  carrying {key id, expiry}; session expiry = min(key expiry, cookie max-age).
- `RequireProductKey` middleware: fail-closed on ALL app + /api/v1 routes except the entry page,
  /healthz, and the Drive-connect callback. No valid key session → entry page / 401.
- This REPLACES the Workspace-hd sign-in for per-tester deploys (Google OAuth kept ONLY for
  /api/v1/integrations/drive/*). Config flag to select gate mode (product-key vs workspace-oauth).
- Verify the security properties myself: fail-closed (no key → no access), expiry enforced,
  revoke takes effect, no route bypass, session can't be forged. Browser-validate the gate.

### P3 — Per-key Vertex token metering
- Capture Vertex `UsageMetadata` (prompt + candidate tokens) per agent LLM call
  (outline planner, writer generator, finalreview checker, scorer); thread the product key via
  context.Context from the gate session into the agent call path; record usage events to Firestore
  `usage` {key, agent, model, in_tokens, out_tokens, ts}.
- Price table (per-model $/1M in/out): gemini-2.5-pro, 3.5-flash, 3.1-pro-preview.
- Surface: `kaimi-key cost <key>` (sum + breakdown) and `kaimi-key list` shows spend-to-date.

### P4 — Per-tester provisioning (Terraform)
- Terraform variant for an OWN-PROJECT deploy under BlueMeta billing: project (create/reference),
  enable Firestore + Vertex + Run + Secret Manager APIs, runtime SA + least-priv IAM (incl. Firestore
  datastore user), **per-project budget alert** (threshold email, optional cap), product-key gate
  config (gate=product-key, Workspace sign-in OFF, Drive OAuth ON), BlueMeta SAM/Vertex secrets.
- `terraform apply` into a clean project yields a running, product-key-gated Kaimi.

### P5 — Provision Ey3 + go-live
- New project (e.g. `ey3-kaimi`) under BlueMeta billing; `terraform apply`; budget alert set.
- `kaimi-key mint --tester "Ey3" --days 14` → the key. Connect/seed nothing (production).
- Fill `docs/TESTER_ONBOARDING.md` placeholders (URL, key, feedback channel); send to Ey3.
- Dry-run the tester flow end-to-end (key → onboarding → hunt → draft → gate → Doc in their Drive).

## Rules
Access control + billing are security/cost-sensitive: strong independent review + Malik verification
on P2/P4 before any tester traffic; never leave the gate open; budget alert MUST exist before a tester
gets a key. Per-customer isolation preserved (no multi-tenancy). ESCALATE design/security ambiguity to Malik.

## Progress log
- 2026-06-17: Goal created from Malik's decisions (own-project-per-tester, product-key gate, Firestore, per-project Billing + budget alert + per-key token metering). RELEASE 002 (parity) ready to cut.
- 2026-06-17: **P0 done** — RELEASE 002 cut (tags + GitHub releases `release-001`/`release-002`), `bm` dogfood deployment redeployed live (Cloud Run rev `00012-x6b`, image `4de708f`). Phase-3 upstream port complete.
- 2026-06-17: **P1 done** — `internal/productkey` (KAIMI-XXXX-XXXX-XXXX, crypto/rand + rejection sampling, `Registry` Mint/Lookup/Revoke/List, `Record.Valid`, `Normalize`; Memory + Firestore impls) and `cmd/kaimi-key` admin CLI (mint/revoke/list, magic-link printing). TDD; gate green (build/test/lint/fmt); independent review (Gemma) flagged + fixed modulo bias. Merged via PR #91 → `main` @ `0e25a60`. `cost` deferred to P3.
