package monitor

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist holds the built Monitor SPA bundle. The all: prefix ensures files Vite
// may emit with leading underscores or dots are included in the embed.
//
//go:embed all:web/dist
var dist embed.FS

// Handler returns an http.Handler that serves the embedded Monitor bundle.
//
// Static assets (anything under the bundle root that exists, e.g. /assets/*,
// favicon, JS/CSS) are served directly. Any other path that does not look like
// a file request falls back to index.html, so client-side routes deep-link
// correctly (SPA fallback).
func Handler() http.Handler {
	// Root the served filesystem at web/dist so URLs map to bundle paths
	// without the embed prefix.
	sub, err := fs.Sub(dist, "web/dist")
	if err != nil {
		// This only fails if the embedded path is wrong at build time, which
		// is a programmer error rather than a runtime condition.
		panic("monitor: embedded bundle missing web/dist: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Normalize the request path relative to the bundle root.
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		// Serve the file directly when it exists in the bundle.
		if f, openErr := sub.Open(name); openErr == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unknown, non-asset paths so the
		// client router can resolve them. Requests that explicitly target a
		// file (have an extension) get a normal 404 instead.
		if ext := path.Ext(name); ext != "" {
			http.NotFound(w, r)
			return
		}
		serveIndex(w, r, sub)
	})
}

// serveIndex writes the bundle's index.html as the SPA entry point.
func serveIndex(w http.ResponseWriter, _ *http.Request, fsys fs.FS) {
	data, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		http.Error(w, "monitor: index.html not found in bundle", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// index.html is the dynamic SPA shell; it must not be cached so clients
	// always pick up the latest hashed asset references after a redeploy.
	w.Header().Set("Cache-Control", "no-cache")
	// Ignore error: the client may close the connection mid-write; nothing to
	// recover and the status line is already sent.
	_, _ = w.Write(data)
}
