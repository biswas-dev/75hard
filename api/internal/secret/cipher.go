// Package secret encrypts values that must be stored and later read back:
// third-party API keys, principally.
//
// This is deliberately not the package for passwords. A password is verified,
// never recovered, and belongs in bcrypt. These are credentials the server has
// to present to somebody else, so they have to be decryptable — which makes
// the threat model "someone reads the database file" rather than "someone
// cracks a hash".
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNoKey means encryption was attempted without a configured key.
var ErrNoKey = errors.New("secret: no encryption key configured")

// ErrCorrupt means the stored value could not be decrypted. It covers a wrong
// key and a tampered ciphertext alike, because GCM cannot tell them apart and
// the caller should treat both the same way.
var ErrCorrupt = errors.New("secret: value could not be decrypted")

// Cipher encrypts and decrypts with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New derives a cipher from a passphrase.
//
// The passphrase is hashed to a fixed 32 bytes rather than being required to
// be exactly that long, so an operator can set a normal secret string. SHA-256
// is enough here precisely because this is not a password: the input is
// expected to be a long random value from the environment, not something
// guessable, so a slow KDF would buy nothing.
func New(passphrase string) (*Cipher, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, ErrNoKey
	}
	sum := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("secret: building cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: building GCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns base64 of nonce||ciphertext.
//
// A fresh random nonce per call is what keeps GCM safe: reusing one with the
// same key is the failure that breaks the mode outright, so it is generated
// here and never derived from anything.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret: reading nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if c == nil {
		return "", ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrCorrupt
	}
	n := c.aead.NonceSize()
	if len(raw) < n {
		return "", ErrCorrupt
	}
	plaintext, err := c.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		// Wrong key or tampered bytes; GCM does not distinguish, and neither
		// should the caller.
		return "", ErrCorrupt
	}
	return string(plaintext), nil
}

// Hint returns the last four characters of a key, for recognising it in a list.
//
// Four characters identify which key is stored without being enough to use it.
// A short key is masked entirely rather than largely revealed.
func Hint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) < 8 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}
