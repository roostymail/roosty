package mail

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Session wraps a logged-in IMAP connection for the duration of one request.
type Session struct {
	c    *imapclient.Client
	conn *conn
	pool *Pool
}

type Mailbox struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Parent string `json:"parent,omitempty"`
	Total  uint32 `json:"total"`
	Unread uint32 `json:"unread"`
}

type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type Summary struct {
	ID            uint32    `json:"id"`
	Mailbox       string    `json:"mailbox"`
	From          []Address `json:"from"`
	To            []Address `json:"to"`
	Subject       string    `json:"subject"`
	Date          time.Time `json:"date"`
	Preview       string    `json:"preview"`
	Unread        bool      `json:"unread"`
	Flagged       bool      `json:"flagged"`
	Answered      bool      `json:"answered"`
	HasAttachment bool      `json:"hasAttachment"`
	Size          int64     `json:"size"`
	MessageID     string    `json:"messageId,omitempty"`
}

type Filter struct {
	Text    string `json:"text,omitempty"`
	Unread  bool   `json:"unread,omitempty"`
	Flagged bool   `json:"flagged,omitempty"`
}

func (s *Session) Caps() imap.CapSet { return s.c.Caps() }

func (s *Session) selectBox(name string) error {
	if s.conn.selected == name {
		return nil
	}
	if _, err := s.c.Select(name, nil).Wait(); err != nil {
		s.conn.selected = ""
		return fmt.Errorf("select %s: %w", name, err)
	}
	s.conn.selected = name
	return nil
}

var roleByAttr = map[imap.MailboxAttr]string{
	imap.MailboxAttrSent: "sent", imap.MailboxAttrDrafts: "drafts", imap.MailboxAttrTrash: "trash",
	imap.MailboxAttrJunk: "junk", imap.MailboxAttrArchive: "archive",
}

var roleByName = map[string]string{
	"sent": "sent", "sent items": "sent", "sent messages": "sent", "sent mail": "sent", "enviados": "sent", "itens enviados": "sent",
	"drafts": "drafts", "draft": "drafts", "rascunhos": "drafts",
	"trash": "trash", "deleted items": "trash", "deleted messages": "trash", "bin": "trash", "lixeira": "trash", "lixo": "trash",
	"junk": "junk", "spam": "junk", "junk e-mail": "junk", "lixo eletrônico": "junk",
	"archive": "archive", "archives": "archive", "arquivo": "archive", "arquivados": "archive",
}

var roleOrder = map[string]int{"inbox": 0, "drafts": 1, "sent": 2, "archive": 3, "junk": 4, "trash": 5}

