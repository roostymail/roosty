// Package settings holds instance configuration. Values come from the admin
// panel (stored in SQLite) and can be overridden by environment variables;
// overridden fields are reported as locked so the panel shows them read-only.
package settings

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/roostymail/roosty/server/internal/store"
)

type Endpoint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"` // tls | starttls | none
}

type MailServer struct {
	IMAP          Endpoint `json:"imap"`
	SMTP          Endpoint `json:"smtp"`
	SkipTLSVerify bool     `json:"skipTlsVerify"`
}

type Access struct {
	Mode     string   `json:"mode"` // domains | list
	Domains  []string `json:"domains"`
	Accounts []string `json:"accounts"`
}

type Links struct {
	Support string `json:"support"`
	Privacy string `json:"privacy"`
}

type Branding struct {
	Name         string   `json:"name"`
	Accent       string   `json:"accent"`
	DefaultTheme string   `json:"defaultTheme"`
	Themes       []string `json:"themes"`
	LoginTitle   string   `json:"loginTitle"`
	LoginMessage string   `json:"loginMessage"`
	Links        Links    `json:"links"`
	CustomCSS    string   `json:"customCss"`
	Logos        []string `json:"logos"` // uploaded kinds: light, dark, icon
}

type Settings struct {
	Mail     MailServer `json:"mail"`
	Access   Access     `json:"access"`
	Branding Branding   `json:"branding"`
}

var AllThemes = []string{"claro", "grafite", "preto", "navy", "roxo", "bege", "sepia", "contraste", "floresta", "nevoa"}

func Defaults() Settings {
	return Settings{
		Mail:   MailServer{IMAP: Endpoint{Port: 993, Security: "tls"}, SMTP: Endpoint{Port: 465, Security: "tls"}},
		Access: Access{Mode: "domains"},
		Branding: Branding{
			Name: "Roosty Mail", Accent: "", DefaultTheme: "claro", Themes: AllThemes,
			LoginTitle: "Entrar no Roosty Mail", LoginMessage: "Use seu endereço de email completo e a senha da sua caixa.",
		},
	}
}

// Manager loads settings from the store and applies environment overrides.
type Manager struct {
	st     *store.Store
	mu     sync.RWMutex
	cur    Settings
	locked map[string]bool
}

func NewManager(st *store.Store) (*Manager, error) {
	m := &Manager{st: st}
	return m, m.Reload()
}

func (m *Manager) Reload() error {
	s := Defaults()
	if err := m.st.GetJSON("settings", &s); err != nil && err != store.ErrNotFound {
		return err
	}
	locked := applyEnv(&s)
	if err := s.Validate(); err != nil {
		return fmt.Errorf("configuração inválida (banco ou variáveis de ambiente): %w", err)
	}
	m.mu.Lock()
	m.cur, m.locked = s, locked
	m.mu.Unlock()
	return nil
}

// Get returns a deep copy, so callers can never modify the live settings.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c := m.cur
	c.Access.Domains = slices.Clone(c.Access.Domains)
	c.Access.Accounts = slices.Clone(c.Access.Accounts)
	c.Branding.Themes = slices.Clone(c.Branding.Themes)
	c.Branding.Logos = slices.Clone(c.Branding.Logos)
	return c
}

func (m *Manager) Locked() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]bool, len(m.locked))
	for k, v := range m.locked {
		out[k] = v
	}
	return out
}

// Save stores the given settings (locked fields are ignored, since env wins).
func (m *Manager) Save(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := m.st.SetJSON("settings", s); err != nil {
		return err
	}
	return m.Reload()
}

// Configured reports whether a mail server is known.
func (m *Manager) Configured() bool {
	s := m.Get()
	return s.Mail.IMAP.Host != "" && s.Mail.SMTP.Host != ""
}

// Allowed checks whether an address may sign in under the access rules.
func (s Settings) Allowed(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return false
	}
	switch s.Access.Mode {
	case "list":
		for _, a := range s.Access.Accounts {
			if strings.EqualFold(strings.TrimSpace(a), email) {
				return true
			}
		}
		return false
	default:
		if len(s.Access.Domains) == 0 {
			return true
		}
		domain := email[at+1:]
		for _, d := range s.Access.Domains {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				return true
			}
		}
		return false
	}
}

func applyEnv(s *Settings) map[string]bool {
	l := map[string]bool{}
	str := func(key, field string, dst *string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			*dst, l[field] = v, true
		}
	}
	num := func(key, field string, dst *int) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst, l[field] = n, true
			}
		}
	}
	list := func(key, field string, dst *[]string) {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			var out []string
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, strings.ToLower(p))
				}
			}
			*dst, l[field] = out, true
		}
	}
	str("ROOSTY_IMAP_HOST", "mail.imap.host", &s.Mail.IMAP.Host)
	num("ROOSTY_IMAP_PORT", "mail.imap.port", &s.Mail.IMAP.Port)
	str("ROOSTY_IMAP_SECURITY", "mail.imap.security", &s.Mail.IMAP.Security)
	str("ROOSTY_SMTP_HOST", "mail.smtp.host", &s.Mail.SMTP.Host)
	num("ROOSTY_SMTP_PORT", "mail.smtp.port", &s.Mail.SMTP.Port)
	str("ROOSTY_SMTP_SECURITY", "mail.smtp.security", &s.Mail.SMTP.Security)
	if v := os.Getenv("ROOSTY_TLS_SKIP_VERIFY"); v == "true" || v == "1" {
		s.Mail.SkipTLSVerify, l["mail.skipTlsVerify"] = true, true
	}
	str("ROOSTY_ACCESS_MODE", "access.mode", &s.Access.Mode)
	list("ROOSTY_ALLOWED_DOMAINS", "access.domains", &s.Access.Domains)
	list("ROOSTY_ALLOWED_ACCOUNTS", "access.accounts", &s.Access.Accounts)
	str("ROOSTY_BRAND_NAME", "branding.name", &s.Branding.Name)
	str("ROOSTY_BRAND_ACCENT", "branding.accent", &s.Branding.Accent)
	str("ROOSTY_DEFAULT_THEME", "branding.defaultTheme", &s.Branding.DefaultTheme)
	return l
}
