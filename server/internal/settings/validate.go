package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Everything stored as JSON in the database goes through a typed struct and
// these checks first. Unknown fields, wrong types, oversized values, control
// characters and unsafe URLs or CSS are rejected, and only the canonical
// re-encoded value is saved.

var (
	reHostLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	reHex       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// CSS that can load external resources, run code or escape the <style> element.
	reBadCSS = regexp.MustCompile(`(?i)@import|url\s*\(|expression\s*\(|javascript:|vbscript:|behavior\s*:|-moz-binding|</?\s*style|<\s*script|\\[0-9a-f]{1,6}`)
)

// DecodeStrict parses JSON into v, refusing unknown fields, invalid UTF-8,
// trailing data and bodies above max bytes.
func DecodeStrict(r io.Reader, max int64, v any) error {
	raw, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return errors.New("corpo inválido")
	}
	if int64(len(raw)) > max {
		return errors.New("dados grandes demais")
	}
	if !utf8.Valid(raw) {
		return errors.New("texto com codificação inválida")
	}
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '{' {
		return errors.New("JSON inválido: esperado um objeto")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("JSON inválido: %s", cleanJSONError(err))
	}
	if dec.More() {
		return errors.New("JSON inválido: dados extras depois do objeto")
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("JSON inválido: dados extras depois do objeto")
	}
	return nil
}

func cleanJSONError(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Sprintf("campo %q com tipo errado", te.Field)
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "json: unknown field") {
		return "campo desconhecido " + strings.TrimPrefix(msg, "json: unknown field ")
	}
	return "formato incorreto"
}

// Text checks length (in characters) and refuses control and bidi override characters.
func Text(field, s string, max int, multiline bool) error {
	if utf8.RuneCountInString(s) > max {
		return fmt.Errorf("%s: no máximo %d caracteres", field, max)
	}
	for _, r := range s {
		if (r == '\n' || r == '\t') && multiline {
			continue
		}
		if unicode.IsControl(r) || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0xFEFF {
			return fmt.Errorf("%s: contém caracteres não permitidos", field)
		}
	}
	return nil
}

func Hostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	for _, l := range strings.Split(strings.TrimSuffix(s, "."), ".") {
		if !reHostLabel.MatchString(l) {
			return false
		}
	}
	return true
}

func Domain(s string) bool { return strings.Contains(s, ".") && net.ParseIP(s) == nil && Hostname(s) }

func Email(s string) bool {
	if len(s) > 254 || Text("email", s, 254, false) != nil {
		return false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return false
	}
	at := strings.LastIndex(s, "@")
	return at > 0 && Domain(s[at+1:])
}

func HexColor(s string) bool { return reHex.MatchString(s) }

