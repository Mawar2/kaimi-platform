package httpapi

import (
	"net/http"
	"strings"
)

// This file implements a small, configurable CORS middleware (WS-B6). It exists so
// the JSON API can be called by a Single-Page App served from a DIFFERENT origin
// (e.g. a Vercel/Cloud-Storage front end talking to the Cloud Run API). The
// preferred deployment is still SAME-ORIGIN — SPA and API behind one host — in which
// case no origins are configured and this middleware is a transparent no-op.
//
// Because the API authenticates with a session COOKIE, cross-origin requests must be
// "credentialed". The CORS spec FORBIDS combining Access-Control-Allow-Credentials:
// true with Access-Control-Allow-Origin: "*" — browsers reject it. So this
// middleware NEVER emits "*": it echoes back the request's specific Origin only when
// that origin is on the explicit allow-list, and otherwise emits no CORS headers at
// all (the request still proceeds, so same-origin and server-to-server callers are
// never blocked).

// corsAllowMethods is advertised on preflight responses. It covers the verbs the API
// actually serves (GET reads, POST select, OPTIONS preflight).
const corsAllowMethods = "GET, POST, OPTIONS"

// corsAllowHeaders is advertised on preflight responses: the request headers a
// browser SPA needs to send. Content-Type for JSON bodies; the cookie itself rides
// automatically with credentials and is not listed here.
const corsAllowHeaders = "Content-Type, Authorization"

// corsMaxAge lets browsers cache a successful preflight (seconds) to avoid an OPTIONS
// round-trip before every credentialed request.
const corsMaxAge = "600"

// CORS returns middleware that applies cross-origin resource sharing for the given
// allow-list of origins. It is meant to wrap the ROOT handler so it also covers the
// public /auth and /healthz routes and answers OPTIONS preflight before routing.
//
// Behavior:
//   - No origins configured (nil/empty, or only blank entries) → the returned
//     middleware is a no-op pass-through. This is the same-origin default.
//   - Request Origin matches an allowed origin → echo that SPECIFIC origin in
//     Access-Control-Allow-Origin (never "*") and set Allow-Credentials:true.
//   - An allowed-origin OPTIONS preflight → answered here with 204 and the
//     methods/headers/max-age advertised; the inner handler is not invoked.
//   - Request Origin absent or not allowed → no Allow-Origin/credentials headers are
//     added and the request is passed through unchanged (the inner handler/mux serves
//     it). This never blocks a request; it simply withholds the cross-origin grant.
//   - Whenever CORS is enabled (allow-list non-empty), EVERY handled request gets
//     Vary: Origin — set before (and regardless of) the origin check — so a shared
//     cache/CDN never serves a CORS-less cached response (one keyed for no Origin, or
//     for a different origin) back to the credentialed SPA. Only the no-op fast path
//     (no origins configured) adds nothing, because there is no CORS at all then.
//
// It does NOT authenticate; RequireSession remains the auth control. CORS only tells
// a browser whether a cross-origin response may be read by page JavaScript.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// Build a set of the configured origins once, at wrap time, normalizing each so
	// operator typos in config (trailing slash, mixed case, stray space) still match
	// the canonical Origin a browser sends. See normalizeOrigin.
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = normalizeOrigin(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		// No-op fast path: with nothing configured, return next untouched so the
		// same-origin deployment carries zero CORS behavior (no headers, no preflight
		// short-circuit, no Vary).
		if len(allowed) == 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CORS is enabled, so the response depends on the request's Origin even
			// when we add no grant (absent/disallowed origins still differ from the
			// allowed one). Advertise that to caches unconditionally, BEFORE the
			// origin check, using Add so we don't clobber any Vary set elsewhere.
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			_, ok := allowed[origin]

			// Only a recognized origin earns a CORS grant. An absent or unlisted
			// origin gets no Allow-Origin/credentials headers; the request still
			// flows to next (with Vary already added above).
			if origin != "" && ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")

				// Answer the preflight here: a browser sends OPTIONS with
				// Access-Control-Request-Method before a credentialed cross-origin
				// call. Respond 204 with the allowed methods/headers and stop — the
				// inner handler need not (and must not) see the preflight.
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
					w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
					w.Header().Set("Access-Control-Max-Age", corsMaxAge)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// normalizeOrigin canonicalizes a CONFIGURED origin so it matches the canonical
// Origin browsers actually send (lowercase scheme+host, no trailing slash). It trims
// surrounding space, strips a trailing slash, and lowercases. Normalizing the config
// side (not the incoming Origin) is what prevents operator-typo mismatches like
// "https://App.Example.com/" failing to match "https://app.example.com". A blank or
// all-space input normalizes to "" so callers can drop it from the allow-list.
func normalizeOrigin(o string) string {
	o = strings.TrimSpace(o)
	o = strings.TrimRight(o, "/")
	return strings.ToLower(o)
}

// parseCORSOrigins splits a comma-separated origins string (the CORS_ALLOWED_ORIGINS
// env value) into a clean slice: each entry trimmed of surrounding space, and empty
// entries (from a trailing comma or stray spaces) dropped so no "" sneaks into the
// allow-list. An empty or all-blank input yields a nil slice, which CORS treats as
// "disabled".
func parseCORSOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}
