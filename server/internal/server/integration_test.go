//go:build integration

// Integration tests against a real IMAP/SMTP server.
//
//	make itest   (starts GreenMail from deploy/test and runs these tests)
//
// Environment: ROOSTY_IT_IMAP (host:port), ROOSTY_IT_SMTP (host:port),
// ROOSTY_IT_USER1/PASS1, ROOSTY_IT_USER2/PASS2.
package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/roostymail/roosty/server/internal/secure"
	"github.com/roostymail/roosty/server/internal/settings"
	"github.com/roostymail/roosty/server/internal/store"
)

func itEnv(t *testing.T, key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if def == "" {
		t.Skipf("%s not set", key)
	}
	return def
}

func endpoint(t *testing.T, key, def string) settings.Endpoint {
	host, port, _ := net.SplitHostPort(itEnv(t, key, def))
	p, _ := strconv.Atoi(port)
	return settings.Endpoint{Host: host, Port: p, Security: "none"}
}

type itCtx struct {
	t                    *testing.T
	base                 string
	smtpAddr             string
	user1, pass1, user2, pass2 string
	srv                        *Server
}

func setupIT(t *testing.T) *itCtx {
	t.Helper()
	ctx := &itCtx{t: t,
		smtpAddr: itEnv(t, "ROOSTY_IT_SMTP", "127.0.0.1:3025"),
		user1:    itEnv(t, "ROOSTY_IT_USER1", "marina@roosty.test"), pass1: itEnv(t, "ROOSTY_IT_PASS1", "roosty123"),
		user2: itEnv(t, "ROOSTY_IT_USER2", "ana@roosty.test"), pass2: itEnv(t, "ROOSTY_IT_PASS2", "roosty123"),
	}
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg, _ := settings.NewManager(st)
	s := settings.Defaults()
	s.Mail.IMAP = endpoint(t, "ROOSTY_IT_IMAP", "127.0.0.1:3143")
	s.Mail.SMTP = endpoint(t, "ROOSTY_IT_SMTP", "127.0.0.1:3025")
	s.Access = settings.Access{Mode: "domains", Domains: []string{"roosty.test"}}
	if err := cfg.Save(s); err != nil {
		t.Fatal(err)
	}
	srv, err := New(st, cfg, Options{DataDir: dir, Web: fstest.MapFS{"index.html": {Data: []byte("x")}}, Version: "it", SetupToken: "x"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	ctx.base, ctx.srv = ts.URL, srv
	return ctx
}

// deliver puts a message straight into a mailbox through SMTP.
func (c *itCtx) deliver(to, subject, body string, extraHeaders ...string) {
	c.t.Helper()
	msg := "From: Remetente Teste <remetente@exemplo.test>\r\nTo: " + to + "\r\nSubject: " + subject +
		"\r\nDate: " + time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <" + secure.Token(8) + "@exemplo.test>\r\n"
	for _, h := range extraHeaders {
		msg += h + "\r\n"
	}
	if !strings.Contains(strings.Join(extraHeaders, ""), "Content-Type") {
		msg += "Content-Type: text/plain; charset=utf-8\r\n"
	}
	msg += "\r\n" + body + "\r\n"
	sc, err := smtp.Dial(c.smtpAddr)
	if err != nil {
		c.t.Fatalf("deliver: %v", err)
	}
	defer sc.Close()
	if err := sc.SendMail("remetente@exemplo.test", []string{to}, strings.NewReader(msg)); err != nil {
		c.t.Fatalf("deliver: %v", err)
	}
}

func (c *itCtx) login(user, pass string) *client {
	c.t.Helper()
	cl := newClient(c.t, c.base)
	code, body, _ := cl.do("POST", "/api/auth/login", fmt.Sprintf(`{"email":%q,"password":%q}`, user, pass), true)
	if code != http.StatusOK {
		c.t.Fatalf("login %s: %d %v", user, code, body)
	}
	return cl
}

// call runs one API method and returns its result, failing on method errors.
func apiCall(t *testing.T, cl *client, method, args string) map[string]any {
	t.Helper()
	code, body, _ := cl.do("POST", "/api", `{"calls":[["`+method+`",`+args+`,"x"]]}`, true)
	if code != http.StatusOK {
		t.Fatalf("%s: HTTP %d %v", method, code, body)
	}
	res := body["results"].([]any)[0].([]any)[1].(map[string]any)
	if e, ok := res["error"]; ok {
		t.Fatalf("%s: %v", method, e)
	}
	return res
}

// waitFor searches a mailbox until a message with the token shows up.
func waitFor(t *testing.T, cl *client, mailbox, token string) map[string]any {
	t.Helper()
	for i := 0; i < 30; i++ {
		q := apiCall(t, cl, "Email/query", fmt.Sprintf(`{"mailbox":%q,"filter":{"text":%q}}`, mailbox, token))
		if ids := q["ids"].([]any); len(ids) > 0 {
			list := apiCall(t, cl, "Email/get", fmt.Sprintf(`{"mailbox":%q,"ids":[%v]}`, mailbox, ids[0]))["list"].([]any)
			return list[0].(map[string]any)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("message %s never arrived in %s", token, mailbox)
	return nil
}

func gone(t *testing.T, cl *client, mailbox, token string) bool {
	q := apiCall(t, cl, "Email/query", fmt.Sprintf(`{"mailbox":%q,"filter":{"text":%q}}`, mailbox, token))
	return len(q["ids"].([]any)) == 0
}

func TestIT_LoginWrongPassword(t *testing.T) {
	c := setupIT(t)
	code, _, _ := newClient(t, c.base).do("POST", "/api/auth/login", fmt.Sprintf(`{"email":%q,"password":"errada"}`, c.user1), true)
	if code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", code)
	}
}

func TestIT_MailboxesHaveInbox(t *testing.T) {
	c := setupIT(t)
	cl := c.login(c.user1, c.pass1)
	list := apiCall(t, cl, "Mailbox/get", `{}`)["list"].([]any)
	if len(list) == 0 || list[0].(map[string]any)["role"] != "inbox" {
		t.Fatalf("inbox must come first: %v", list)
	}
}

func TestIT_ReceiveReadFlagMoveDelete(t *testing.T) {
	c := setupIT(t)
	token := "tok" + secure.Token(6)
	c.deliver(c.user1, "Assunto "+token, "Corpo com acentuação: ção ã é "+token)
	cl := c.login(c.user1, c.pass1)

	m := waitFor(t, cl, "INBOX", token)
	id := m["id"]
	if m["unread"] != true || !strings.Contains(m["preview"].(string), "acentuação") {
		t.Fatalf("summary wrong: %v", m)
	}
	b := apiCall(t, cl, "Email/body", fmt.Sprintf(`{"mailbox":"INBOX","id":%v}`, id))
	if !strings.Contains(b["message"].(map[string]any)["text"].(string), token) {
		t.Fatalf("body missing token: %v", b)
	}
	if again := waitFor(t, cl, "INBOX", token); again["unread"] != false {
		t.Error("opening the message must mark it as read")
	}
	apiCall(t, cl, "Email/set", fmt.Sprintf(`{"mailbox":"INBOX","ids":[%v],"flagged":true,"seen":false}`, id))
	if f := waitFor(t, cl, "INBOX", token); f["flagged"] != true || f["unread"] != true {
		t.Errorf("flags not applied: %v", f)
	}
	q := apiCall(t, cl, "Email/query", fmt.Sprintf(`{"mailbox":"INBOX","filter":{"flagged":true,"text":%q}}`, token))
	if len(q["ids"].([]any)) != 1 {
		t.Error("flagged filter did not find the message")
	}

	apiCall(t, cl, "Email/set", fmt.Sprintf(`{"mailbox":"INBOX","ids":[%v],"moveToRole":"archive"}`, id))
	if !gone(t, cl, "INBOX", token) {
		t.Fatal("message still in INBOX after archive")
	}
	archived := waitFor(t, cl, "Archive", token)

	apiCall(t, cl, "Email/set", fmt.Sprintf(`{"mailbox":"Archive","ids":[%v],"destroy":true}`, archived["id"]))
	trashed := waitFor(t, cl, "Trash", token)
	apiCall(t, cl, "Email/set", fmt.Sprintf(`{"mailbox":"Trash","ids":[%v],"destroy":true}`, trashed["id"]))
	if !gone(t, cl, "Trash", token) {
		t.Fatal("destroy in Trash must delete permanently")
	}
}

func TestIT_SanitizedHTMLAndTrackers(t *testing.T) {
	c := setupIT(t)
	token := "tok" + secure.Token(6)
	html := `<p>` + token + `</p><script>alert(1)</script><img src=x onerror=alert(1)><img src="https://cdn.example/a.png" width="200"><img src="https://t.example/p.gif" width="1" height="1"><svg onload=alert(1)></svg><a href="javascript:alert(1)">x</a>`
	c.deliver(c.user1, "HTML "+token, html, "Content-Type: text/html; charset=utf-8")
	cl := c.login(c.user1, c.pass1)
	m := waitFor(t, cl, "INBOX", token)
	b := apiCall(t, cl, "Email/body", fmt.Sprintf(`{"mailbox":"INBOX","id":%v}`, m["id"]))
	h := strings.ToLower(b["html"].(string))
	for _, bad := range []string{"<script", "onerror", "<svg", "javascript:", "cdn.example"} {
		if strings.Contains(h, bad) {
			t.Errorf("sanitized HTML contains %q: %s", bad, h)
		}
	}
	if b["blocked"].(float64) != 1 || b["trackers"].(float64) != 1 {
		t.Errorf("blocked=%v trackers=%v, want 1/1", b["blocked"], b["trackers"])
	}
	allowed := apiCall(t, cl, "Email/body", fmt.Sprintf(`{"mailbox":"INBOX","id":%v,"allowRemote":true}`, m["id"]))
	if !strings.Contains(allowed["html"].(string), "/img/") {
		t.Error("allowed remote image not proxied")
	}
}

func TestIT_SendWithAttachmentBetweenAccounts(t *testing.T) {
	c := setupIT(t)
	token := "tok" + secure.Token(6)
	g := c.login(c.user1, c.pass1)

	// upload
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "relatorio.txt")
	content := "conteudo do anexo " + token
	fw.Write([]byte(content))
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Roosty", "1")
	resp, err := g.http.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("upload failed: %v %v", err, resp.Status)
	}
	var up struct{ ID string }
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	_ = json.Unmarshal(b, &up)

	apiCall(t, g, "Email/send", fmt.Sprintf(`{"fromName":"Marina","to":[%q],"subject":"Envio %s","html":"<p>Oi <b>Ana</b> %s</p><script>x</script>","attachments":[%q]}`, c.user2, token, token, up.ID))
	sent := waitFor(t, g, "Sent", token)
	if sent["hasAttachment"] != true {
		t.Error("copy in Sent should have the attachment")
	}

	a := c.login(c.user2, c.pass2)
	m := waitFor(t, a, "INBOX", token)
	body := apiCall(t, a, "Email/body", fmt.Sprintf(`{"mailbox":"INBOX","id":%v}`, m["id"]))
	if strings.Contains(body["html"].(string), "<script") {
		t.Error("outgoing HTML must be sanitized")
	}
	atts := body["message"].(map[string]any)["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("attachments = %v", atts)
	}
	part := atts[0].(map[string]any)["index"]
	resp, err = a.http.Get(fmt.Sprintf("%s/api/attachment?m=INBOX&id=%v&part=%v", c.base, m["id"], part))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(got) != content || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Errorf("attachment download wrong: %q %s", got, resp.Header.Get("Content-Disposition"))
	}
}

