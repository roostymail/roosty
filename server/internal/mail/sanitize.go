package mail

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Roosty's own HTML sanitizer for email bodies.
//
// The input is parsed with the HTML5 parser from golang.org/x/net/html (the
// same algorithm browsers use), then the tree is walked against allowlists of
// elements, attributes, URL schemes and CSS properties. The result is
// serialized by us, escaping every text node and attribute value, so the
// browser re-parses exactly the tree we approved.
//
// Anything not explicitly allowed is removed. Elements that switch parsing
// modes (svg, math, style, script, textarea, noscript, template…) are dropped
// with their content, which closes the namespace-confusion and mutation-XSS
// classes behind most webmail sanitizer bypasses.

// SanitizeOptions controls how remote resources and inline images are rewritten.
type SanitizeOptions struct {
	AllowRemote bool
	ProxyURL    func(src string) string // remote image -> signed proxy URL
	InlineURL   func(cid string) string // cid: image -> attachment URL
}

type SanitizeResult struct {
	HTML     string `json:"html"`
	Blocked  int    `json:"blocked"`
	Trackers int    `json:"trackers"`
}

const (
	maxDepth    = 100     // deeper nesting is flattened to text
	maxElements = 50000   // beyond this only text is kept
	maxInput    = 8 << 20 // longer bodies are truncated before parsing
	maxStyleLen = 2000
)

// dropWithContent are removed together with everything inside them.
var dropWithContent = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Head: true, atom.Title: true, atom.Meta: true, atom.Link: true,
	atom.Base: true, atom.Iframe: true, atom.Frame: true, atom.Frameset: true, atom.Object: true, atom.Embed: true,
	atom.Applet: true, atom.Svg: true, atom.Math: true, atom.Template: true, atom.Noscript: true, atom.Noembed: true,
	atom.Noframes: true, atom.Xmp: true, atom.Plaintext: true, atom.Textarea: true, atom.Select: true,
	atom.Option: true, atom.Button: true, atom.Input: true, atom.Audio: true, atom.Video: true, atom.Source: true,
	atom.Track: true, atom.Canvas: true, atom.Dialog: true, atom.Param: true, atom.Keygen: true,
}

// Tags with no closing tag.
var voidElements = map[atom.Atom]bool{atom.Br: true, atom.Hr: true, atom.Img: true, atom.Col: true, atom.Wbr: true}

var globalAttrs = set("dir", "lang", "title", "align", "valign", "width", "height", "bgcolor", "border", "style")

// allowed maps each element to the attributes it may keep (besides globalAttrs).
var allowed = map[atom.Atom]map[string]bool{
	atom.A: set("href"), atom.Abbr: nil, atom.Address: nil, atom.Article: nil, atom.Aside: nil, atom.B: nil,
	atom.Bdi: nil, atom.Bdo: nil, atom.Big: nil, atom.Blockquote: nil, atom.Br: nil, atom.Caption: nil,
	atom.Center: nil, atom.Cite: nil, atom.Code: nil, atom.Col: set("span"), atom.Colgroup: set("span"),
	atom.Dd: nil, atom.Del: nil, atom.Details: nil, atom.Dfn: nil, atom.Div: nil, atom.Dl: nil, atom.Dt: nil,
	atom.Em: nil, atom.Figcaption: nil, atom.Figure: nil, atom.Font: set("color", "face", "size"),
	atom.Footer: nil, atom.H1: nil, atom.H2: nil, atom.H3: nil, atom.H4: nil, atom.H5: nil, atom.H6: nil,
	atom.Header: nil, atom.Hr: nil, atom.I: nil, atom.Img: set("src", "alt"), atom.Ins: nil, atom.Kbd: nil,
	atom.Li: set("value"), atom.Main: nil, atom.Mark: nil, atom.Nav: nil, atom.Ol: set("start", "type"),
	atom.P: nil, atom.Pre: nil, atom.Q: nil, atom.Rp: nil, atom.Rt: nil, atom.Ruby: nil, atom.S: nil,
	atom.Samp: nil, atom.Section: nil, atom.Small: nil, atom.Span: nil, atom.Strike: nil, atom.Strong: nil,
	atom.Sub: nil, atom.Summary: nil, atom.Sup: nil, atom.Table: set("cellpadding", "cellspacing"),
	atom.Tbody: nil, atom.Td: set("colspan", "rowspan", "nowrap"), atom.Tfoot: nil,
	atom.Th: set("colspan", "rowspan", "nowrap", "scope"), atom.Thead: nil, atom.Time: nil, atom.Tr: nil,
	atom.Tt: nil, atom.U: nil, atom.Ul: set("type"), atom.Var: nil, atom.Wbr: nil,
}