// Mailboxes lists folders with roles and counters.
func (s *Session) Mailboxes() ([]Mailbox, error) {
	caps := s.c.Caps()
	opts := &imap.ListOptions{}
	listStatus := caps.Has(imap.CapListStatus) || caps.Has(imap.CapIMAP4rev2)
	if listStatus {
		opts.ReturnStatus = &imap.StatusOptions{NumMessages: true, NumUnseen: true}
	}
	if caps.Has(imap.CapSpecialUse) {
		opts.ReturnSpecialUse = true
	}
	list, err := s.c.List("", "*", opts).Collect()
	if err != nil {
		return nil, err
	}
	seenRole := map[string]bool{}
	var out []Mailbox
	for _, l := range list {
		noSelect := false
		role := ""
		for _, a := range l.Attrs {
			if a == imap.MailboxAttrNoSelect || a == imap.MailboxAttrNonExistent {
				noSelect = true
			}
			if r, ok := roleByAttr[a]; ok {
				role = r
			}
		}
		if noSelect {
			continue
		}
		name := l.Mailbox
		display, parent := name, ""
		if l.Delim != 0 {
			if i := strings.LastIndex(name, string(l.Delim)); i > 0 {
				display, parent = name[i+1:], name[:i]
			}
		}
		if strings.EqualFold(name, "INBOX") {
			role = "inbox"
		}
		if role == "" && parent == "" {
			role = roleByName[strings.ToLower(display)]
		}
		if role != "" {
			if seenRole[role] {
				role = ""
			} else {
				seenRole[role] = true
			}
		}
		mb := Mailbox{ID: name, Name: display, Role: role, Parent: parent}
		st := l.Status
		if st == nil {
			if d, err := s.c.Status(name, &imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait(); err == nil {
				st = d
			}
		}
		if st != nil {
			if st.NumMessages != nil {
				mb.Total = *st.NumMessages
			}
			if st.NumUnseen != nil {
				mb.Unread = *st.NumUnseen
			}
		}
		out = append(out, mb)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, iok := roleOrder[out[i].Role]
		rj, jok := roleOrder[out[j].Role]
		if iok != jok {
			return iok
		}
		if iok && jok {
			return ri < rj
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out, nil
}

// FindRole returns the mailbox with a role, creating a default one if allowed.
func (s *Session) FindRole(role string, create bool) (string, error) {
	boxes, err := s.Mailboxes()
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if b.Role == role {
			return b.ID, nil
		}
	}
	if !create {
		return "", fmt.Errorf("no %s folder", role)
	}
	names := map[string]string{"sent": "Sent", "drafts": "Drafts", "trash": "Trash", "junk": "Junk", "archive": "Archive"}
	name := names[role]
	if err := s.c.Create(name, nil).Wait(); err != nil {
		return "", err
	}
	return name, nil
}

// Query returns message UIDs of a mailbox, newest first, and the total.
func (s *Session) Query(mailbox string, f Filter, position, limit int) ([]uint32, int, error) {
	if err := s.selectBox(mailbox); err != nil {
		return nil, 0, err
	}
	crit := &imap.SearchCriteria{}
	if f.Unread {
		crit.NotFlag = append(crit.NotFlag, imap.FlagSeen)
	}
	if f.Flagged {
		crit.Flag = append(crit.Flag, imap.FlagFlagged)
	}
	if t := strings.TrimSpace(f.Text); t != "" {
		crit.Text = []string{t}
	}
	var uids []uint32
	if s.c.Caps().Has(imap.CapSort) {
		res, err := s.c.UIDSort(&imapclient.SortOptions{SearchCriteria: crit,
			SortCriteria: []imapclient.SortCriterion{{Key: imapclient.SortKeyArrival, Reverse: true}}}).Wait()
		if err != nil {
			return nil, 0, err
		}
		uids = res
	} else {
		res, err := s.c.UIDSearch(crit, nil).Wait()
		if err != nil {
			return nil, 0, err
		}
		for _, u := range res.AllUIDs() {
			uids = append(uids, uint32(u))
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	}
	total := len(uids)
	if position > total {
		position = total
	}
	end := position + limit
	if limit <= 0 || end > total {
		end = total
	}
	out := append([]uint32{}, uids[position:end]...)
	return out, total, nil
}

func toUIDSet(ids []uint32) imap.UIDSet {
	u := make([]imap.UID, len(ids))
	for i, id := range ids {
		u[i] = imap.UID(id)
	}
	return imap.UIDSetNum(u...)
}

func addrs(list []imap.Address) []Address {
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Email: a.Addr()})
	}
	return out
}

func hasFlag(flags []imap.Flag, f imap.Flag) bool {
	for _, x := range flags {
		if strings.EqualFold(string(x), string(f)) {
			return true
		}
	}
	return false
}

type previewPart struct {
	size     uint32
	path     []int
	encoding string
	charset  string
	html     bool
}

