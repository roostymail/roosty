package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/roostymail/roosty/server/internal/mail"
	"github.com/roostymail/roosty/server/internal/secure"
	"github.com/roostymail/roosty/server/internal/settings"
)

// The API follows the JMAP shape: a batch of [method, args, tag] calls where
// an argument named "#key" can reference a previous result.

type call [3]json.RawMessage

type batchReq struct {
	Calls []call `json:"calls"`
}

type methodErr struct {
	Error string `json:"error"`
}

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	var req batchReq
	if err := readJSON(r, &req); err != nil || len(req.Calls) == 0 || len(req.Calls) > 16 {
		jsonError(w, http.StatusBadRequest, "requisição inválida")
		return
	}
	u := current(r)
	var results [][3]any
	err := s.pool.Do(u.sessionID, u.creds, func(sess *mail.Session) error {
		// Reset on every attempt: the pool may retry after a reconnect.
		results = make([][3]any, 0, len(req.Calls))
		byTag := map[string]any{}
		for _, c := range req.Calls {
			var name, tag string
			_ = json.Unmarshal(c[0], &name)
			_ = json.Unmarshal(c[2], &tag)
			args, err := resolveRefs(c[1], byTag)
			var res any
			if err == nil {
				res, err = s.dispatch(r, u, sess, name, args)
			}
			if err != nil {
				if sess.Closed() {
					return err
				}
				slog.Debug("api method failed", "method", knownMethod(name))
				res = methodErr{Error: err.Error()}
			} else {
				var generic any
				raw, _ := json.Marshal(res)
				_ = json.Unmarshal(raw, &generic)
				byTag[tag] = generic
			}
			results = append(results, [3]any{name, res, tag})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, mail.ErrAuth) {
			jsonError(w, http.StatusUnauthorized, "a senha da caixa mudou, entre novamente")
			return
		}
		slog.Error("batch", "err", err)
		jsonError(w, http.StatusBadGateway, "não foi possível falar com o servidor de email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func resolveRefs(raw json.RawMessage, byTag map[string]any) (map[string]any, error) {
	args := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("argumentos inválidos")
		}
	}
	for k, v := range args {
		if !strings.HasPrefix(k, "#") {
			continue
		}
		ref, _ := v.(map[string]any)
		tag, _ := ref["resultOf"].(string)
		path, _ := ref["path"].(string)
		prev, ok := byTag[tag].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("referência %s inválida", tag)
		}
		args[k[1:]] = prev[strings.TrimPrefix(path, "/")]
		delete(args, k)
	}
	return args, nil
}

// decode converts generic args into a typed struct, refusing unknown fields.
func decode(args map[string]any, v any) error {
	raw, _ := json.Marshal(args)
	if err := settings.DecodeStrict(bytes.NewReader(raw), 2<<20, v); err != nil {
		return fmt.Errorf("argumentos inválidos: %w", err)
	}
	return nil
}

