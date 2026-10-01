package server

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/roostymail/roosty/server/internal/mail"
	"github.com/roostymail/roosty/server/internal/secure"
	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
)

const adminCookie = "roosty_admin"

type adminKey struct{}

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(adminCookie)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "entre como administrador")
			return
		}
		a, err := s.st.AdminSession(c.Value)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "sessão de administrador expirada")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminKey{}, a)))
	}
}

func (s *Server) startAdminSession(w http.ResponseWriter, r *http.Request, adminID int64) error {
	id := secure.Token(24)
	if err := s.st.CreateAdminSession(id, adminID, 8*time.Hour); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: id, Path: "/", HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	return nil
}

// ---------- branding (public) ----------

type publicBranding struct {
	settings.Branding
	Version    string `json:"version"`
	SetupNeeded bool  `json:"setupNeeded"`
}

func (s *Server) handleBranding(w http.ResponseWriter, r *http.Request) {
	b := s.cfg.Get().Branding
	if len(b.Themes) == 0 {
		b.Themes = settings.AllThemes
	}
	writeJSON(w, http.StatusOK, publicBranding{Branding: b, Version: s.version, SetupNeeded: s.setupToken != ""})
}

var logoKinds = map[string]bool{"light": true, "dark": true, "icon": true}

func (s *Server) logoPath(kind string) string { return filepath.Join(s.dataDir, "branding", "logo-"+kind) }

func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !logoKinds[kind] {
		http.NotFound(w, r)
		return
	}
	data, err := os.ReadFile(s.logoPath(kind))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct, _ := os.ReadFile(s.logoPath(kind) + ".type")
	w.Header().Set("Content-Type", string(ct))
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	_, _ = w.Write(data)
}

// ---------- first-run setup ----------

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"needed": s.setupToken != "", "settings": s.cfg.Get(), "locked": s.cfg.Locked(),
	})
}

func (s *Server) checkSetupToken(w http.ResponseWriter, r *http.Request, token string) bool {
	if s.setupToken == "" {
		jsonError(w, http.StatusGone, "a configuração inicial já foi concluída")
		return false
	}
	if !s.limiter.allow("setup:"+clientIP(r), 15, 10*time.Minute) {
		jsonError(w, http.StatusTooManyRequests, "muitas tentativas")
		return false
	}
	if !secure.Equal(strings.TrimSpace(token), s.setupToken) {
		jsonError(w, http.StatusForbidden, "código de configuração incorreto. Veja o log do container.")
		return false
	}
	return true
}

