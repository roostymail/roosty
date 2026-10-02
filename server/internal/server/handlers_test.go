package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
)

// newTestServer starts a Roosty server with a temporary data dir. The mail
// server points to a closed port, so tests here never reach IMAP.
func newTestServer(t *testing.T, setupToken string) (*httptest.Server, *Server) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, err := settings.NewManager(st)
	if err != nil {
		t.Fatal(err)
	}
	s := settings.Defaults()
	s.Mail.IMAP = settings.Endpoint{Host: "127.0.0.1", Port: 1, Security: "none"}
	s.Mail.SMTP = settings.Endpoint{Host: "127.0.0.1", Port: 1, Security: "none"}
	s.Access = settings.Access{Mode: "domains", Domains: []string{"roosty.test"}}
	if err := cfg.Save(s); err != nil {
		t.Fatal(err)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>app</title>")}, "assets/app.js": {Data: []byte("x")}}
	srv, err := New(st, cfg, Options{DataDir: dir, Web: web, Version: "test", SetupToken: setupToken})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path, body string, protect bool) (int, map[string]any, http.Header) {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if protect {
		req.Header.Set("X-Roosty", "1")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, resp.Header
}

func TestSecurityHeaders(t *testing.T) {
	ts, _ := newTestServer(t, "")
	_, _, h := newClient(t, ts.URL).do("GET", "/", "", false)
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "SAMEORIGIN",
	} {
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	csp := h.Get("Content-Security-Policy")
	for _, d := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "base-uri 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("CSP missing %q: %s", d, csp)
		}
	}
}

func TestCSRF_WritesRequireHeader(t *testing.T) {
	ts, _ := newTestServer(t, "")
	c := newClient(t, ts.URL)
	for _, path := range []string{"/api/auth/login", "/api", "/api/admin/login", "/api/setup/complete", "/api/upload"} {
		if code, _, _ := c.do("POST", path, "{}", false); code != http.StatusForbidden {
			t.Errorf("POST %s without X-Roosty = %d, want 403", path, code)
		}
	}
}

func TestAPI_RequiresSession(t *testing.T) {
	ts, _ := newTestServer(t, "")
	c := newClient(t, ts.URL)
	cases := []struct{ method, path string }{
		{"GET", "/api/me"}, {"POST", "/api"}, {"GET", "/api/events"}, {"GET", "/api/attachment?m=INBOX&id=1&part=1"},
		{"GET", "/api/inline?m=INBOX&id=1&cid=x"}, {"POST", "/api/upload"}, {"PUT", "/api/prefs"}, {"GET", "/img/a/b"},
	}
	for _, tc := range cases {
		if code, _, _ := c.do(tc.method, tc.path, "{}", true); code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, code)
		}
	}
}

func TestSession_ForgedCookiesRejected(t *testing.T) {
	ts, _ := newTestServer(t, "")
	for _, v := range []string{"x", "abc.def", ".", "aaaaaaaaaaaaaaaaaaaaaaaa.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		req, _ := http.NewRequest("GET", ts.URL+"/api/me", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: v})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("cookie %q accepted: %d", v, resp.StatusCode)
		}
	}
}

func TestLogin_DomainNotAllowed(t *testing.T) {
	ts, _ := newTestServer(t, "")
	code, body, _ := newClient(t, ts.URL).do("POST", "/api/auth/login", `{"email":"a@evil.test","password":"x"}`, true)
	if code != http.StatusForbidden || body["error"] == nil {
		t.Fatalf("got %d %v, want 403", code, body)
	}
}

func TestLogin_MailServerDown(t *testing.T) {
	ts, _ := newTestServer(t, "")
	code, _, _ := newClient(t, ts.URL).do("POST", "/api/auth/login", `{"email":"a@roosty.test","password":"x"}`, true)
	if code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502 when IMAP is unreachable", code)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	ts, _ := newTestServer(t, "")
	c := newClient(t, ts.URL)
	var last int
	for i := 0; i < 12; i++ {
		last, _, _ = c.do("POST", "/api/auth/login", `{"email":"a@roosty.test","password":"x"}`, true)
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("12th attempt = %d, want 429", last)
	}
}