func TestIT_RealtimeEvent(t *testing.T) {
	c := setupIT(t)
	cl := c.login(c.user1, c.pass1)
	req, _ := http.NewRequest("GET", c.base+"/api/events", nil)
	resp, err := cl.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event: mailbox") {
				got <- true
				return
			}
		}
	}()
	time.Sleep(1500 * time.Millisecond) // let the watcher enter IDLE
	c.deliver(c.user1, "Tempo real "+secure.Token(4), "oi")
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("no realtime event within 10s")
	}
}

func TestIT_DraftSaved(t *testing.T) {
	c := setupIT(t)
	token := "tok" + secure.Token(6)
	cl := c.login(c.user1, c.pass1)
	apiCall(t, cl, "Email/saveDraft", fmt.Sprintf(`{"subject":"Rascunho %s","html":"<p>texto</p>"}`, token))
	if d := waitFor(t, cl, "Drafts", token); d == nil {
		t.Fatal("draft not saved")
	}
}

func TestIT_ReconnectAfterDrop(t *testing.T) {
	c := setupIT(t)
	cl := c.login(c.user1, c.pass1)
	apiCall(t, cl, "Mailbox/get", `{}`)
	// Simulate a dropped connection: forget all pooled connections.
	// The next call must transparently reconnect.
	for _, ck := range cl.http.Jar.Cookies(func() *url.URL { u, _ := url.Parse(c.base); return u }()) {
		if ck.Name == sessionCookie {
			id, _, _ := strings.Cut(ck.Value, ".")
			c.srv.pool.Drop(id)
		}
	}
	apiCall(t, cl, "Mailbox/get", `{}`)
}

