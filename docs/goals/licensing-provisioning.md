# Kaimi Licensing & Dynamic Provisioning Plan

**Last updated:** 2026-06-16
**Owner:** BlueMeta (Malik / Timm)
**Status:** Decision-ready plan (no code). Pilot = Ey3 Technologies.
**Scope:** How BlueMeta dynamically issues / "spins up" Kaimi **license keys** for the commercial product, given the per-customer isolated-deployment model.

> This is a planning document. It contains NO code and changes nothing in any repo. It assumes the existing per-customer Terraform module at `deploy/terraform/modules/kaimi` (Cloud Run Service `cmd/api` + Cloud Run Job pipeline + Secret Manager + Vertex AI), which already var-drives `tenant_id`, creates Secret Manager containers populated out-of-band, and uses an ADC-only runtime SA with no exported keys.

---

## 1. Problem & Constraints

Kaimi is sold as a **per-customer isolated stack**: each customer gets their own (or BlueMeta-managed) GCP project, own Workspace sign-in, own Drive. There is **no pooled multi-tenancy** — BlueMeta is itself a federal contractor competing with its customers, so cross-customer data sharing is a hard no.

A "license key" must:

- **Authorize** a given deployment to run at all (gate startup).
- **Bind** the deployment to a specific tenant/customer (anti-reuse).
- **Encode entitlement**: trial vs paid, seats/usage limits, expiry.
- **Support revocation** (kill a non-paying or breached tenant).
- Be **issued dynamically** — self-serve or one-click by BlueMeta — so spinning up a new customer is fast.
- Work for **both** future distribution channels: **GCP Marketplace** and **direct SaaS** (channel chosen after pilots).

Operating constraints that shape the design:

- **Solo/two-person team.** Whatever we build must be operable by one person and cheap to run. Bias to managed, scale-to-zero services.
- **Isolation is sacred.** The licensing control plane must NOT become a data-sharing backchannel. It only ever sees license metadata (tenant_id, plan, expiry, deploy fingerprint) — never opportunity/Drive/proposal data.
- **Fail behavior matters.** A customer deployment that can't reach the licensing service must not silently go dark mid-business-day, but a revoked/expired license must eventually stop working. This implies a **grace period**, not pure fail-closed and not pure fail-open.
- **No exfiltratable private key.** The repo already prides itself on ADC-only, no exported SA keys. The license-signing key must follow the same standard → **Cloud KMS asymmetric signing** (private key never leaves KMS).

---

## 2. Key Model — Three Options Compared

### (a) Offline-signed license token (KMS-signed JWT)

A JWT signed by a BlueMeta private key, carrying `tenant_id`, plan, seats, usage limits, `exp`. The customer app validates the signature **at startup** with the corresponding **public key** (baked into the image or fetched once). No phone-home required. Revocation = short `exp` + reissue (and an optional revocation list / CRL endpoint).

- **Pros:** Works fully offline (no runtime dependency on BlueMeta infra → no single point of failure for paying customers). Trivial to validate (verify signature + check claims). Fits the "isolated stack" ethos: the deploy is self-contained. Private key stays in KMS (never shipped). Cheap. Natural fit for air-gapped / FedRAMP-adjacent buyers later.
- **Cons:** Revocation is not instant — bounded by token lifetime. Can't meter usage centrally from the token alone (the deploy would have to report). A determined customer with a valid unexpired token can keep running until expiry even after non-payment.

### (b) Online license API (central validation + heartbeat)

A central BlueMeta service validates a key on every startup and on a periodic heartbeat. Supports instant revocation and central usage metering.

- **Pros:** Instant revocation. Real-time usage/seat metering. Central audit of which deploys are live. Can push entitlement changes (trial→paid) without redeploy.
- **Cons:** Introduces a **runtime dependency from every customer stack back to BlueMeta** — exactly the kind of coupling the isolation model wants to avoid, and an availability risk (if the license service is down, customers are at risk of going dark unless grace logic is careful). More to operate. Slight optics problem: a competitor-vendor's central service "watching" each isolated deploy. Heartbeats still carry only license metadata, but the coupling is real.

