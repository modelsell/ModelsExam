// Package secretbox encrypts saved API keys at rest with AES-256-GCM. The
// master key lives outside the database (MODEL_CHECK_SECRET_KEY), so the
// database and its backups hold only ciphertext. The server still decrypts
// a key right before it uses it for a check.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

// prefix marks the format of a sealed value; plaintext rows from before
// encryption have none.
const prefix = "v1:"

var (
	ErrKey    = errors.New("MODEL_CHECK_SECRET_KEY must be 32 random bytes, base64 encoded (e.g. openssl rand -base64 32)")
	ErrSealed = errors.New("saved key cannot be decrypted with the configured master key")
)

type Box struct{ aead cipher.AEAD }

// New parses a base64 (standard or URL, padded or not) 32-byte master key.
func New(encoded string) (*Box, error) {
	encoded = strings.TrimSpace(encoded)
	var key []byte
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(encoded); err == nil {
			key = k
			break
		}
	}
	if len(key) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// NewKey returns a fresh master key, base64 encoded.
func NewKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// Sealed reports whether a stored value is already encrypted.
func Sealed(value string) bool { return strings.HasPrefix(value, prefix) }

// Seal encrypts plaintext. aad binds the ciphertext to its row (credential
// id and owner), so a value copied into another row does not decrypt.
func (b *Box) Seal(plaintext, aad string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plaintext), []byte(aad))
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

func (b *Box) Open(sealed, aad string) (string, error) {
	if !Sealed(sealed) {
		return "", ErrSealed
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, prefix))
	n := b.aead.NonceSize()
	if err != nil || len(raw) < n {
		return "", ErrSealed
	}
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], []byte(aad))
	if err != nil {
		return "", ErrSealed
	}
	return string(plain), nil
}