func (s *Server) dispatch(r *http.Request, u *userCtx, sess *mail.Session, name string, args map[string]any) (any, error) {
	switch name {
	case "Mailbox/get":
		list, err := sess.Mailboxes()
		return map[string]any{"list": list}, err

	case "Mailbox/create":
		var a struct {
			Name string `json:"name"`
		}
		if err := decode(args, &a); err != nil || strings.TrimSpace(a.Name) == "" {
			return nil, fmt.Errorf("nome de pasta inválido")
		}
		return map[string]bool{"ok": true}, sessCreate(sess, a.Name)

	case "Email/query":
		var a struct {
			Mailbox  string      `json:"mailbox"`
			Filter   mail.Filter `json:"filter"`
			Position int         `json:"position"`
			Limit    int         `json:"limit"`
		}
		if err := decode(args, &a); err != nil {
			return nil, err
		}
		if a.Limit <= 0 || a.Limit > 200 {
			a.Limit = 50
		}
		ids, total, err := sess.Query(a.Mailbox, a.Filter, a.Position, a.Limit)
		return map[string]any{"ids": ids, "total": total, "position": a.Position}, err

	case "Email/get":
		var a struct {
			Mailbox string   `json:"mailbox"`
			IDs     []uint32 `json:"ids"`
		}
		if err := decode(args, &a); err != nil {
			return nil, err
		}
		list, err := sess.Get(a.Mailbox, a.IDs)
		return map[string]any{"list": list}, err

	case "Email/body":
		var a struct {
			Mailbox     string `json:"mailbox"`
			ID          uint32 `json:"id"`
			AllowRemote bool   `json:"allowRemote"`
		}
		if err := decode(args, &a); err != nil {
			return nil, err
		}
		p, err := sess.Message(a.Mailbox, a.ID, true)
		if err != nil {
			return nil, err
		}
		allow := a.AllowRemote || s.trustedSender(u.creds.Email, p.From)
		res := mail.Sanitize(p.HTML, mail.SanitizeOptions{
			AllowRemote: allow,
			ProxyURL:    s.proxyURL,
			InlineURL: func(cid string) string {
				return "/api/inline?" + url.Values{"m": {a.Mailbox}, "id": {strconv.Itoa(int(a.ID))}, "cid": {cid}}.Encode()
			},
		})
		atts := []mail.Attachment{}
		for _, at := range p.Attachments {
			if !at.Inline {
				atts = append(atts, at)
			}
		}
		p.Attachments = atts
		return map[string]any{"message": p, "html": res.HTML, "hasHtml": p.HTML != "", "blocked": res.Blocked, "trackers": res.Trackers, "remoteAllowed": allow}, nil

	case "Email/set":
		var a struct {
			Mailbox  string   `json:"mailbox"`
			IDs      []uint32 `json:"ids"`
			Seen     *bool    `json:"seen"`
			Flagged  *bool    `json:"flagged"`
			MoveTo   string   `json:"moveTo"`
			MoveRole string   `json:"moveToRole"`
			Destroy  bool     `json:"destroy"`
		}
		if err := decode(args, &a); err != nil || len(a.IDs) == 0 {
			return nil, fmt.Errorf("argumentos inválidos")
		}
		if a.Seen != nil {
			if err := sess.SetFlag(a.Mailbox, a.IDs, imap.FlagSeen, *a.Seen); err != nil {
				return nil, err
			}
		}
		if a.Flagged != nil {
			if err := sess.SetFlag(a.Mailbox, a.IDs, imap.FlagFlagged, *a.Flagged); err != nil {
				return nil, err
			}
		}
		if a.MoveRole != "" {
			dest, err := sess.FindRole(a.MoveRole, true)
			if err != nil {
				return nil, err
			}
			a.MoveTo = dest
		}
		if a.MoveTo != "" {
			if err := sess.Move(a.Mailbox, a.IDs, a.MoveTo); err != nil {
				return nil, err
			}
		}
		if a.Destroy {
			if err := sess.Destroy(a.Mailbox, a.IDs); err != nil {
				return nil, err
			}
		}
		return map[string]bool{"ok": true}, nil

	case "Email/send", "Email/saveDraft":
		var a struct {
			FromName    string   `json:"fromName"`
			To          []string `json:"to"`
			Cc          []string `json:"cc"`
			Bcc         []string `json:"bcc"`
			Subject     string   `json:"subject"`
			HTML        string   `json:"html"`
			InReplyTo   string   `json:"inReplyTo"`
			References  string   `json:"references"`
			Attachments []string `json:"attachments"`
			ReplaceID   uint32   `json:"replaceDraft"`
		}
		if err := decode(args, &a); err != nil {
			return nil, err
		}
		d := mail.Draft{FromName: a.FromName, FromEmail: u.creds.Email, To: a.To, Cc: a.Cc, Bcc: a.Bcc,
			Subject: a.Subject, HTML: a.HTML, InReplyTo: a.InReplyTo, References: a.References}
		for _, id := range a.Attachments {
			att, err := s.loadUpload(u.sessionID, id)
			if err != nil {
				return nil, err
			}
			d.Attachments = append(d.Attachments, att)
		}
		if name == "Email/saveDraft" {
			if len(d.To)+len(d.Cc)+len(d.Bcc) == 0 {
				d.To = []string{u.creds.Email}
			}
			msg, _, err := mail.Build(d)
			if err != nil {
				return nil, err
			}
			return map[string]bool{"ok": true}, sess.SaveDraft(msg, a.ReplaceID)
		}
		msg, rcpt, err := mail.Build(d)
		if err != nil {
			return nil, err
		}
		if err := mail.Submit(s.cfg.Get().Mail, u.creds, u.creds.Email, rcpt, msg); err != nil {
			return nil, err
		}
		if err := sess.SaveSent(msg); err != nil {
			slog.Warn("save to Sent failed", "err", err)
		}
		if a.ReplaceID > 0 {
			if drafts, err := sess.FindRole("drafts", false); err == nil {
				_ = sess.Destroy(drafts, []uint32{a.ReplaceID})
			}
		}
		for _, id := range a.Attachments {
			s.removeUpload(u.sessionID, id)
		}
		return map[string]bool{"sent": true}, nil

	case "Sender/trust":
		var a struct {
			Email string `json:"email"`
		}
		if err := decode(args, &a); err != nil {
			return nil, fmt.Errorf("endereço inválido")
		}
		return map[string]bool{"ok": true}, s.addTrusted(u.creds.Email, a.Email)
	}
	return nil, fmt.Errorf("método desconhecido: %s", name)
}

var methods = map[string]bool{"Mailbox/get": true, "Mailbox/create": true, "Email/query": true, "Email/get": true,
	"Email/body": true, "Email/set": true, "Email/send": true, "Email/saveDraft": true, "Sender/trust": true}

// knownMethod keeps client-supplied text out of the logs.
func knownMethod(name string) string {
	if methods[name] {
		return name
	}
	return "unknown"
}

func sessCreate(sess *mail.Session, name string) error { return sess.CreateMailbox(name) }

func (s *Server) proxyURL(src string) string {
	enc := secure.EncodeKey([]byte(src))
	return "/img/" + secure.Sign(s.secret, src) + "/" + enc
}

func (s *Server) trustedSender(owner string, from []mail.Address) bool {
	if len(from) == 0 {
		return false
	}
	for _, t := range s.loadPrefs(owner).TrustedSenders {
		if strings.EqualFold(t, from[0].Email) {
			return true
		}
	}
	return false
}

func (s *Server) addTrusted(owner, sender string) error {
	sender = strings.ToLower(strings.TrimSpace(sender))
	if !settings.Email(sender) {
		return fmt.Errorf("endereço inválido")
	}
	p := s.loadPrefs(owner)
	for _, t := range p.TrustedSenders {
		if t == sender {
			return nil
		}
	}
	p.TrustedSenders = append(p.TrustedSenders, sender)
	return s.savePrefs(owner, p)
}
