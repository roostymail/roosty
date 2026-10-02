// Package server exposes Roosty's HTTP API and serves the web app.
package server

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/roostymail/roosty/server/internal/mail"
	"github.com/roostymail/roosty/server/internal/secure"
	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
)

type Server struct {
	st         *store.Store
	cfg        *settings.Manager
	pool       *mail.Pool
	secret     []byte
	dataDir    string
	web        fs.FS
	version    string
	setupToken string
	limiter    *limiter
	mux        *http.ServeMux
}

type Options struct {
	DataDir    string
	Web        fs.FS
	Version    string
	SetupToken string
}

func New(st *store.Store, cfg *settings.Manager, o Options) (*Server, error) {
	s := &Server{st: st, cfg: cfg, dataDir: o.DataDir, web: o.Web, version: o.Version, limiter: newLimiter()}
	var secret string
	if err := st.GetJSON("secret", &secret); err != nil {
		secret = secure.Token(32)
		if err := st.SetJSON("secret", secret); err != nil {
			return nil, err
		}
	}
	s.secret = []byte(secret)
	s.pool = mail.NewPool(func() settings.MailServer { return cfg.Get().Mail })
	if n, _ := st.AdminCount(); n == 0 {
		s.setupToken = o.SetupToken
		if s.setupToken == "" {
			s.setupToken = secure.Token(9)
		}
		slog.Warn("first-run setup required: open /admin/setup and enter the setup token", "token", s.setupToken)
	}
	if err := os.MkdirAll(filepath.Join(s.dataDir, "uploads"), 0o700); err != nil {
		return nil, err
	}
	s.routes()
	go s.housekeeping()
	return s, nil
}

func (s *Server) housekeeping() {
	for range time.Tick(10 * time.Minute) {
		s.st.PurgeExpired()
		_ = filepath.WalkDir(filepath.Join(s.dataDir, "uploads"), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, err := d.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
					_ = os.Remove(p)
				}
			}
			return nil
		})
	}
}

func (s *Server) Handler() http.Handler { return s.secureHeaders(s.mux) }

func (s *Server) routes() {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	m.HandleFunc("GET /api/branding", s.handleBranding)
	m.HandleFunc("GET /branding/logo/{kind}", s.handleLogo)

	m.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	m.HandleFunc("POST /api/setup/test", s.handleSetupTest)
	m.HandleFunc("POST /api/setup/complete", s.handleSetupComplete)

	m.HandleFunc("POST /api/auth/login", s.handleLogin)
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("GET /api/me", s.user(s.handleMe))
	m.HandleFunc("PUT /api/prefs", s.user(s.handlePrefs))
	m.HandleFunc("POST /api", s.user(s.handleBatch))
	m.HandleFunc("GET /api/events", s.user(s.handleEvents))
	m.HandleFunc("GET /api/attachment", s.user(s.handleAttachment))
	m.HandleFunc("GET /api/inline", s.user(s.handleInline))
	m.HandleFunc("POST /api/upload", s.user(s.handleUpload))
	m.HandleFunc("GET /img/{sig}/{enc}", s.user(s.handleImageProxy))

	m.HandleFunc("POST /api/admin/login", s.handleAdminLogin)
	m.HandleFunc("POST /api/admin/logout", s.handleAdminLogout)
	m.HandleFunc("GET /api/admin/me", s.admin(s.handleAdminMe))
	m.HandleFunc("GET /api/admin/overview", s.admin(s.handleAdminOverview))
	m.HandleFunc("GET /api/admin/settings", s.admin(s.handleAdminSettings))
	m.HandleFunc("PUT /api/admin/settings", s.admin(s.handleAdminSaveSettings))
	m.HandleFunc("POST /api/admin/test", s.admin(s.handleAdminTest))
	m.HandleFunc("GET /api/admin/accounts", s.admin(s.handleAdminAccounts))
	m.HandleFunc("POST /api/admin/accounts/{action}", s.admin(s.handleAdminAccountAction))
	m.HandleFunc("POST /api/admin/logo/{kind}", s.admin(s.handleAdminLogoUpload))
	m.HandleFunc("DELETE /api/admin/logo/{kind}", s.admin(s.handleAdminLogoDelete))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { jsonError(w, http.StatusNotFound, "rota não encontrada") })
	m.HandleFunc("/", s.handleStatic)
	s.mux = m
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a request body strictly (see settings.DecodeStrict).
func readJSON(r *http.Request, v any) error {
	return settings.DecodeStrict(r.Body, 2<<20, v)
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if !strings.HasPrefix(r.URL.Path, "/img/") && !strings.HasPrefix(r.URL.Path, "/api/attachment") && !strings.HasPrefix(r.URL.Path, "/api/inline") {
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; font-src 'self'; connect-src 'self'; frame-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'self'; object-src 'none'")
		}
		// CSRF: state-changing API calls must carry a custom header, which
		// browsers never attach to cross-site requests without a preflight.
		isAPI := r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && isAPI && r.Header.Get("X-Roosty") != "1" {
			jsonError(w, http.StatusForbidden, "requisição sem cabeçalho de proteção")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------- rate limiting ----------

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

// allow returns false when key exceeded max attempts within window.
func (l *limiter) allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	list := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < window {
			list = append(list, t)
		}
	}
	if len(list) >= max {
		l.hits[key] = list
		return false
	}
	l.hits[key] = append(list, now)
	return true
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}

var errUnauthorized = errors.New("unauthorized")
