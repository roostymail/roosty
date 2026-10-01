// Package secure holds the small set of cryptographic helpers Roosty uses:
// random tokens, sealing IMAP credentials with a per-session key, password
// hashing for admins and HMAC signatures for proxied URLs.
package secure

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

var b64 = base64.RawURLEncoding

// RandomBytes returns n cryptographically random bytes.
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

// Token returns a URL-safe random token with n bytes of entropy.
func Token(n int) string { return b64.EncodeToString(RandomBytes(n)) }

// Seal encrypts plaintext with key (32 bytes) using XChaCha20-Poly1305.
func Seal(key, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := RandomBytes(aead.NonceSize())
	return append(nonce, aead.Seal(nil, nonce, plaintext, nil)...), nil
}

// Open decrypts data produced by Seal.
func Open(key, data []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	if len(data) < aead.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:aead.NonceSize()], data[aead.NonceSize():]
	return aead.Open(nil, nonce, ct, nil)
}

// HashPassword returns an encoded argon2id hash.
func HashPassword(password string) string {
	salt := RandomBytes(16)
	sum := argon2.IDKey([]byte(password), salt, 2, 64*1024, 2, 32)
	return fmt.Sprintf("argon2id$%s$%s", b64.EncodeToString(salt), b64.EncodeToString(sum))
}

// CheckPassword verifies a password against a hash from HashPassword.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err1 := b64.DecodeString(parts[1])
	want, err2 := b64.DecodeString(parts[2])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 2, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Sign returns a short URL-safe HMAC-SHA256 signature of msg.
func Sign(key []byte, msg string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return b64.EncodeToString(m.Sum(nil)[:18])
}

// Verify checks a signature produced by Sign.
func Verify(key []byte, msg, sig string) bool {
	return hmac.Equal([]byte(Sign(key, msg)), []byte(sig))
}

// Equal compares two strings in constant time.
func Equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// EncodeKey and DecodeKey convert binary keys for cookies.
func EncodeKey(k []byte) string           { return b64.EncodeToString(k) }
func DecodeKey(s string) ([]byte, error)  { return b64.DecodeString(s) }
