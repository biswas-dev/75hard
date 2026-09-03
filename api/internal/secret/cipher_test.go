package secret

import (
	"errors"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	c, err := New("a-long-random-value-from-the-environment")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const key = "nvapi-abcdefghijklmnopqrstuvwxyz0123456789"
	enc, err := c.Encrypt(key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(enc, key) || strings.Contains(enc, "nvapi") {
		t.Fatal("the ciphertext contains the plaintext")
	}

	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != key {
		t.Errorf("round trip gave %q", got)
	}
}

func TestNonceIsFreshPerCall(t *testing.T) {
	// Reusing a nonce with the same key breaks GCM outright, so identical
	// plaintexts must not produce identical ciphertexts.
	c, _ := New("passphrase")
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatal("two encryptions of the same value are identical; the nonce is being reused")
	}
	for _, enc := range []string{a, b} {
		if got, err := c.Decrypt(enc); err != nil || got != "same" {
			t.Errorf("decrypt gave %q, %v", got, err)
		}
	}
}

func TestWrongKeyAndTamperingBothFail(t *testing.T) {
	c, _ := New("the-right-passphrase")
	enc, _ := c.Encrypt("secret-value")

	other, _ := New("a-different-passphrase")
	if _, err := other.Decrypt(enc); !errors.Is(err, ErrCorrupt) {
		t.Errorf("wrong key gave %v, want ErrCorrupt", err)
	}

	// Flip a byte in the middle of the ciphertext.
	bad := []byte(enc)
	bad[len(bad)/2] ^= 'x'
	if _, err := c.Decrypt(string(bad)); !errors.Is(err, ErrCorrupt) {
		t.Errorf("tampered value gave %v, want ErrCorrupt", err)
	}

	if _, err := c.Decrypt("not base64 at all !!"); !errors.Is(err, ErrCorrupt) {
		t.Errorf("garbage gave %v, want ErrCorrupt", err)
	}
	if _, err := c.Decrypt(""); !errors.Is(err, ErrCorrupt) {
		t.Errorf("empty gave %v, want ErrCorrupt", err)
	}
}

func TestNoKeyIsAnError(t *testing.T) {
	// An unconfigured instance must refuse rather than store plaintext.
	if _, err := New(""); !errors.Is(err, ErrNoKey) {
		t.Errorf("empty passphrase gave %v, want ErrNoKey", err)
	}
	if _, err := New("   "); !errors.Is(err, ErrNoKey) {
		t.Errorf("blank passphrase gave %v, want ErrNoKey", err)
	}

	var nilCipher *Cipher
	if _, err := nilCipher.Encrypt("x"); !errors.Is(err, ErrNoKey) {
		t.Errorf("nil cipher Encrypt gave %v, want ErrNoKey", err)
	}
	if _, err := nilCipher.Decrypt("x"); !errors.Is(err, ErrNoKey) {
		t.Errorf("nil cipher Decrypt gave %v, want ErrNoKey", err)
	}
}

func TestHint(t *testing.T) {
	// Enough to recognise which key is stored, useless for using it.
	if got := Hint("nvapi-abcdefghijklmnop6789"); got != "••••6789" {
		t.Errorf("Hint = %q", got)
	}
	// A short key is masked entirely rather than mostly revealed.
	if got := Hint("abc"); got != "••••" {
		t.Errorf("short key Hint = %q", got)
	}
	if got := Hint(""); got != "••••" {
		t.Errorf("empty Hint = %q", got)
	}
}