var cssProps = set(
	"color", "background-color", "font-family", "font-size", "font-weight", "font-style", "font-variant",
	"text-align", "text-decoration", "text-indent", "text-transform", "line-height", "letter-spacing",
	"word-spacing", "vertical-align", "white-space", "word-break", "overflow-wrap",
	"margin", "margin-top", "margin-bottom", "margin-left", "margin-right",
	"padding", "padding-top", "padding-bottom", "padding-left", "padding-right",
	"border", "border-top", "border-bottom", "border-left", "border-right", "border-color", "border-width",
	"border-style", "border-radius", "border-collapse", "border-spacing",
	"width", "max-width", "min-width", "height", "max-height", "min-height",
	"display", "table-layout", "list-style", "list-style-type", "text-overflow", "overflow",
)

// A CSS value may only contain plain tokens and color functions: no url(),
// expression(), escapes, comments, quotes breaking out, or angle brackets.
var (
	safeCSSValue = regexp.MustCompile(`^(?i)(?:[#%,.\w\s\-'"!/]|(?:rgba?|hsla?)\([\d\s.,%/]+\))*$`)
	badCSSValue  = regexp.MustCompile(`(?i)url|expression|javascript|vbscript|behavior|binding|import|\\|/\*|<|>`)
	safeDataImg  = regexp.MustCompile(`^(?i)data:image/(png|jpe?g|gif|webp);base64,[a-z0-9+/=\s]+$`)
	safeColor    = regexp.MustCompile(`^(?i)(#[0-9a-f]{3,8}|[a-z]{3,20}|rgba?\([\d\s.,%]+\))$`)
	safeNumber   = regexp.MustCompile(`^[0-9]{1,5}%?$`)
	safeFontFace = regexp.MustCompile(`^[\w\s,'"-]{1,200}$`)
	safeKeyword  = regexp.MustCompile(`^[a-zA-Z0-9-]*$`)
)

func set(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

// Sanitize cleans email HTML and rewrites images: remote ones are blocked or
// proxied, tracking pixels are dropped and cid: references point to the
// attachment endpoint.
func Sanitize(src string, o SanitizeOptions) SanitizeResult {
	if len(src) > maxInput {
		src = src[:maxInput]
	}
	ctx := &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := html.ParseFragment(strings.NewReader(src), ctx)
	res := SanitizeResult{}
	if err != nil {
		// The parser refuses pathological input (e.g. nesting deeper than 512
		// levels). Show the readable text only, with no markup at all.
		res.HTML = "<pre>" + html.EscapeString(textOnly(src)) + "</pre>"
		return res
	}
	w := &writer{o: o, res: &res}
	for _, n := range nodes {
		w.node(n, 0)
	}
	res.HTML = w.b.String()
	return res
}

// textOnly extracts visible text with the tokenizer, skipping raw-text elements.
func textOnly(src string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(src))
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.StartTagToken:
			if a := atom.Lookup([]byte(z.Token().Data)); a != 0 && dropWithContent[a] {
				skip++
			}
		case html.EndTagToken:
			if a := atom.Lookup([]byte(z.Token().Data)); a != 0 && dropWithContent[a] && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				b.Write(z.Text())
			}
		}
	}
}

type writer struct {
	b        strings.Builder
	o        SanitizeOptions
	res      *SanitizeResult
	elements int
}

func (w *writer) children(n *html.Node, depth int) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c, depth+1)
	}
}

func (w *writer) node(n *html.Node, depth int) {
	switch n.Type {
	case html.TextNode:
		w.b.WriteString(html.EscapeString(n.Data))
		return
	case html.ElementNode:
	case html.DocumentNode:
		w.children(n, depth)
		return
	default: // comments, doctypes, raw nodes
		return
	}
	// Only HTML-namespace elements; foreign content (svg, math) never survives.
	if n.Namespace != "" || dropWithContent[n.DataAtom] {
		return
	}
	attrsAllowed, ok := allowed[n.DataAtom]
	if !ok || n.DataAtom == 0 || depth > maxDepth || w.elements >= maxElements {
		w.children(n, depth) // unknown or too deep: keep only the content
		return
	}
	w.elements++
	attrs, drop := w.attrs(n, attrsAllowed)
	if drop {
		return
	}
	w.b.WriteByte('<')
	w.b.WriteString(n.DataAtom.String())
	for _, a := range attrs {
		w.b.WriteByte(' ')
		w.b.WriteString(a.Key)
		w.b.WriteString(`="`)
		w.b.WriteString(html.EscapeString(a.Val))
		w.b.WriteByte('"')
	}
	w.b.WriteByte('>')
	if voidElements[n.DataAtom] {
		return
	}
	w.children(n, depth)
	w.b.WriteString("</")
	w.b.WriteString(n.DataAtom.String())
	w.b.WriteByte('>')
}