// Get returns summaries (with previews) for the given UIDs, in the same order.
func (s *Session) Get(mailbox string, ids []uint32) ([]Summary, error) {
	if len(ids) == 0 {
		return []Summary{}, nil
	}
	if err := s.selectBox(mailbox); err != nil {
		return nil, err
	}
	msgs, err := s.c.Fetch(toUIDSet(ids), &imap.FetchOptions{
		UID: true, Flags: true, Envelope: true, RFC822Size: true, InternalDate: true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}).Collect()
	if err != nil {
		return nil, err
	}
	byUID := map[uint32]*Summary{}
	parts := map[uint32]previewPart{}
	for _, m := range msgs {
		sm := &Summary{ID: uint32(m.UID), Mailbox: mailbox, Size: m.RFC822Size,
			Unread: !hasFlag(m.Flags, imap.FlagSeen), Flagged: hasFlag(m.Flags, imap.FlagFlagged), Answered: hasFlag(m.Flags, imap.FlagAnswered)}
		if e := m.Envelope; e != nil {
			sm.Subject, sm.Date, sm.MessageID = e.Subject, e.Date, e.MessageID
			sm.From, sm.To = addrs(e.From), addrs(e.To)
		}
		if sm.Date.IsZero() {
			sm.Date = m.InternalDate
		}
		if m.BodyStructure != nil {
			pp, att := inspectStructure(m.BodyStructure)
			sm.HasAttachment = att
			if pp != nil {
				parts[sm.ID] = *pp
			}
		}
		byUID[sm.ID] = sm
	}
	// Fetch preview text, grouping messages that share the same part path.
	groups := map[string][]uint32{}
	for uid, pp := range parts {
		key := fmt.Sprint(pp.path)
		groups[key] = append(groups[key], uid)
	}
	for _, uids := range groups {
		pp := parts[uids[0]]
		res, err := s.fetchPreview(uids, pp)
		if err != nil {
			if s.Closed() {
				return nil, err // the pool reconnects and retries
			}
			continue
		}
		for _, m := range res {
			p := parts[uint32(m.UID)]
			var raw []byte
			for _, b := range m.BodySection {
				raw = b.Bytes
			}
			if sm := byUID[uint32(m.UID)]; sm != nil {
				sm.Preview = previewText(raw, p)
			}
		}
	}
	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		if sm, ok := byUID[id]; ok {
			out = append(out, *sm)
		}
	}
	return out, nil
}

func (s *Session) fetchPreview(uids []uint32, pp previewPart) ([]*imapclient.FetchMessageBuffer, error) {
	sec := &imap.FetchItemBodySection{Part: pp.path, Peek: true}
	if len(pp.path) == 0 {
		sec.Specifier = imap.PartSpecifierText
	}
	if !s.pool.noPartial.Load() {
		sec.Partial = &imap.SectionPartial{Offset: 0, Size: 3072}
		res, err := s.c.Fetch(toUIDSet(uids), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{sec}}).Collect()
		if err == nil {
			return res, nil
		}
		s.pool.noPartial.Store(true)
		if s.Closed() {
			return nil, err
		}
		sec.Partial = nil
	}
	// Whole-part fallback, only for parts small enough to be cheap.
	var small []uint32
	for _, u := range uids {
		small = append(small, u)
	}
	if pp.size > 256<<10 {
		return nil, fmt.Errorf("part too large for preview")
	}
	return s.c.Fetch(toUIDSet(small), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{sec}}).Collect()
}

func inspectStructure(bs imap.BodyStructure) (*previewPart, bool) {
	var plain, html *previewPart
	attachment := false
	bs.Walk(func(path []int, part imap.BodyStructure) bool {
		sp, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		disp := ""
		if sp.Extended != nil && sp.Extended.Disposition != nil {
			disp = strings.ToLower(sp.Extended.Disposition.Value)
		}
		if disp == "attachment" || (sp.Filename() != "" && !strings.EqualFold(sp.Type, "text")) {
			if !(disp == "inline" && sp.ID != "") {
				attachment = true
			}
			return true
		}
		if strings.EqualFold(sp.Type, "text") {
			p := &previewPart{size: sp.Size, path: append([]int(nil), path...), encoding: strings.ToLower(sp.Encoding), charset: sp.Params["charset"]}
			if strings.EqualFold(sp.Subtype, "plain") && plain == nil {
				plain = p
			} else if strings.EqualFold(sp.Subtype, "html") && html == nil {
				p.html = true
				html = p
			}
		}
		return true
	})
	if plain != nil {
		return plain, attachment
	}
	return html, attachment
}

