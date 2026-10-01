package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// handleStatic serves the built web app and falls back to index.html so
// client-side routes work on reload.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := fs.Stat(s.web, p); err != nil || f.IsDir() {
		p = "index.html"
	}
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.web, p)
}
