// Package mail talks to the user's IMAP and SMTP servers. It keeps one IMAP
// connection per signed-in session, parses and sanitizes messages and sends
// mail through SMTP submission.
package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/roostymail/roosty/server/internal/settings"
)

// Creds are the mailbox credentials of a session, decrypted per request.
type Creds struct {
	Email    string
	Password string
}

var ErrAuth = errors.New("authentication failed")

const idleTimeout = 5 * time.Minute

// Pool keeps one IMAP connection per session and closes idle ones.
type Pool struct {
	server func() settings.MailServer
	mu     sync.Mutex
	conns  map[string]*conn
	// noPartial is set when the server mangles partial FETCH responses
	// (seen on GreenMail); previews then fetch whole small parts instead.
	noPartial atomic.Bool
}

type conn struct {
	mu       sync.Mutex
	c        *imapclient.Client
	selected string
	lastUsed time.Time
}

func NewPool(server func() settings.MailServer) *Pool {
	p := &Pool{server: server, conns: map[string]*conn{}}
	go p.janitor()
	return p
}

func (p *Pool) janitor() {
	for range time.Tick(time.Minute) {
		p.mu.Lock()
		for id, cn := range p.conns {
			if cn.mu.TryLock() {
				if time.Since(cn.lastUsed) > idleTimeout {
					if cn.c != nil {
						_ = cn.c.Logout().Wait()
						_ = cn.c.Close()
					}
					delete(p.conns, id)
				}
				cn.mu.Unlock()
			}
		}
		p.mu.Unlock()
	}
}

// Drop closes the connection of a session (logout, revoked session).
func (p *Pool) Drop(sessionID string) {
	p.mu.Lock()
	cn := p.conns[sessionID]
	delete(p.conns, sessionID)
	p.mu.Unlock()
	if cn != nil {
		cn.mu.Lock()
		if cn.c != nil {
			_ = cn.c.Close()
		}
		cn.mu.Unlock()
	}
}

func tlsConfig(host string, skip bool) *tls.Config {
	return &tls.Config{ServerName: host, InsecureSkipVerify: skip, MinVersion: tls.VersionTLS12}
}

// Dial opens a new IMAP connection to the configured server (not logged in).
func Dial(ms settings.MailServer, opts *imapclient.Options) (*imapclient.Client, error) {
	if opts == nil {
		opts = &imapclient.Options{}
	}
	opts.TLSConfig = tlsConfig(ms.IMAP.Host, ms.SkipTLSVerify)
	opts.Dialer = &net.Dialer{Timeout: 15 * time.Second}
	addr := net.JoinHostPort(ms.IMAP.Host, strconv.Itoa(ms.IMAP.Port))
	switch ms.IMAP.Security {
	case "starttls":
		return imapclient.DialStartTLS(addr, opts)
	case "none":
		return imapclient.DialInsecure(addr, opts)
	default:
		return imapclient.DialTLS(addr, opts)
	}
}

// Login dials and authenticates, returning a ready client.
func Login(ms settings.MailServer, cr Creds, opts *imapclient.Options) (*imapclient.Client, error) {
	c, err := Dial(ms, opts)
	if err != nil {
		return nil, fmt.Errorf("connect to IMAP server: %w", err)
	}
	if err := c.Login(cr.Email, cr.Password).Wait(); err != nil {
		_ = c.Close()
		var ie *imap.Error
		if errors.As(err, &ie) {
			return nil, ErrAuth
		}
		return nil, err
	}
	return c, nil
}

// Do runs fn with the session's connection, connecting or reconnecting as needed.
func (p *Pool) Do(sessionID string, cr Creds, fn func(s *Session) error) error {
	p.mu.Lock()
	cn := p.conns[sessionID]
	if cn == nil {
		cn = &conn{}
		p.conns[sessionID] = cn
	}
	p.mu.Unlock()

	cn.mu.Lock()
	defer cn.mu.Unlock()
	cn.lastUsed = time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		if cn.c == nil || isClosed(cn.c) {
			c, err := Login(p.server(), cr, nil)
			if err != nil {
				return err
			}
			cn.c, cn.selected = c, ""
		}
		err := fn(&Session{c: cn.c, conn: cn, pool: p})
		if err != nil && isClosed(cn.c) && attempt == 0 {
			slog.Info("imap connection lost, reconnecting", "err", err)
			cn.c = nil
			continue
		}
		return err
	}
	return errors.New("imap: could not reconnect")
}

func isClosed(c *imapclient.Client) bool {
	select {
	case <-c.Closed():
		return true
	default:
		return false
	}
}
