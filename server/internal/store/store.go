// Package store keeps Roosty's own small state in SQLite: settings, admins,
// user sessions, known accounts and preferences. Mail itself stays on the
// mail server.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

var ErrNotFound = errors.New("not found")

const schema = `
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS admins (
  id INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS admin_sessions (
  id TEXT PRIMARY KEY, admin_id INTEGER NOT NULL, created_at INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS accounts (
  email TEXT PRIMARY KEY, first_seen INTEGER NOT NULL, last_login INTEGER NOT NULL, blocked INTEGER NOT NULL DEFAULT 0,
  prefs TEXT NOT NULL DEFAULT '{}');
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY, email TEXT NOT NULL, secret BLOB NOT NULL, user_agent TEXT, ip TEXT,
  created_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS sessions_email ON sessions(email);
`

func Open(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "roosty.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().Unix() }

// ---- settings (JSON values) ----

func (s *Store) GetJSON(key string, v any) error {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), v)
}

func (s *Store) SetJSON(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(raw))
	return err
}

// ---- admins ----

type Admin struct {
	ID           int64
	Username     string
	PasswordHash string
}

func (s *Store) AdminCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(username, hash string) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO admins(username,password_hash,created_at) VALUES(?,?,?)`, username, hash, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AdminByUsername(username string) (*Admin, error) {
	a := &Admin{}
	err := s.db.QueryRow(`SELECT id,username,password_hash FROM admins WHERE username=?`, username).Scan(&a.ID, &a.Username, &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) CreateAdminSession(id string, adminID int64, ttl time.Duration) error {
	_, err := s.db.Exec(`INSERT INTO admin_sessions(id,admin_id,created_at,expires_at) VALUES(?,?,?,?)`, id, adminID, now(), now()+int64(ttl.Seconds()))
	return err
}

func (s *Store) AdminSession(id string) (*Admin, error) {
	a := &Admin{}
	err := s.db.QueryRow(`SELECT a.id,a.username,a.password_hash FROM admin_sessions s JOIN admins a ON a.id=s.admin_id WHERE s.id=? AND s.expires_at>?`, id, now()).
		Scan(&a.ID, &a.Username, &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) DeleteAdminSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM admin_sessions WHERE id=?`, id)
	return err
}

// ---- accounts and sessions ----

type Account struct {
	Email     string          `json:"email"`
	FirstSeen int64           `json:"firstSeen"`
	LastLogin int64           `json:"lastLogin"`
	Blocked   bool            `json:"blocked"`
	Sessions  int             `json:"sessions"`
	Prefs     json.RawMessage `json:"-"`
}

func (s *Store) TouchAccount(email string) error {
	_, err := s.db.Exec(`INSERT INTO accounts(email,first_seen,last_login) VALUES(?,?,?)
		ON CONFLICT(email) DO UPDATE SET last_login=excluded.last_login`, email, now(), now())
	return err
}

func (s *Store) IsBlocked(email string) bool {
	var b int
	_ = s.db.QueryRow(`SELECT blocked FROM accounts WHERE email=?`, email).Scan(&b)
	return b == 1
}

func (s *Store) SetBlocked(email string, blocked bool) error {
	v := 0
	if blocked {
		v = 1
	}
	_, err := s.db.Exec(`INSERT INTO accounts(email,first_seen,last_login,blocked) VALUES(?,?,0,?)
		ON CONFLICT(email) DO UPDATE SET blocked=excluded.blocked`, email, now(), v)
	return err
}

func (s *Store) Accounts() ([]Account, error) {
	rows, err := s.db.Query(`SELECT a.email,a.first_seen,a.last_login,a.blocked,
		(SELECT COUNT(*) FROM sessions s WHERE s.email=a.email AND s.expires_at>?) FROM accounts a ORDER BY a.last_login DESC`, now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var a Account
		var b int
		if err := rows.Scan(&a.Email, &a.FirstSeen, &a.LastLogin, &b, &a.Sessions); err != nil {
			return nil, err
		}
		a.Blocked = b == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Prefs(email string) (json.RawMessage, error) {
	var raw string
	err := s.db.QueryRow(`SELECT prefs FROM accounts WHERE email=?`, email).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(raw), err
}

func (s *Store) SetPrefs(email string, prefs json.RawMessage) error {
	_, err := s.db.Exec(`UPDATE accounts SET prefs=? WHERE email=?`, string(prefs), email)
	return err
}

type Session struct {
	ID        string
	Email     string
	Secret    []byte
	CreatedAt int64
	ExpiresAt int64
}

func (s *Store) CreateSession(sess Session, ua, ip string) error {
	_, err := s.db.Exec(`INSERT INTO sessions(id,email,secret,user_agent,ip,created_at,last_seen,expires_at) VALUES(?,?,?,?,?,?,?,?)`,
		sess.ID, sess.Email, sess.Secret, ua, ip, now(), now(), sess.ExpiresAt)
	return err
}

func (s *Store) Session(id string) (*Session, error) {
	x := &Session{}
	err := s.db.QueryRow(`SELECT id,email,secret,created_at,expires_at FROM sessions WHERE id=? AND expires_at>?`, id, now()).
		Scan(&x.ID, &x.Email, &x.Secret, &x.CreatedAt, &x.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return x, err
}

func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id=?`, id)
	return err
}

// DeleteSessionsFor removes every session of an account and returns their IDs.
func (s *Store) DeleteSessionsFor(email string) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM sessions WHERE email=?`, email)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	_, err = s.db.Exec(`DELETE FROM sessions WHERE email=?`, email)
	return ids, err
}

func (s *Store) PurgeExpired() {
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at<=?`, now())
	_, _ = s.db.Exec(`DELETE FROM admin_sessions WHERE expires_at<=?`, now())
}

func (s *Store) Counts() (accounts, sessions int) {
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE expires_at>?`, now()).Scan(&sessions)
	return
}