### (c) GCP Marketplace Procurement API / entitlements

GCP handles purchase → entitlement → billing for Marketplace SaaS. BlueMeta receives entitlement lifecycle events (ENTITLEMENT_ACTIVE, PLAN_CHANGE, CANCELLED) via Pub/Sub and reflects them into its own license state. Google is the system of record for the commercial relationship.

- **Pros:** Google handles billing, tax, contracts, and (with auto-approval) the purchase flow. Buyers can put Kaimi on their existing GCP bill / burn committed spend. Entitlement state is authoritative and event-driven. Strong go-to-market lever for federal/enterprise buyers already on GCP.
- **Cons:** Marketplace approval is a process (Build Partner status; tiering as of Q1 2026; reviews up to ~2 weeks; pricing + technical-integration reviews gated in sequence). It governs *commerce*, not *runtime enforcement* — you **still need (a) or (b)** to actually gate each isolated deploy. Doesn't help direct-SaaS or pilot customers who aren't buying through Marketplace. Overkill for the Ey3 pilot.

### Comparison Table

| Dimension | (a) Offline KMS-signed JWT | (b) Online license API | (c) Marketplace Procurement API |
|---|---|---|---|
| Works without phoning home | Yes | No | N/A (commerce layer) |
| Instant revocation | No (bounded by `exp`) | Yes | Via entitlement event (commerce only) |
| Central usage metering | No (needs reporting) | Yes | Yes (Service Control usage reporting) |
| Fits isolated-stack ethos | Excellent | Weak (runtime coupling) | Neutral |
| Operating burden (2-person) | Low | Medium–High | Medium (approval + integration) |
| Handles billing/contracts | No | No | Yes (Google) |
| Good for Ey3 pilot now | Yes | Overkill | No (not listed yet) |
| Runtime availability risk | None | Yes (license svc must be up) | None at runtime |
| Cost | ~$0 | Cloud Run + DB | Google rev-share (~3%) |

**Read:** For the per-customer isolated model, **(a) is the spine**. (b) adds *optional, soft* online checks for revocation/metering without making runtime depend on it. (c) is a *commerce front-end* that feeds entitlements into the same license state — additive, not a replacement.

---

## 3. Architecture — Central "BlueMeta Licensing" Project

A **dedicated GCP project** (e.g. `bluemeta-licensing`) that is SEPARATE from every customer deploy. It is the control plane; it never touches customer data planes.

```
                         BlueMeta Licensing Project (bluemeta-licensing)
                         ---------------------------------------------------
                         |                                                 |
  Admin (Malik) -------> |  License Issuing Service (Cloud Run, private)    |
  one-click / CLI        |    - POST /licenses  (create/reissue)           |
                         |    - POST /licenses/{id}/revoke                 |
                         |    - GET  /licenses/{id}/status  (soft online)  |
                         |    - GET  /.well-known/kaimi-license.pub        |  <-- public key + CRL
                         |          |            |                |        |
                         |          v            v                v        |
                         |     Cloud KMS    Firestore        Secret Mgr    |
                         |   (asym sign,   (license records, (SAM-style    |
                         |    EC P-256,     audit, CRL)      issuer config) |
                         |    priv key                                     |
                         |    never leaves)                                |
                         |                                                 |
   GCP Marketplace ----> |  Procurement webhook (Pub/Sub push -> Cloud Run)|
   (Pub/Sub entitlement) |    maps entitlement events -> Firestore license |
                         ---------------------------------------------------
                                                 |
              token (JWT) handed to provisioning  |  public key distributed with image
                                                 v
   ============================ Per-Customer Project (isolated) ============================
   |  Terraform module deploy/terraform/modules/kaimi                                       |
   |    var "kaimi_license_token" --> Secret Manager secret "kaimi-license" (added         |
   |        out-of-band, same pattern as samgov-api-key)                                    |
   |    Cloud Run Service (cmd/api) + Job (pipeline) read KAIMI_LICENSE at boot,            |
   |        verify signature w/ embedded public key, check tenant_id/plan/exp,             |
   |        optional soft online check against licensing /status (grace window)            |
   =========================================================================================
```

