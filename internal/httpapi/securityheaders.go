package httpapi

import "net/http"

// SecurityHeaders wraps a handler to set a baseline of HTTP security response headers on
// every response, following the OWASP Secure Headers Project recommendations. It is applied
// at the root of the handler chain so it covers public routes (/healthz, /auth, /access,
// /entry) as well as the gated app.
//
// Notes on the chosen values:
//   - HSTS: 2 years + includeSubDomains. Cloud Run serves HTTPS only; browsers ignore HSTS
//     over plain HTTP, so this is safe to send unconditionally. "preload" is intentionally
//     omitted (preload is a hard-to-reverse commitment for the apex domain).
//   - Clickjacking: X-Frame-Options: DENY plus CSP frame-ancestors 'none' (the modern
//     equivalent) — the app is never meant to be embedded in a frame.
//   - CSP allows 'unsafe-inline' for style/script because the server-rendered pages use
//     inline <style> and a small amount of inline wizard JS. html/template contextual
//     auto-escaping remains the primary XSS defense; the CSP adds defense-in-depth
//     (locks origins, blocks objects/base-uri hijack, forbids framing) without breaking
//     the existing UI.
//   - Referrer-Policy: strict-origin-when-cross-origin so full paths/query (which could
//     carry an access link) never leak to third-party hosts via the Referer header.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; base-uri 'self'; frame-ancestors 'none'; " +
		"object-src 'none'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
		"script-src 'self' 'unsafe-inline'; connect-src 'self'; form-action 'self'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}