func (s *Server) handleSetupTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string              `json:"token"`
		Mail  settings.MailServer `json:"mail"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	if !s.checkSetupToken(w, r, req.Token) {
		return
	}
	writeJSON(w, http.StatusOK, mail.Test(s.mergeLockedMail(req.Mail)))
}

// mergeLockedMail keeps environment-provided values over submitted ones.
func (s *Server) mergeLockedMail(m settings.MailServer) settings.MailServer {
	cur, l := s.cfg.Get().Mail, s.cfg.Locked()
	if l["mail.imap.host"] {
		m.IMAP.Host = cur.IMAP.Host
	}
	if l["mail.imap.port"] {
		m.IMAP.Port = cur.IMAP.Port
	}
	if l["mail.imap.security"] {
		m.IMAP.Security = cur.IMAP.Security
	}
	if l["mail.smtp.host"] {
		m.SMTP.Host = cur.SMTP.Host
	}
	if l["mail.smtp.port"] {
		m.SMTP.Port = cur.SMTP.Port
	}
	if l["mail.smtp.security"] {
		m.SMTP.Security = cur.SMTP.Security
	}
	if l["mail.skipTlsVerify"] {
		m.SkipTLSVerify = cur.SkipTLSVerify
	}
	return m
}

func (s *Server) handleSetupComplete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string            `json:"token"`
		Username string            `json:"username"`
		Password string            `json:"password"`
		Settings settings.Settings `json:"settings"`
	}
	req.Settings = settings.Defaults() // omitted fields keep safe defaults
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	if !s.checkSetupToken(w, r, req.Token) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Password) < 10 {
		jsonError(w, http.StatusBadRequest, "use um usuário com 3+ caracteres e uma senha com 10+ caracteres")
		return
	}
	req.Settings.Mail = s.mergeLockedMail(req.Settings.Mail)
	if err := req.Settings.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.Save(req.Settings); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar configurações")
		return
	}
	id, err := s.st.CreateAdmin(req.Username, secure.HashPassword(req.Password))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao criar administrador")
		return
	}
	s.setupToken = ""
	_ = s.startAdminSession(w, r, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- admin session ----------

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	if !s.limiter.allow("admin:"+clientIP(r), 10, 15*time.Minute) {
		jsonError(w, http.StatusTooManyRequests, "muitas tentativas. Aguarde 15 minutos.")
		return
	}
	a, err := s.st.AdminByUsername(strings.TrimSpace(req.Username))
	if err != nil || !secure.CheckPassword(a.PasswordHash, req.Password) {
		jsonError(w, http.StatusUnauthorized, "usuário ou senha incorretos")
		return
	}
	if err := s.startAdminSession(w, r, a.ID); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"username": a.Username})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(adminCookie); err == nil {
		_ = s.st.DeleteAdminSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	a := r.Context().Value(adminKey{}).(*store.Admin)
	writeJSON(w, http.StatusOK, map[string]string{"username": a.Username})
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	accounts, sessions := s.st.Counts()
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "sessions": sessions, "version": s.version, "configured": s.cfg.Configured()})
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.cfg.Get(), "locked": s.cfg.Locked(), "allThemes": settings.AllThemes})
}

func (s *Server) handleAdminSaveSettings(w http.ResponseWriter, r *http.Request) {
	in := s.cfg.Get() // partial updates keep current values
	if err := readJSON(r, &in); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Mail = s.mergeLockedMail(in.Mail)
	in.Branding.Logos = s.cfg.Get().Branding.Logos
	if err := in.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.Save(in); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.cfg.Get(), "locked": s.cfg.Locked()})
}

func (s *Server) handleAdminTest(w http.ResponseWriter, r *http.Request) {
	var m settings.MailServer
	if err := readJSON(r, &m); err != nil {
		jsonError(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	writeJSON(w, http.StatusOK, mail.Test(s.mergeLockedMail(m)))
}

func (s *Server) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.Accounts()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao listar contas")
		return
	}
	if list == nil {
		list = []store.Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"list": list})
}

func (s *Server) handleAdminAccountAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil || !settings.Email(strings.ToLower(strings.TrimSpace(req.Email))) {
		jsonError(w, http.StatusBadRequest, "endereço inválido")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	revoke := func() {
		ids, _ := s.st.DeleteSessionsFor(email)
		for _, id := range ids {
			s.pool.Drop(id)
		}
	}
	switch r.PathValue("action") {
	case "block":
		_ = s.st.SetBlocked(email, true)
		revoke()
	case "unblock":
		_ = s.st.SetBlocked(email, false)
	case "revoke":
		revoke()
	default:
		jsonError(w, http.StatusNotFound, "ação desconhecida")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

var logoTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/svg+xml": true}

func (s *Server) handleAdminLogoUpload(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !logoKinds[kind] {
		jsonError(w, http.StatusNotFound, "tipo de logo inválido")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "envie uma imagem de até 1 MB")
		return
	}
	defer f.Close()
	ct := hdr.Header.Get("Content-Type")
	if !logoTypes[ct] {
		jsonError(w, http.StatusBadRequest, "use PNG, JPG, WebP ou SVG")
		return
	}
	data, _ := io.ReadAll(io.LimitReader(f, 1<<20))
	_ = os.MkdirAll(filepath.Join(s.dataDir, "branding"), 0o700)
	if err := os.WriteFile(s.logoPath(kind), data, 0o600); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar")
		return
	}
	_ = os.WriteFile(s.logoPath(kind)+".type", []byte(ct), 0o600)
	s.setLogo(kind, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminLogoDelete(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	_ = os.Remove(s.logoPath(kind))
	s.setLogo(kind, false)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) setLogo(kind string, on bool) {
	cfg := s.cfg.Get()
	var out []string
	for _, k := range cfg.Branding.Logos {
		if k != kind {
			out = append(out, k)
		}
	}
	if on {
		out = append(out, kind)
	}
	cfg.Branding.Logos = out
	_ = s.cfg.Save(cfg)
}
