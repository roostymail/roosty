package mail

import (
	"bytes"
	"encoding/base64"
	"html"
	"io"
	"mime/quotedprintable"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
)

const maxPartSize = 30 << 20

type Attachment struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Size      int    `json:"size"`
	ContentID string `json:"contentId,omitempty"`
	Inline    bool   `json:"inline"`
	data      []byte
}

type Parsed struct {
	Subject     string       `json:"subject"`
	From        []Address    `json:"from"`
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc"`
	ReplyTo     []Address    `json:"replyTo"`
	Date        time.Time    `json:"date"`
	MessageID   string       `json:"messageId"`
	References  string       `json:"references"`
	Text        string       `json:"text"`
	HTML        string       `json:"-"`
	Attachments []Attachment `json:"attachments"`
	Flagged     bool         `json:"flagged"`
}

func addrList(h gomail.Header, key string) []Address {
	list, err := h.AddressList(key)
	if err != nil {
		return []Address{}
	}
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Email: a.Address})
	}
	return out
}

func readLimited(r io.Reader) []byte {
	b, _ := io.ReadAll(io.LimitReader(r, maxPartSize))
	return b
}

// Parse reads a full message. Unknown charsets are tolerated.
func Parse(r io.Reader) (*Parsed, error) {
	mr, err := gomail.CreateReader(r)
	if err != nil && !message.IsUnknownCharset(err) {
		return nil, err
	}
	h := mr.Header
	p := &Parsed{From: addrList(h, "From"), To: addrList(h, "To"), Cc: addrList(h, "Cc"), ReplyTo: addrList(h, "Reply-To"), Attachments: []Attachment{}}
	p.Subject, _ = h.Subject()
	p.Date, _ = h.Date()
	p.MessageID = h.Get("Message-Id")
	p.References = strings.TrimSpace(h.Get("References"))
	idx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if message.IsUnknownCharset(err) && part != nil {
				// keep going with raw bytes
			} else {
				break
			}
		}
		idx++
		switch ph := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, params, _ := ph.ContentType()
			cid := strings.Trim(ph.Get("Content-Id"), "<> ")
			switch {
			case ct == "text/plain" && p.Text == "":
				p.Text = string(readLimited(part.Body))
			case ct == "text/html" && p.HTML == "":
				p.HTML = string(readLimited(part.Body))
			case strings.HasPrefix(ct, "text/"):
				_ = readLimited(part.Body)
			default:
				data := readLimited(part.Body)
				name := params["name"]
				if name == "" {
					name = "anexo-" + strconv.Itoa(idx)
				}
				p.Attachments = append(p.Attachments, Attachment{Index: idx, Name: name, Type: ct, Size: len(data), ContentID: cid, Inline: cid != "", data: data})
			}
		case *gomail.AttachmentHeader:
			ct, _, _ := ph.ContentType()
			name, _ := ph.Filename()
			if name == "" {
				name = "anexo-" + strconv.Itoa(idx)
			}
			data := readLimited(part.Body)
			cid := strings.Trim(ph.Get("Content-Id"), "<> ")
			p.Attachments = append(p.Attachments, Attachment{Index: idx, Name: name, Type: ct, Size: len(data), ContentID: cid, data: data})
		}
	}
	return p, nil
}

// Part returns an attachment by index or Content-ID.
func (p *Parsed) Part(index int, cid string) *Attachment {
	for i := range p.Attachments {
		a := &p.Attachments[i]
		if (index > 0 && a.Index == index) || (cid != "" && strings.EqualFold(a.ContentID, cid)) {
			return a
		}
	}
	return nil
}

func (a *Attachment) Data() []byte { return a.data }

var (
	reTags   = regexp.MustCompile(`(?s)<(script|style|head)[^>]*>.*?</(script|style|head)>|<[^>]+>`)
	reSpaces = regexp.MustCompile(`\s+`)
	reQuote  = regexp.MustCompile(`(?m)^>.*$`)
)

// HTMLToText makes a rough plain-text version of HTML.
func HTMLToText(s string) string {
	s = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>|</tr>|</td>|</h[1-6]>|</table>|</blockquote>`).ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(reSpaces.ReplaceAllString(l, " "))
	}
	return strings.TrimSpace(regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

// previewText decodes a possibly truncated text part into a short preview.
func previewText(raw []byte, p previewPart) string {
	var r io.Reader = bytes.NewReader(raw)
	switch p.encoding {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' {
				return -1
			}
			return r
		}, raw)
		clean = clean[:len(clean)/4*4]
		dec, _ := base64.StdEncoding.DecodeString(string(clean))
		r = bytes.NewReader(dec)
	case "quoted-printable":
		r = quotedprintable.NewReader(bytes.NewReader(raw))
	}
	if p.charset != "" && !strings.EqualFold(p.charset, "utf-8") && !strings.EqualFold(p.charset, "us-ascii") {
		if cr, err := charset.Reader(p.charset, r); err == nil {
			r = cr
		}
	}
	b, _ := io.ReadAll(r)
	s := string(b)
	if p.html {
		s = HTMLToText(s)
	}
	s = reQuote.ReplaceAllString(s, "")
	s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	if len([]rune(s)) > 180 {
		s = string([]rune(s)[:180])
	}
	return s
}
