package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/roostymail/roosty/server/internal/mail"
	"github.com/roostymail/roosty/server/internal/secure"
	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
)

const (
	sessionCookie = "roosty_session"
	shortSession  = 12 * time.Hour
	longSession   = 30 * 24 * time.Hour
)

type userCtx struct {
	sessionID string
	creds     mail.Creds
}

type ctxKey struct{}

func current(r *http.Request) *userCtx { return r.Context().Value(ctxKey{}).(*userCtx) }

// user authenticates a request: the cookie holds the session ID and the key
// that decrypts the stored mailbox password.
func (s *Server) user(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.resolveSession(r)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "sessão expirada, entre novamente")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func (s *Server) resolveSession(r *http.Request) (*userCtx, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, errUnauthorized
	}
	id, keyStr, ok := strings.Cut(c.Value, ".")
	if !ok {
		return nil, errUnauthorized
	}
	sess, err := s.st.Session(id)
	if err != nil {
		return nil, errUnauthorized
	}
	key, err := secure.DecodeKey(keyStr)
	if err != nil || len(key) != 32 {
		return nil, errUnauthorized
	}
	pw, err := secure.Open(key, sess.Secret)
	if err != nil {
		return nil, errUnauthorized
	}
	if s.st.IsBlocked(sess.Email) {
		return nil, errUnauthorized
	}
	return &userCtx{sessionID: sess.ID, creds: mail.Creds{Email: sess.Email, Password: string(pw)}}, nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "dados inválidos")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	ip := clientIP(r)
	if !s.limiter.allow("ip:"+ip, 20, 10*time.Minute) || !s.limiter.allow("user:"+email, 8, 10*time.Minute) {
		jsonError(w, http.StatusTooManyRequests, "muitas tentativas. Aguarde alguns minutos e tente de novo.")
		return
	}
	if !s.cfg.Configured() {
		jsonError(w, http.StatusServiceUnavailable, "o servidor de email ainda não foi configurado pelo administrador")
		return
	}
	cfg := s.cfg.Get()
	if !cfg.Allowed(email) || s.st.IsBlocked(email) {
		jsonError(w, http.StatusForbidden, "este endereço não tem acesso a este webmail")
		return
	}
	c, err := mail.Login(cfg.Mail, mail.Creds{Email: email, Password: req.Password}, nil)
	if err != nil {
		if errors.Is(err, mail.ErrAuth) {
			jsonError(w, http.StatusUnauthorized, "email ou senha incorretos")
		} else {
			slog.Error("imap login", "err", err)
			jsonError(w, http.StatusBadGateway, "não foi possível conectar ao servidor de email")
		}
		return
	}
	_ = c.Logout().Wait()
	_ = c.Close()
	s.limiter.reset("user:" + email)

	key := secure.RandomBytes(32)
	sealed, err := secure.Seal(key, []byte(req.Password))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	ttl := shortSession
	if req.Remember {
		ttl = longSession
	}
	sess := store.Session{ID: secure.Token(18), Email: email, Secret: sealed, ExpiresAt: time.Now().Add(ttl).Unix()}
	if err := s.st.TouchAccount(email); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	if err := s.st.CreateSession(sess, r.UserAgent(), ip); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro interno")
		return
	}
	ck := &http.Cookie{Name: sessionCookie, Value: sess.ID + "." + secure.EncodeKey(key), Path: "/", HttpOnly: true,
		Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode}
	if req.Remember {
		ck.Expires = time.Now().Add(ttl)
	}
	http.SetCookie(w, ck)
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if u, err := s.resolveSession(r); err == nil {
		_ = s.st.DeleteSession(u.sessionID)
		s.pool.Drop(u.sessionID)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	writeJSON(w, http.StatusOK, map[string]any{"email": u.creds.Email, "prefs": s.loadPrefs(u.creds.Email)})
}

// loadPrefs reads stored preferences; anything that fails validation is
// discarded instead of being sent to the browser.
func (s *Server) loadPrefs(email string) settings.Prefs {
	raw, err := s.st.Prefs(email)
	if err != nil {
		return settings.Prefs{}
	}
	p, err := settings.ParsePrefs(bytes.NewReader(raw))
	if err != nil {
		slog.Warn("stored preferences failed validation, using defaults", "err", err)
		return settings.Prefs{}
	}
	return p
}

func (s *Server) savePrefs(email string, p settings.Prefs) error {
	if err := p.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.st.SetPrefs(email, raw)
}

func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	p, err := settings.ParsePrefs(r.Body)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.savePrefs(current(r).creds.Email, p); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
