# GOAL: Security hardening pass (pre-pilot)

**Owner:** Malik + agent · **Created:** 2026-06-26 · **Repo:** Mawar2/kaimi-platform
**Branch:** feat/security-hardening (off main) · **Driven by:** /loop

## Objective
Close the MEDIUM hardening findings from the pre-pilot QA, anchored to standard practice
(OWASP Secure Headers; minimal runtime deps per CONVENTIONS), so the Ey3 pilot ships with a
defensible, repeatable security posture. No behavior change for the tester beyond safer
defaults. The UI agent owns the flow/copy gaps separately — do NOT edit onboarding wizard
templates here (avoid collisions); coordinate on shared files.

## Tasks
1. **Security headers middleware** (OWASP Secure Headers) — `internal/httpapi`: HSTS,
   `X-Frame-Options: DENY` + CSP `frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`,
   `Referrer-Policy: strict-origin-when-cross-origin`, a conservative CSP (allow inline
   style/script so the server-rendered UI isn't broken). Applied to every response. TDD.
2. **SAM resolver host-allowlist** — `internal/samgov/noticedesc.go`: only attach the SAM
   api_key when the URL host is `api.sam.gov` over https; re-validate host on redirects.
   Prevents the key ever reaching a non-SAM host (SSRF/credential-leak). TDD.
3. **Session cookie — stop exposing the product key** — `internal/httpapi/session.go`:
   encrypt the cookie payload (AES-GCM, key derived from SESSION_SECRET) so the embedded
   `kid` (product key) is no longer base64-readable from a captured cookie. Contained to
   session encode/decode (sign-then-... keep verify path). Existing sessions invalidate on
   deploy (re-login) — acceptable. TDD round-trip + tamper + wrong-key.
4. **CI security scanners** — `.github/workflows/ci.yml`: add `govulncheck`
   (golang.org/x/vuln) + `gosec` (securego/gosec, or via golangci-lint) as CI steps.

## Discipline
TDD (test first); go gate (build/test/lint/fmt; only cmd/desktop may fail); independent
security review (sub-agent) of the diff before merge; commit on feat/security-hardening;
human-authorized merge to main; deploy to the pilot (api image) + verify LIVE (headers
present, cookie payload no longer reveals the key, resolver rejects non-SAM host via test).
Never weaken the gate's fail-closed behavior or break the deployed onboarding/board.

## Progress log
- 2026-06-26: Goal created. Starting tasks 1 (headers) + 2 (resolver allowlist).
