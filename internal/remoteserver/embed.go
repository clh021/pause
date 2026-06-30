package remoteserver

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"pause/internal/logx"
)

//go:embed static
var embeddedStatic embed.FS

// staticFileHandler returns an http.Handler that serves the embedded static files
// (the built frontend dist/). It falls back to the "frontend/dist" directory on
// disk when the embedded files are not available (local development without the
// copy step).
func staticFileHandler() http.Handler {
	// Try embedded static files first.
	sub, err := fs.Sub(embeddedStatic, "static")
	if err == nil {
		// Check that the embedded FS actually has content (not just .gitkeep).
		if hasContent(sub) {
			return withSPAFallback(http.FileServer(http.FS(sub)))
		}
	}

	// Fall back to the frontend dist directory for local development.
	devPath := findFrontendDist()
	if devPath != "" {
		logx.Infof("static_file_handler using dev directory: %s", devPath)
		return withSPAFallback(http.FileServer(http.Dir(devPath)))
	}

	logx.Warnf("static_file_handler no static files available (run: npm --prefix frontend run build && cp -r frontend/dist/* internal/remoteserver/static/)")
	return http.NotFoundHandler()
}

// withSPAFallback wraps a file server handler so that any 404 (missing file)
// serves index.html instead, enabling SPA client-side routing.
func withSPAFallback(next http.Handler) http.Handler {
	fsrv := next
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a response recorder to capture 404s
		rec := &spaResponseWriter{ResponseWriter: w, status: http.StatusOK}
		fsrv.ServeHTTP(rec, r)
		if rec.status == http.StatusNotFound {
			// Rewind and serve index.html
			r.URL.Path = "/"
			fsrv.ServeHTTP(w, r)
		}
	})
}

// spaResponseWriter wraps http.ResponseWriter to capture the status code.
type spaResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *spaResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	if statusCode != http.StatusNotFound {
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

// hasContent returns true if the filesystem contains at least one entry other
// than ".gitkeep".
func hasContent(fsys fs.FS) bool {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}

// findFrontendDist searches for the frontend/dist directory relative to the
// current working directory and common project roots.
func findFrontendDist() string {
	candidates := []string{
		"frontend/dist",
		"../frontend/dist",
		filepath.Join(filepath.Dir(os.Args[0]), "frontend/dist"),
	}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(abs, "index.html")); err == nil {
				return abs
			}
		}
	}
	return ""
}