// HTTPURL accepts only absolute http(s) URLs without credentials.
func HTTPURL(s string) bool {
	if len(s) > 2048 || Text("url", s, 2048, false) != nil {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && Hostname(u.Hostname())
}

func SafeCSS(s string) error {
	if err := Text("CSS da instância", s, 20000, true); err != nil {
		return err
	}
	if reBadCSS.MatchString(s) {
		return errors.New("CSS da instância: @import, url(), expression() e escapes não são permitidos")
	}
	return nil
}

func validTheme(id string) bool { return slices.Contains(AllThemes, id) }

func (e Endpoint) validate(name string) error {
	if e.Host == "" {
		return nil // not configured yet (fresh install)
	}
	if !Hostname(e.Host) {
		return fmt.Errorf("%s: servidor inválido", name)
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("%s: porta inválida", name)
	}
	if !slices.Contains([]string{"tls", "starttls", "none"}, e.Security) {
		return fmt.Errorf("%s: segurança deve ser tls, starttls ou none", name)
	}
	return nil
}

// Validate checks every field of the instance settings.
func (s Settings) Validate() error {
	if err := s.Mail.IMAP.validate("IMAP"); err != nil {
		return err
	}
	if err := s.Mail.SMTP.validate("SMTP"); err != nil {
		return err
	}
	if !slices.Contains([]string{"domains", "list"}, s.Access.Mode) {
		return errors.New("modo de acesso inválido")
	}
	if len(s.Access.Domains) > 500 || len(s.Access.Accounts) > 5000 {
		return errors.New("listas de acesso grandes demais")
	}
	for _, d := range s.Access.Domains {
		if !Domain(d) {
			return fmt.Errorf("domínio inválido: %q", d)
		}
	}
	for _, a := range s.Access.Accounts {
		if !Email(a) {
			return fmt.Errorf("endereço inválido: %q", a)
		}
	}
	b := s.Branding
	if err := Text("Nome da instância", b.Name, 80, false); err != nil {
		return err
	}
	if strings.ContainsAny(b.Name, "<>") {
		return errors.New("Nome da instância: não use < ou >")
	}
	if b.Accent != "" && !HexColor(b.Accent) {
		return errors.New("Cor de destaque: use o formato #RRGGBB")
	}
	if len(b.Themes) > len(AllThemes) {
		return errors.New("lista de temas inválida")
	}
	for i, t := range b.Themes {
		if !validTheme(t) || slices.Contains(b.Themes[:i], t) {
			return fmt.Errorf("tema inválido: %q", t)
		}
	}
	if !validTheme(b.DefaultTheme) || (len(b.Themes) > 0 && !slices.Contains(b.Themes, b.DefaultTheme)) {
		return errors.New("o tema padrão precisa estar entre os temas disponíveis")
	}
	if err := Text("Título da tela de login", b.LoginTitle, 120, false); err != nil {
		return err
	}
	if err := Text("Mensagem da tela de login", b.LoginMessage, 500, true); err != nil {
		return err
	}
	for name, u := range map[string]string{"Link de suporte": b.Links.Support, "Link de privacidade": b.Links.Privacy} {
		if u != "" && !HTTPURL(u) {
			return fmt.Errorf("%s: use um endereço http:// ou https://", name)
		}
	}
	if err := SafeCSS(b.CustomCSS); err != nil {
		return err
	}
	for i, l := range b.Logos {
		if !slices.Contains([]string{"light", "dark", "icon"}, l) || slices.Contains(b.Logos[:i], l) {
			return errors.New("lista de logos inválida")
		}
	}
	return nil
}

// ---------- user preferences ----------

// Prefs are a user's own settings, stored per account.
type Prefs struct {
	Theme          string   `json:"theme,omitempty"`
	Accent         string   `json:"accent,omitempty"`
	Density        string   `json:"density,omitempty"`
	FontSize       string   `json:"fontSize,omitempty"`
	DisplayName    string   `json:"displayName,omitempty"`
	Signature      string   `json:"signature,omitempty"`
	TrustedSenders []string `json:"trustedSenders,omitempty"`
}

const MaxPrefsBytes = 64 << 10

func (p Prefs) Validate() error {
	if p.Theme != "" && !validTheme(p.Theme) {
		return errors.New("tema inválido")
	}
	if p.Accent != "" && !HexColor(p.Accent) {
		return errors.New("cor de destaque inválida")
	}
	if p.Density != "" && !slices.Contains([]string{"comfortable", "compact"}, p.Density) {
		return errors.New("densidade inválida")
	}
	if p.FontSize != "" && !slices.Contains([]string{"small", "normal", "large"}, p.FontSize) {
		return errors.New("tamanho de texto inválido")
	}
	if err := Text("Nome de exibição", p.DisplayName, 100, false); err != nil {
		return err
	}
	if strings.ContainsAny(p.DisplayName, "<>\"") {
		return errors.New("Nome de exibição: não use < > ou aspas")
	}
	if err := Text("Assinatura", p.Signature, 2000, true); err != nil {
		return err
	}
	if len(p.TrustedSenders) > 1000 {
		return errors.New("remetentes confiáveis demais")
	}
	for i, s := range p.TrustedSenders {
		if !Email(s) || slices.Contains(p.TrustedSenders[:i], s) {
			return fmt.Errorf("remetente inválido: %q", s)
		}
	}
	return nil
}

// ParsePrefs decodes and validates user preferences.
func ParsePrefs(r io.Reader) (Prefs, error) {
	var p Prefs
	if err := DecodeStrict(r, MaxPrefsBytes, &p); err != nil {
		return Prefs{}, err
	}
	for i, s := range p.TrustedSenders {
		p.TrustedSenders[i] = strings.ToLower(strings.TrimSpace(s))
	}
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	return p, p.Validate()
}