// attrs returns the attributes to keep. drop=true removes the element entirely.
func (w *writer) attrs(n *html.Node, own map[string]bool) ([]html.Attribute, bool) {
	var out []html.Attribute
	seen := map[string]bool{}
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if a.Namespace != "" || seen[key] || (!globalAttrs[key] && !own[key]) {
			continue
		}
		seen[key] = true
		val, ok := cleanAttr(n.DataAtom, key, a.Val)
		if ok {
			out = append(out, html.Attribute{Key: key, Val: val})
		}
	}
	switch n.DataAtom {
	case atom.A:
		if href, ok := find(out, "href"); ok && isRemote(href) {
			out = append(out, html.Attribute{Key: "target", Val: "_blank"}, html.Attribute{Key: "rel", Val: "noopener noreferrer nofollow"})
		}
	case atom.Img:
		return w.img(out)
	}
	return out, false
}

func cleanAttr(el atom.Atom, key, val string) (string, bool) {
	val = strings.TrimSpace(val)
	switch key {
	case "style":
		return cleanStyle(val)
	case "href":
		return val, urlScheme(val, "http", "https", "mailto", "tel")
	case "src":
		return val, true // checked in img()
	case "bgcolor", "color":
		return val, safeColor.MatchString(val)
	case "width", "height", "border", "cellpadding", "cellspacing", "colspan", "rowspan", "span", "start", "value", "size":
		return val, safeNumber.MatchString(val)
	case "face":
		return val, safeFontFace.MatchString(val)
	case "align", "valign", "dir", "type", "scope", "nowrap":
		return val, len(val) <= 20 && safeKeyword.MatchString(val)
	default: // title, lang, alt
		return val, len(val) <= 500
	}
}

// urlScheme reports whether a URL is absolute with one of the schemes. Browsers
// ignore ASCII whitespace and control characters inside schemes ("jav\tascript:"),
// so they are removed before checking.
func urlScheme(raw string, schemes ...string) bool {
	clean := strings.Map(func(r rune) rune {
		if r <= 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, raw)
	i := strings.IndexByte(clean, ':')
	if i <= 0 || strings.ContainsAny(clean[:i], "/?#") {
		return false // relative URLs make no sense in an email
	}
	scheme := strings.ToLower(clean[:i])
	for _, s := range schemes {
		if scheme == s {
			if s == "http" || s == "https" {
				u, err := url.Parse(clean)
				return err == nil && u.Host != "" && u.User == nil
			}
			return len(clean) > i+1
		}
	}
	return false
}

func isRemote(u string) bool { return urlScheme(u, "http", "https") }

func find(attrs []html.Attribute, key string) (string, bool) {
	for _, a := range attrs {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func cleanStyle(s string) (string, bool) {
	if len(s) > maxStyleLen || badCSSValue.MatchString(s) {
		return "", false
	}
	var keep []string
	for _, decl := range strings.Split(s, ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		val = strings.TrimSpace(val)
		if cssProps[prop] && val != "" && safeCSSValue.MatchString(val) {
			keep = append(keep, prop+": "+val)
		}
	}
	if len(keep) == 0 {
		return "", false
	}
	return strings.Join(keep, "; "), true
}

func tiny(v string) bool {
	v = strings.TrimSuffix(strings.TrimSpace(v), "px")
	return v == "0" || v == "1" || v == "2"
}

// img rewrites the image source. drop=true removes the image.
func (w *writer) img(attrs []html.Attribute) ([]html.Attribute, bool) {
	src, _ := find(attrs, "src")
	wv, _ := find(attrs, "width")
	hv, _ := find(attrs, "height")
	lower := strings.ToLower(src)
	remote := isRemote(src) || strings.HasPrefix(lower, "//")
	if remote && (tiny(wv) || tiny(hv)) {
		w.res.Trackers++
		return nil, true
	}
	var newSrc string
	extra := []html.Attribute{}
	switch {
	case urlScheme(src, "cid"):
		newSrc = w.o.InlineURL(strings.TrimSpace(src[strings.IndexByte(src, ':')+1:]))
	case safeDataImg.MatchString(src):
		newSrc = src
	case remote:
		if strings.HasPrefix(lower, "//") {
			src = "https:" + src
		}
		if w.o.AllowRemote {
			newSrc = w.o.ProxyURL(src)
		} else {
			w.res.Blocked++
			extra = append(extra, html.Attribute{Key: "data-blocked", Val: "1"})
		}
	default:
		return nil, true // javascript:, relative paths, svg data, empty…
	}
	out := make([]html.Attribute, 0, len(attrs)+1)
	for _, a := range attrs {
		if a.Key == "src" {
			a.Val = newSrc
		}
		out = append(out, a)
	}
	if _, ok := find(out, "src"); !ok {
		out = append(out, html.Attribute{Key: "src", Val: newSrc})
	}
	return append(out, extra...), false
}
