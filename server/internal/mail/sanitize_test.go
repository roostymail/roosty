package mail

import (
	"strings"
	"testing"
)

var opts = SanitizeOptions{
	ProxyURL:  func(s string) string { return "/img/sig/" + s },
	InlineURL: func(c string) string { return "/api/inline?cid=" + c },
}

func TestSanitizeRemovesActiveContent(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`<img src=x onerror="alert(1)">`,
		`<svg><animate onbegin="alert(1)" attributeName="x"/></svg>`,
		`<a href="javascript:alert(1)">x</a>`,
		`<iframe src="https://evil.example"></iframe>`,
		`<form action="https://evil.example"><input name=p></form>`,
		`<math><mi xlink:href="javascript:alert(1)">x</mi></math>`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<div style="width:expression(alert(1))">x</div>`,
		`<img src="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=">`,
		`<object data="x.swf"></object><embed src="x.swf">`,
		`<base href="https://evil.example/">`,
		`<meta http-equiv="refresh" content="0;url=https://evil.example">`,
	}
	bad := []string{"<script", "onerror", "onbegin", "javascript:", "<iframe", "<form", "<svg", "expression(", "url(", "svg+xml", "<object", "<embed", "<base", "<meta"}
	for _, p := range payloads {
		out := Sanitize(p, opts).HTML
		for _, b := range bad {
			if strings.Contains(strings.ToLower(out), b) {
				t.Errorf("payload %q left %q in output: %s", p, b, out)
			}
		}
	}
}

func TestSanitizeImages(t *testing.T) {
	in := `<img src="https://cdn.example/a.png" width="100"><img src="https://t.example/p.gif" width="1" height="1"><img src="cid:logo@x"><img src="data:image/png;base64,AAAA">`
	r := Sanitize(in, opts)
	if r.Blocked != 1 || r.Trackers != 1 {
		t.Fatalf("blocked=%d trackers=%d, want 1 and 1", r.Blocked, r.Trackers)
	}
	if strings.Contains(r.HTML, "cdn.example") {
		t.Errorf("remote image not blocked: %s", r.HTML)
	}
	if !strings.Contains(r.HTML, "/api/inline?cid=logo@x") || !strings.Contains(r.HTML, "data:image/png") {
		t.Errorf("inline images rewritten wrongly: %s", r.HTML)
	}
	opts2 := opts
	opts2.AllowRemote = true
	if r := Sanitize(in, opts2); !strings.Contains(r.HTML, "/img/sig/https://cdn.example/a.png") {
		t.Errorf("remote image not proxied: %s", r.HTML)
	}
}

func TestParseCharsets(t *testing.T) {
	raw := "From: =?ISO-8859-1?Q?Jo=E3o?= <j@x.test>\r\nSubject: =?UTF-8?B?T2zDoQ==?=\r\nContent-Type: text/plain; charset=ISO-8859-1\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nPreview: s=E1bado =F3timo\r\n"
	p, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Olá" || p.From[0].Name != "João" || !strings.Contains(p.Text, "sábado ótimo") {
		t.Errorf("decoded wrongly: %q %q %q", p.Subject, p.From[0].Name, p.Text)
	}
}