func TestIT_PrefsRejectMaliciousJSON(t *testing.T) {
	c := setupIT(t)
	cl := c.login(c.user1, c.pass1)
	if code, body, _ := cl.do("PUT", "/api/prefs", `{"theme":"navy","displayName":"Marina","trustedSenders":["news@roosty.dev"]}`, true); code != 200 {
		t.Fatalf("valid prefs refused: %d %v", code, body)
	}
	attacks := []string{
		`{"__proto__":{"isAdmin":true}}`, `{"theme":"<script>alert(1)</script>"}`, `{"accent":"red;x:url(y)"}`,
		`{"displayName":"<img src=x onerror=alert(1)>"}`, `{"trustedSenders":["javascript:alert(1)"]}`,
		`{"signature":"` + strings.Repeat("a", 70000) + `"}`, `null`, `[]`, `{"theme":"claro"} {}`,
	}
	for _, a := range attacks {
		if code, _, _ := cl.do("PUT", "/api/prefs", a, true); code != http.StatusBadRequest {
			t.Errorf("prefs attack accepted (%d): %.60s", code, a)
		}
	}
	_, me, _ := cl.do("GET", "/api/me", "", false)
	p := me["prefs"].(map[string]any)
	if p["theme"] != "navy" || p["displayName"] != "Marina" || p["isAdmin"] != nil {
		t.Errorf("stored prefs changed by rejected requests: %v", p)
	}
	// Batch arguments are strict too.
	code, body, _ := cl.do("POST", "/api", `{"calls":[["Email/query",{"mailbox":"INBOX","evil":1},"q"]]}`, true)
	res := body["results"].([]any)[0].([]any)[1].(map[string]any)
	if code != 200 || res["error"] == nil {
		t.Errorf("unknown batch argument accepted: %v", body)
	}
	if code, _, _ := cl.do("POST", "/api", `{"calls":[],"admin":true}`, true); code != http.StatusBadRequest {
		t.Errorf("unknown batch field accepted: %d", code)
	}
}