### Component choices

- **Issuing/validation service:** **Cloud Run** (one small private service). Cloud Functions would also work, but Cloud Run matches the existing stack, scales to zero, and keeps one deployment idiom. Locked down: no `allUsers`; admin endpoints behind IAM/IAP; the public-key + CRL endpoint is the only unauthenticated route (it's public by design).
- **License datastore:** **Firestore (Native mode)**. Rationale: serverless, scale-to-zero, no instance to babysit, generous free tier, simple document model (one doc per license keyed by `license_id`/`tenant_id`), trivial audit subcollection. **Cloud SQL is rejected** for the pilot — it means an always-on instance (cost + ops) for a tiny dataset (tens of licenses). Revisit only if relational reporting becomes a real need.
- **Signing keys:** **Cloud KMS asymmetric signing key**, `EC_SIGN_P256_SHA256` (software protection level is fine and cheap; HSM optional later for high-assurance buyers). The **private key never leaves KMS** — the service calls `AsymmetricSign`; it never holds key material, mirroring the repo's ADC-only / no-exported-key standard. The corresponding **public key** is exported once and (i) baked into the Kaimi container image and/or (ii) served at `/.well-known/kaimi-license.pub` for rotation.
- **Secret Manager (licensing project):** holds issuer config / any third-party tokens for the licensing service itself. Customer license *tokens* are NOT secrets in the cryptographic sense (signature integrity, not confidentiality), but we still inject them via Secret Manager on the customer side to reuse the existing secret-handling pattern and keep them out of Terraform state.

### How a customer deploy obtains & validates its key

1. BlueMeta issues a token (admin one-click or Marketplace webhook) → token string returned.
2. Provisioning injects it: Terraform creates an empty `kaimi-license` Secret Manager container (same pattern as `samgov-api-key`); the operator runs `gcloud secrets versions add kaimi-license --data-file=-` with the token. (Or, for self-serve, the issuing service writes the secret version directly into the customer project — see §4.) The token value never enters tfvars or state.
3. Cloud Run injects it as `KAIMI_LICENSE` env (secret_key_ref, `version=latest`), exactly like `SAM_API_KEY`.
4. At boot the Go app: verifies the JWT signature against the embedded public key → checks `tenant_id` matches `TENANT_ID` → checks `exp` not passed → checks plan/limits → starts. On failure it refuses to serve (fail-closed at startup) with a clear log line.
5. Optionally, at a low cadence (e.g. daily), the app does a **soft online check** against the licensing `/status` endpoint to learn of revocation. If the check is *unreachable*, it keeps running within a **grace window** (see §3 fail behavior). If the check *succeeds and says revoked*, it stops.

### Fail behavior (explicit)

- **Startup, no/invalid/expired token → fail-closed.** Don't start. This is the hard gate.
- **Runtime soft online check unreachable → fail-open within grace.** Keep running until `exp` or until grace expires (e.g. token carries `exp` = paid-through + grace days; online check can only *shorten* life via revocation, never extend beyond `exp`). This protects paying customers from BlueMeta outages while guaranteeing eventual stop.
- **Runtime online check returns revoked → fail-closed** at next boot / next check.

This is the **hybrid**: offline JWT is authoritative for "can I run," online check is an *optional accelerator* for revocation.

---

## 4. Integration With Existing Terraform + Onboarding

The licensing layer is a thin addition to the proven module pattern; it deliberately reuses the `samgov-api-key` mechanics.

**Terraform (per-customer module):**
- Add one Secret Manager container `${prefix}kaimi-license` to the existing `secret_ids` set (same `auto{}` replication, same placeholder version, same `ignore_changes = [secret_data]` so the real token is never reverted nor stored in state).
- Add `KAIMI_LICENSE` env (secret_key_ref → `kaimi-license:latest`) to **both** the Cloud Run Service and Job, next to `SAM_API_KEY`.
- Optionally add a non-secret `var "license_public_key_url"` (default = embedded key) so rotation can point a deploy at `/.well-known/kaimi-license.pub` without rebuilding the image.
- No new IAM, no new always-on resource. Net new infra per customer ≈ one secret container.

**Two provisioning flows (both supported, same end-state):**
- **Operator-injected (pilot default):** BlueMeta runs the issuing service to mint a token, then `gcloud secrets versions add kaimi-license`. Matches today's out-of-band secret flow exactly. Zero new privileges granted to the licensing service.
- **Self-serve / one-click (later):** the issuing service, granted scoped `secretmanager.secretVersionAdder` on the target customer project only, writes the token version directly after mint. Faster spin-up; costs a cross-project grant, so gate behind the isolation review.

**Go app validation (where it lives):**
- A small startup check in the API service (`cmd/api`) and the pipeline (`cmd/pipeline`), or shared in `internal/config` / a new `internal/license` package: read `KAIMI_LICENSE`, verify signature with public key, assert `tenant_id == TENANT_ID`, check `exp` + plan/limits. Wire it as a hard precondition before the server/pipeline starts (fail-closed). Per CLAUDE.md anti-bloat: extend `internal/config` or add a single well-named `internal/license` package — no `utils.go`. Requires an approved ticket + TDD (offline fixture tokens for unit tests; a live KMS-signed token for E2E).

**Binding to tenant_id:** the token's `tenant_id` claim MUST equal the deploy's `TENANT_ID`. This is the anti-reuse mechanism: a token minted for Ey3 won't validate in a deploy configured for another tenant. Optionally also bind `aud` to the customer's GCP project number (`project_id`) for a second factor — a token can then only run in the intended project.

---

## 5. License Lifecycle

- **Dynamic generation:** Admin console (thin web UI on the issuing service) or `POST /licenses` API → service builds claims, calls KMS `AsymmetricSign`, writes the Firestore record (license_id, tenant_id, plan, seats, limits, issued_at, exp, status=active), returns the token. One-click "spin up Ey3" = fill tenant_id + plan + expiry → get token → inject.
- **Trial → paid:** trial = short `exp` (e.g. 30 days) + `plan=trial`. Upgrade = **reissue** a new token with `plan=paid` and a longer `exp`; push the new secret version (new "latest" → next boot picks it up). No redeploy needed. Marketplace path: a PLAN_CHANGE entitlement event triggers the same reissue.
- **Seat / usage limits:** encoded as claims (`seats`, `monthly_opp_limit`, etc.). Enforced **in-app** (the deploy knows its own usage). For *central* metering (Marketplace metered billing or SaaS overage), the deploy reports usage to the licensing service / Service Control — optional, phase 3+.
- **Expiry:** every token has `exp`. Paid tokens are paid-through + grace. Renewal = reissue + push new version (scriptable; later, auto-reissue on payment).
- **Rotation (signing key):** create a new KMS key version, publish both old+new public keys at `/.well-known/kaimi-license.pub` (a key set), sign new tokens with the new version, retire the old once all tokens minted under it have expired. Deploys that fetch the key URL pick up rotation automatically; image-embedded deploys get it at next image bump.
- **Revocation:** set Firestore `status=revoked`; add to CRL served at the public endpoint. Effect lands at the next soft online check or next boot. For instant kill of a hosted-by-BlueMeta deploy, also flip Terraform `active=false` (pauses the scheduler) and/or remove the secret version — but the license-level revoke is the channel-agnostic mechanism.
- **Audit:** every issue/reissue/revoke writes an immutable Firestore audit entry (who, when, tenant, plan, exp). Cloud Audit Logs cover KMS sign operations and Secret Manager access on top.

---

## 6. Marketplace Path vs Direct SaaS — One Design for Both

The runtime gate (KMS-signed JWT + optional soft online check) is **identical** regardless of channel. Only the *source of entitlement truth* differs:

- **Direct SaaS:** BlueMeta's admin/issuing service is the source of truth. Contract, invoice, and provisioning are manual/scripted. Best for pilots (Ey3) and customers not on GCP commerce.
- **GCP Marketplace:** Google is the commerce source of truth. BlueMeta completes the Cloud Marketplace Project Info Form → Producer Portal → integrates the **Commerce Procurement API** + **Service Control** (for metered usage). Entitlement lifecycle events arrive via **Pub/Sub** (ENTITLEMENT_ACTIVE / PLAN_CHANGE / CANCELLED; with auto-approval the CREATION_REQUESTED/PLAN_CHANGE_REQUESTED steps are skipped). A **Pub/Sub push → Cloud Run** webhook in the licensing project maps each event to the *same* Firestore license record and triggers the *same* mint/reissue/revoke. Note Marketplace prerequisites: Build Partner status, the Q1-2026 Select/Premier/Diamond tiering (team certifications), and review cycles up to ~2 weeks — so start the application early but don't block the pilot on it.

**Conclusion:** Yes — one design serves both. Marketplace is a *front door* that writes into the same license state machine; it never changes how a deploy enforces its license. Build direct-SaaS issuing first; add the Procurement webhook as an alternate event source later.

---

## 7. Security & Threat Model

| Threat | Mitigation |
|---|---|
| **Token tampering** (edit plan/exp) | JWT signature verified against KMS public key; any edit invalidates it. |
| **Forging a token** | Requires the private key, which **never leaves Cloud KMS** (no exported material, mirroring the repo's no-SA-key standard). KMS IAM tightly scoped to the issuing service SA. |
| **Token reuse across tenants** (copy Ey3's token to another deploy) | `tenant_id` claim must equal `TENANT_ID`; optional `aud=project_number` binds it to one GCP project. Reuse fails validation. |
| **Token theft from a deploy** | Stored in Secret Manager (not in code/state/tfvars), access-logged. Stolen token only works in a deploy whose `TENANT_ID`/project matches the claims; bind tightly to shrink blast radius. Revoke + reissue if leaked. |
| **Offline crack / brute force** | EC P-256 signatures; no feasible offline forgery without the private key. No shared secret to brute-force. |
| **Running past non-payment** (offline weakness of model (a)) | Short `exp` on trials; paid tokens carry bounded paid-through+grace; soft online check accelerates revocation. Worst case the customer runs until `exp`, then fail-closed. |
| **Licensing service outage causing customer downtime** | Runtime never *requires* the online check; grace window keeps paying customers alive during BlueMeta outages. |
| **Control plane as data backchannel** | Licensing service is in a separate project, sees only license metadata, and has no path to customer data planes. Enforced by IAM + project isolation; documented as an invariant. |
| **Privilege creep from self-serve injection** | If/when the service writes secret versions cross-project, grant only `secretVersionAdder` on the specific target project, time-boxed, audited. Pilot avoids this entirely (operator-injected). |

---

## 8. Recommendation

Adopt the **hybrid, Marketplace-ready** design:

1. **KMS-signed offline JWT** carrying `tenant_id` + plan + seats/limits + `exp` (+ optional `aud=project_number`) is the **authoritative runtime gate**. Private key lives in Cloud KMS (EC P-256), never exported. Public key embedded in the image and served at a `/.well-known` endpoint for rotation.
2. **Optional, soft periodic online check** against a central **BlueMeta licensing Cloud Run service** (separate project, Firestore-backed) for **revocation + audit** — fail-open within a grace window, never a hard runtime dependency.
3. **Marketplace Procurement API** added later as an **alternate entitlement source** that writes into the same Firestore license state machine via a Pub/Sub webhook — same runtime enforcement, Google handles commerce.

This honors the isolation model (self-contained deploys), the team's "no exfiltratable key" standard, the cheap-to-idle cost posture, and keeps both channels (direct + Marketplace) on one design.

### Phased Build Plan

**Phase 1 — Ey3 pilot MVP (offline-only, fastest path):**
- Create `bluemeta-licensing` project; create the KMS EC_SIGN_P256 key; export the public key.
- Write a tiny issuing CLI/Cloud Run endpoint: input tenant_id+plan+exp → KMS-sign → return token (Firestore record + audit). No online check, no Marketplace yet.
- Terraform: add `kaimi-license` secret container + `KAIMI_LICENSE` env to Service & Job (one ticket, TDD).
- Go: `internal/license` startup verifier (signature + tenant_id + exp), fail-closed (one ticket, TDD with offline fixture tokens + one E2E live-signed token).
- Provision Ey3 operator-injected (mint → `gcloud secrets versions add`).
- **Outcome:** Ey3 runs only with a valid BlueMeta-issued token bound to its tenant; trials expire automatically.

**Phase 2 — Revocation + admin UX:**
- Add `/status` + CRL endpoints; add the daily soft online check + grace window in the app.
- Add a thin admin console (one-click issue/reissue/revoke) over the issuing service (IAP-protected).
- Add `/.well-known/kaimi-license.pub` + key-set rotation support.
- **Outcome:** BlueMeta can revoke a tenant and roll keys without redeploys.

**Phase 3 — Self-serve + Marketplace-ready:**
- Optional cross-project secret-version injection for one-click spin-up (scoped grant).
- Begin GCP Marketplace onboarding (Project Info Form → Producer Portal → Build Partner status); implement the Procurement Pub/Sub webhook mapping entitlements → Firestore; wire Service Control for any metered plans.
- **Outcome:** New customers can be spun up in minutes; Marketplace becomes a second front door on the same enforcement core.

### Rough GCP Cost (control plane, monthly)

For a pilot/early-commercial scale (tens of licenses, low traffic), the licensing control plane is **effectively free-tier to low-single-digit dollars**:
- **Cloud KMS:** asymmetric software key ~$0.06/key/month + signing at ~$0.03 / 10,000 ops. Tokens are minted rarely (issue/reissue), so signing cost is negligible (cents).
- **Firestore (Native):** tens of license docs + audit entries → well within free tier; pennies at most.
- **Cloud Run (issuing service):** scales to zero; admin/webhook traffic is tiny → effectively free-tier.
- **Secret Manager:** a handful of secrets/access → negligible.
- **Total:** ~$1–5/month at pilot scale. Marketplace adds Google's revenue share (~3%) on transactions, not infra cost.

---

## Sources

- [Integrate your SaaS solution with the GCP Marketplace API (Producer Portal) — Google for Developers](https://developers.google.com/codelabs/gcp-marketplace-saas)
- [Manage entitlements for private offers — GCP Marketplace Partners](https://docs.cloud.google.com/marketplace/docs/partners/offers/manage-entitlements)
- [Turn on automatic offer approval for SaaS products — GCP Marketplace Partners](https://docs.cloud.google.com/marketplace/docs/partners/offers/automatic-approval)
- [Configure your app's backend (Procurement API) — GCP Marketplace Partners](https://cloud.google.com/marketplace/docs/partners/integrated-saas/backend-integration)
- [Setting up your SaaS product for Google Cloud — GCP Marketplace Partners](https://docs.cloud.google.com/marketplace/docs/partners/integrated-saas/set-up-environment)
- [Cloud KMS key purposes and algorithms (EC_SIGN_P256_SHA256)](https://docs.cloud.google.com/kms/docs/algorithms)
- [Cloud Key Management Service pricing](https://cloud.google.com/kms/pricing)
- [Sign and verify data with Cloud KMS (Asymmetric) — codelab](https://codelabs.developers.google.com/codelabs/sign-and-verify-data-with-cloud-kms-asymmetric)
- [How to Set Up Asymmetric Keys for Digital Signing with Cloud KMS in GCP](https://oneuptime.com/blog/post/2026-02-17-how-to-set-up-asymmetric-keys-for-digital-signing-with-cloud-kms-in-gcp/view)
- [Firestore CMEK / Cloud KMS integration](https://firebase.google.com/docs/firestore/cmek)
- [The Complete Guide to Selling on Google Cloud Marketplace (2026) — Suger](https://www.suger.io/resources/guides/gcp-marketplace/)
- [A Complete Guide To GCP Marketplace (2026) — Clazar](https://clazar.io/guides/google-marketplace)
