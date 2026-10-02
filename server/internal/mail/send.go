package mail

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	gomail "github.com/emersion/go-message/mail"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/roostymail/roosty/server/internal/settings"
)

type OutgoingAttachment struct {
	Name string
	Type string
	Path string
}

type Draft struct {
	FromName    string
	FromEmail   string
	To, Cc, Bcc []string
	Subject     string
	HTML        string
	InReplyTo   string
	References  string
	Attachments []OutgoingAttachment
}

func parseList(list []string) ([]*gomail.Address, error) {
	var out []*gomail.Address
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		as, err := gomail.ParseAddressList(s)
		if err != nil {
			return nil, fmt.Errorf("endereço inválido: %s", s)
		}
		out = append(out, as...)
	}
	return out, nil
}

// Build renders the draft as an RFC 5322 message and returns the envelope recipients.
func Build(d Draft) ([]byte, []string, error) {
	to, err := parseList(d.To)
	if err != nil {
		return nil, nil, err
	}
	cc, err := parseList(d.Cc)
	if err != nil {
		return nil, nil, err
	}
	bcc, err := parseList(d.Bcc)
	if err != nil {
		return nil, nil, err
	}
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil, nil, fmt.Errorf("adicione pelo menos um destinatário")
	}
	var h gomail.Header
	h.SetDate(time.Now())
	h.SetAddressList("From", []*gomail.Address{{Name: d.FromName, Address: d.FromEmail}})
	h.SetAddressList("To", to)
	if len(cc) > 0 {
		h.SetAddressList("Cc", cc)
	}
	h.SetSubject(d.Subject)
	domain := "roosty.local"
	if at := strings.LastIndex(d.FromEmail, "@"); at > 0 {
		domain = d.FromEmail[at+1:]
	}
	_ = h.GenerateMessageIDWithHostname(domain)
	if d.InReplyTo != "" {
		h.Set("In-Reply-To", d.InReplyTo)
		refs := strings.TrimSpace(d.References + " " + d.InReplyTo)
		h.Set("References", refs)
	}
	h.Set("User-Agent", "Roosty Mail")

	safe := Sanitize(d.HTML, SanitizeOptions{AllowRemote: true, ProxyURL: func(s string) string { return s }, InlineURL: func(c string) string { return "cid:" + c }})
	htmlBody := "<!doctype html><html><body>" + safe.HTML + "</body></html>"
	text := HTMLToText(safe.HTML)

	var buf bytes.Buffer
	mw, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, nil, err
	}
	iw, err := mw.CreateInline()
	if err != nil {
		return nil, nil, err
	}
	for _, p := range []struct{ ct, body string }{{"text/plain", text}, {"text/html", htmlBody}} {
		var ih gomail.InlineHeader
		ih.SetContentType(p.ct, map[string]string{"charset": "utf-8"})
		w, err := iw.CreatePart(ih)
		if err != nil {
			return nil, nil, err
		}
		_, _ = io.WriteString(w, p.body)
		_ = w.Close()
	}
	_ = iw.Close()
	for _, a := range d.Attachments {
		f, err := os.Open(a.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("anexo %s: %w", a.Name, err)
		}
		var ah gomail.AttachmentHeader
		ct := a.Type
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah.SetContentType(ct, nil)
		ah.SetFilename(a.Name)
		w, err := mw.CreateAttachment(ah)
		if err != nil {
			f.Close()
			return nil, nil, err
		}
		_, _ = io.Copy(w, f)
		f.Close()
		_ = w.Close()
	}
	if err := mw.Close(); err != nil {
		return nil, nil, err
	}
	var rcpt []string
	for _, l := range [][]*gomail.Address{to, cc, bcc} {
		for _, a := range l {
			rcpt = append(rcpt, a.Address)
		}
	}
	return buf.Bytes(), rcpt, nil
}

func dialSMTP(ms settings.MailServer) (*smtp.Client, error) {
	addr := net.JoinHostPort(ms.SMTP.Host, strconv.Itoa(ms.SMTP.Port))
	cfg := tlsConfig(ms.SMTP.Host, ms.SkipTLSVerify)
	switch ms.SMTP.Security {
	case "starttls":
		return smtp.DialStartTLS(addr, cfg)
	case "none":
		return smtp.Dial(addr)
	default:
		return smtp.DialTLS(addr, cfg)
	}
}

// Submit sends a built message through SMTP submission as the user.
func Submit(ms settings.MailServer, cr Creds, from string, rcpt []string, msg []byte) error {
	c, err := dialSMTP(ms)
	if err != nil {
		return fmt.Errorf("conectar ao servidor SMTP: %w", err)
	}
	defer c.Close()
	if err := c.Auth(sasl.NewPlainClient("", cr.Email, cr.Password)); err != nil {
		return fmt.Errorf("autenticação SMTP: %w", err)
	}
	if err := c.SendMail(from, rcpt, bytes.NewReader(msg)); err != nil {
		return fmt.Errorf("envio SMTP: %w", err)
	}
	return c.Quit()
}

// SaveSent stores a copy in the Sent folder, creating it when missing.
func (s *Session) SaveSent(msg []byte) error {
	sent, err := s.FindRole("sent", true)
	if err != nil {
		return err
	}
	return s.Append(sent, msg, []imap.Flag{imap.FlagSeen})
}

// SaveDraft stores a draft, replacing the previous version when given.
func (s *Session) SaveDraft(msg []byte, replace uint32) error {
	drafts, err := s.FindRole("drafts", true)
	if err != nil {
		return err
	}
	if err := s.Append(drafts, msg, []imap.Flag{imap.FlagSeen, imap.FlagDraft}); err != nil {
		return err
	}
	if replace > 0 {
		if err := s.selectBox(drafts); err == nil {
			_ = s.expunge(toUIDSet([]uint32{replace}))
		}
	}
	return nil
}

// TestResult describes a connection check made from the admin panel.
type TestResult struct {
	IMAPOK    bool     `json:"imapOk"`
	IMAPError string   `json:"imapError,omitempty"`
	IMAPCaps  []string `json:"imapCaps"`
	SMTPOK    bool     `json:"smtpOk"`
	SMTPError string   `json:"smtpError,omitempty"`
	SMTPAuth  bool     `json:"smtpAuth"`
}

// Test connects to IMAP and SMTP without signing in.
func Test(ms settings.MailServer) TestResult {
	r := TestResult{IMAPCaps: []string{}}
	if c, err := Dial(ms, nil); err != nil {
		r.IMAPError = err.Error()
	} else {
		if caps, err := c.Capability().Wait(); err != nil {
			r.IMAPError = err.Error()
		} else {
			r.IMAPOK = true
			for k := range caps {
				r.IMAPCaps = append(r.IMAPCaps, string(k))
			}
		}
		_ = c.Logout().Wait()
		_ = c.Close()
	}
	if c, err := dialSMTP(ms); err != nil {
		r.SMTPError = err.Error()
	} else {
		if err := c.Hello("roosty"); err != nil {
			r.SMTPError = err.Error()
		} else {
			r.SMTPOK = true
			r.SMTPAuth, _ = c.Extension("AUTH")
		}
		_ = c.Quit()
	}
	return r
}