func TestLogin_BadJSON(t *testing.T) {
	ts, _ := newTestServer(t, "")
	if code, _, _ := newClient(t, ts.URL).do("POST", "/api/auth/login", `{`, true); code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}

func TestSetup_FlowAndSingleUse(t *testing.T) {
	ts, srv := newTestServer(t, "tok-123")
	c := newClient(t, ts.URL)
	if _, body, _ := c.do("GET", "/api/setup/status", "", false); body["needed"] != true {
		t.Fatalf("setup should be needed: %v", body)
	}
	if code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"wrong","username":"admin","password":"long-password-1"}`, true); code != http.StatusForbidden {
		t.Errorf("wrong token = %d, want 403", code)
	}
	if code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"tok-123","username":"ad","password":"short"}`, true); code != http.StatusBadRequest {
		t.Errorf("weak credentials = %d, want 400", code)
	}
	settingsJSON := `{"mail":{"imap":{"host":"127.0.0.1","port":1,"security":"none"},"smtp":{"host":"127.0.0.1","port":1,"security":"none"}},"access":{"mode":"domains","domains":["roosty.test"]},"branding":{"name":"Teste","defaultTheme":"bege"}}`
	code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"tok-123","username":"admin","password":"long-password-1","settings":`+settingsJSON+`}`, true)
	if code != http.StatusOK {
		t.Fatalf("setup = %d", code)
	}
	if code, body, _ := c.do("GET", "/api/admin/me", "", false); code != http.StatusOK || body["username"] != "admin" {
		t.Fatalf("admin session after setup: %d %v", code, body)
	}
	if code, _, _ := c.do("POST", "/api/setup/complete", `{"token":"tok-123","username":"other","password":"long-password-2"}`, true); code != http.StatusGone {
		t.Errorf("second setup = %d, want 410", code)
	}
	if srv.setupToken != "" {
		t.Error("setup token not cleared")
	}
	if _, body, _ := c.do("GET", "/api/branding", "", false); body["name"] != "Teste" || body["setupNeeded"] != false {
		t.Errorf("branding not saved: %v", body)
	}
}

func TestAdmin_LoginAndProtection(t *testing.T) {
	ts, _ := newTestServer(t, "tok")
	c := newClient(t, ts.URL)
	for _, p := range []string{"/api/admin/me", "/api/admin/settings", "/api/admin/accounts", "/api/admin/overview"} {
		if code, _, _ := c.do("GET", p, "", false); code != http.StatusUnauthorized {
			t.Errorf("GET %s without admin = %d, want 401", p, code)
		}
	}
	c.do("POST", "/api/setup/complete", `{"token":"tok","username":"admin","password":"long-password-1","settings":{"mail":{"imap":{"host":"127.0.0.1","port":1,"security":"none"},"smtp":{"host":"127.0.0.1","port":1,"security":"none"}},"access":{"mode":"domains","domains":["roosty.test"]}}}`, true)
	c.do("POST", "/api/admin/logout", "{}", true)
	if code, _, _ := c.do("POST", "/api/admin/login", `{"username":"admin","password":"nope"}`, true); code != http.StatusUnauthorized {
		t.Errorf("wrong admin password = %d", code)
	}
	if code, _, _ := c.do("POST", "/api/admin/login", `{"username":"admin","password":"long-password-1"}`, true); code != http.StatusOK {
		t.Fatalf("admin login = %d", code)
	}
	if code, _, _ := c.do("PUT", "/api/admin/settings", `{"branding":{"customCss":"</style><script>x</script>"}}`, true); code != http.StatusBadRequest {
		t.Errorf("CSS breaking out of <style> accepted: %d", code)
	}
	if code, _, _ := c.do("POST", "/api/admin/accounts/explode", `{"email":"a@roosty.test"}`, true); code != http.StatusNotFound {
		t.Errorf("unknown action = %d, want 404", code)
	}
	if code, _, _ := c.do("POST", "/api/admin/accounts/block", `{"email":"ana@roosty.test"}`, true); code != http.StatusOK {
		t.Fatalf("block = %d", code)
	}
	_, body, _ := c.do("GET", "/api/admin/accounts", "", false)
	list, _ := body["list"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["blocked"] != true {
		t.Errorf("blocked account not listed: %v", body)
	}
	code, _, _ := newClient(t, ts.URL).do("POST", "/api/auth/login", `{"email":"ana@roosty.test","password":"x"}`, true)
	if code != http.StatusForbidden {
		t.Errorf("blocked account login = %d, want 403", code)
	}
}

func TestStatic_SPAFallbackAndCaching(t *testing.T) {
	ts, _ := newTestServer(t, "")
	resp, _ := http.Get(ts.URL + "/settings/anything")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "<title>app</title>") {
		t.Error("SPA fallback did not serve index.html")
	}
	resp, _ = http.Get(ts.URL + "/assets/app.js")
	resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Error("assets should be cached as immutable")
	}
	if code, _, _ := newClient(t, ts.URL).do("GET", "/api/nope", "", false); code != http.StatusNotFound {
		t.Errorf("unknown API route = %d, want 404", code)
	}
}

func TestLoadUpload_RejectsTraversal(t *testing.T) {
	_, srv := newTestServer(t, "")
	for _, id := range []string{"../../etc/passwd", "a/b", `a\b`, "x.json"} {
		if _, err := srv.loadUpload("sess", id); err == nil {
			t.Errorf("upload id %q accepted", id)
		}
	}
}

func TestImageProxy_RejectsUnsignedAndPrivate(t *testing.T) {
	_, srv := newTestServer(t, "")
	u := srv.proxyURL("http://127.0.0.1:9/x.png")
	if !strings.HasPrefix(u, "/img/") {
		t.Fatalf("unexpected proxy URL %s", u)
	}
	if _, err := proxyClient.Get("http://169.254.169.254/latest/meta-data/"); err == nil {
		t.Error("cloud metadata address must be refused")
	}
}

func TestUploadPath_OnlyGeneratedIDs(t *testing.T) {
	_, srv := newTestServer(t, "")
	for _, id := range []string{"../../etc/passwd", "..%2F..%2Fx", "abc", "aaaaaaaaaaaaaaaa/", "aaaaaaaaaaaaaaa.", "aaaaaaaaaaaaaaaa\x00", "AAAAAAAAAAAAAAAAA"} {
		if _, err := srv.uploadPath("sess", id, ""); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
	p, err := srv.uploadPath("sess", "Abc-def_ghij1234", ".json")
	if err != nil || !strings.HasSuffix(p, "Abc-def_ghij1234.json") {
		t.Fatalf("valid id refused: %v %s", err, p)
	}
}

func TestLogout_ClearsCookiesWithSameAttributes(t *testing.T) {
	ts, _ := newTestServer(t, "")
	c := newClient(t, ts.URL)
	for _, path := range []string{"/api/auth/logout", "/api/admin/logout"} {
		_, _, h := c.do("POST", path, "{}", true)
		ck := h.Get("Set-Cookie")
		for _, want := range []string{"Max-Age=0", "HttpOnly", "SameSite=Strict"} {
			if !strings.Contains(ck, want) {
				t.Errorf("%s cookie %q missing %s", path, ck, want)
			}
		}
	}
}

func TestKnownMethod_KeepsUserTextOutOfLogs(t *testing.T) {
	if knownMethod("Email/get") != "Email/get" || knownMethod("x\nINFO fake log line") != "unknown" {
		t.Fatal("unexpected method name in logs")
	}
}