// Raw fetches the full RFC 5322 message without marking it read.
func (s *Session) Raw(mailbox string, uid uint32) ([]byte, []imap.Flag, error) {
	if err := s.selectBox(mailbox); err != nil {
		return nil, nil, err
	}
	sec := &imap.FetchItemBodySection{Peek: true}
	msgs, err := s.c.Fetch(toUIDSet([]uint32{uid}), &imap.FetchOptions{UID: true, Flags: true, BodySection: []*imap.FetchItemBodySection{sec}}).Collect()
	if err != nil {
		return nil, nil, err
	}
	if len(msgs) == 0 {
		return nil, nil, fmt.Errorf("message not found")
	}
	var raw []byte
	for _, b := range msgs[0].BodySection {
		raw = b.Bytes
	}
	return raw, msgs[0].Flags, nil
}

// SetFlag adds or removes a flag on messages.
func (s *Session) SetFlag(mailbox string, ids []uint32, flag imap.Flag, on bool) error {
	if err := s.selectBox(mailbox); err != nil {
		return err
	}
	op := imap.StoreFlagsAdd
	if !on {
		op = imap.StoreFlagsDel
	}
	return s.c.Store(toUIDSet(ids), &imap.StoreFlags{Op: op, Silent: true, Flags: []imap.Flag{flag}}, nil).Close()
}

// Move moves messages to another mailbox, with a COPY fallback.
func (s *Session) Move(mailbox string, ids []uint32, dest string) error {
	if err := s.selectBox(mailbox); err != nil {
		return err
	}
	set := toUIDSet(ids)
	if s.c.Caps().Has(imap.CapMove) {
		_, err := s.c.Move(set, dest).Wait()
		return err
	}
	if _, err := s.c.Copy(set, dest).Wait(); err != nil {
		return err
	}
	return s.expunge(set)
}

func (s *Session) expunge(set imap.UIDSet) error {
	if err := s.c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return err
	}
	if s.c.Caps().Has(imap.CapUIDPlus) {
		return s.c.UIDExpunge(set).Close()
	}
	return s.c.Expunge().Close()
}

// Destroy moves messages to Trash, or deletes them for good when already there.
func (s *Session) Destroy(mailbox string, ids []uint32) error {
	trash, err := s.FindRole("trash", true)
	if err != nil {
		return err
	}
	if mailbox == trash {
		if err := s.selectBox(mailbox); err != nil {
			return err
		}
		return s.expunge(toUIDSet(ids))
	}
	return s.Move(mailbox, ids, trash)
}

// Append stores a message in a mailbox.
func (s *Session) Append(mailbox string, msg []byte, flags []imap.Flag) error {
	cmd := s.c.Append(mailbox, int64(len(msg)), &imap.AppendOptions{Flags: flags, Time: time.Now()})
	if _, err := cmd.Write(msg); err != nil {
		return err
	}
	if err := cmd.Close(); err != nil {
		return err
	}
	_, err := cmd.Wait()
	return err
}

// Message fetches, parses and marks a message as read.
func (s *Session) Message(mailbox string, uid uint32, markRead bool) (*Parsed, error) {
	raw, flags, err := s.Raw(mailbox, uid)
	if err != nil {
		return nil, err
	}
	p, err := Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	p.Flagged = hasFlag(flags, imap.FlagFlagged)
	if markRead && !hasFlag(flags, imap.FlagSeen) {
		_ = s.SetFlag(mailbox, []uint32{uid}, imap.FlagSeen, true)
	}
	return p, nil
}

// CreateMailbox creates a folder.
func (s *Session) CreateMailbox(name string) error { return s.c.Create(name, nil).Wait() }

// Closed reports whether the underlying connection dropped.
func (s *Session) Closed() bool { return isClosed(s.c) }
