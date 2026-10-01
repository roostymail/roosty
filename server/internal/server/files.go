package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/roostymail/roosty/server/internal/mail"
	"github.com/roostymail/roosty/server/internal/secure"
)

const maxUpload = 25 << 20

// ---------- attachments ----------

func (s *Server) fetchParsed(r *http.Request) (*mail.Parsed, error) {
	u := current(r)
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil {
		return nil, err
	}
	var p *mail.Parsed
	err = s.pool.Do(u.sessionID, u.creds, func(sess *mail.Session) error {
		var e error
		p, e = sess.Message(r.URL.Query().Get("m"), uint32(id), false)
		return e
	})
	return p, err
}

func safeType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" || strings.Contains(ct, "html") || strings.Contains(ct, "svg") || strings.Contains(ct, "xml") || strings.Contains(ct, "javascript") {
		return "application/octet-stream"
	}
	return ct
}

func (s *Server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	p, err := s.fetchParsed(r)
	if err != nil {
		jsonError(w, http.StatusNotFound, "mensagem não encontrada")
		return
	}
	idx, _ := strconv.Atoi(r.URL.Query().Get("part"))
	a := p.Part(idx, "")
	if a == nil {
		jsonError(w, http.StatusNotFound, "anexo não encontrado")
		return
	}
	disp := "attachment"
	ct := safeType(a.Type)
	if r.URL.Query().Get("view") == "1" && (strings.HasPrefix(ct, "image/") || ct == "application/pdf") {
		disp = "inline"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": a.Name}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(a.Data())
}

func (s *Server) handleInline(w http.ResponseWriter, r *http.Request) {
	p, err := s.fetchParsed(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a := p.Part(0, r.URL.Query().Get("cid"))
	if a == nil || !strings.HasPrefix(strings.ToLower(a.Type), "image/") || strings.Contains(strings.ToLower(a.Type), "svg") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", a.Type)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = w.Write(a.Data())
}

// ---------- uploads ----------

type uploadMeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func (s *Server) uploadDir(sessionID string) string {
	return filepath.Join(s.dataDir, "uploads", secure.Sign(s.secret, sessionID))
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusRequestEntityTooLarge, "arquivo inválido ou maior que 25 MB")
		return
	}
	defer f.Close()
	dir := s.uploadDir(current(r).sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar")
		return
	}
	id := secure.Token(12)
	out, err := os.Create(filepath.Join(dir, id))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "erro ao salvar")
		return
	}
	n, err := io.Copy(out, io.LimitReader(f, maxUpload+1))
	out.Close()
	if err != nil || n > maxUpload {
		_ = os.Remove(filepath.Join(dir, id))
		jsonError(w, http.StatusRequestEntityTooLarge, "arquivo maior que 25 MB")
		return
	}
	meta := uploadMeta{ID: id, Name: filepath.Base(hdr.Filename), Type: hdr.Header.Get("Content-Type"), Size: n}
	raw, _ := json.Marshal(meta)
	_ = os.WriteFile(filepath.Join(dir, id+".json"), raw, 0o600)
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) loadUpload(sessionID, id string) (mail.OutgoingAttachment, error) {
	if strings.ContainsAny(id, "/\\.") {
		return mail.OutgoingAttachment{}, fmt.Errorf("anexo inválido")
	}
	dir := s.uploadDir(sessionID)
	raw, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return mail.OutgoingAttachment{}, fmt.Errorf("anexo expirou, envie de novo")
	}
	var m uploadMeta
	_ = json.Unmarshal(raw, &m)
	return mail.OutgoingAttachment{Name: m.Name, Type: m.Type, Path: filepath.Join(dir, id)}, nil
}

// ---------- image proxy ----------

func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() ||
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64) // CGNAT 100.64/10
}

// proxyClient refuses to connect to private, loopback or link-local
// addresses, checked on the resolved IP of every connection (redirects included).
var proxyClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(host); ip == nil || blockedIP(ip) {
				return errors.New("blocked address")
			}
			return nil
		}}).DialContext,
		MaxIdleConns: 20, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return errors.New("bad scheme")
		}
		return nil
	},
}

func (s *Server) handleImageProxy(w http.ResponseWriter, r *http.Request) {
	raw, err := secure.DecodeKey(r.PathValue("enc"))
	src := string(raw)
	if err != nil || !secure.Verify(s.secret, src, r.PathValue("sig")) || !(strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")) {
		http.Error(w, "invalid", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	req.Header.Set("User-Agent", "RoostyImageProxy/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/gif,image/*;q=0.8")
	resp, err := proxyClient.Do(req)
	if err != nil {
		http.Error(w, "unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "image/") || strings.Contains(ct, "svg") {
		http.Error(w, "not an image", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 10<<20))
}

// ---------- server-sent events ----------

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, http.StatusInternalServerError, "streaming indisponível")
		return
	}
	u := current(r)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 5000\n\n")
	fl.Flush()
	events := make(chan string, 4)
	ctx := r.Context()
	go func() {
		for ctx.Err() == nil {
			err := mail.Watch(ctx, s.cfg.Get().Mail, u.creds, "INBOX", func() {
				select {
				case events <- "INBOX":
				default:
				}
			})
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, mail.ErrAuth) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
			}
		}
	}()
	keep := time.NewTicker(25 * time.Second)
	defer keep.Stop()
	var pending bool
	debounce := time.NewTimer(time.Hour)
	for {
		select {
		case <-ctx.Done():
			return
		case <-keep.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-events:
			if !pending {
				pending = true
				debounce.Reset(400 * time.Millisecond)
			}
		case <-debounce.C:
			if pending {
				pending = false
				fmt.Fprint(w, "event: mailbox\ndata: {\"mailbox\":\"INBOX\"}\n\n")
				fl.Flush()
			}
		}
	}
}
