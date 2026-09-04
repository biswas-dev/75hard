package api

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	gologin "github.com/anchoo2kewl/go-login"
	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
	"github.com/anchoo2kewl/75hard/api/internal/secret"
	"go.uber.org/zap"
)

func twoFactorServer(t *testing.T) *Server {
	t.Helper()

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return &Server{
		db:  database,
		log: zap.NewNop(),
		cfg: &config.Config{
			JWTSecret:     "session-signing-secret-for-tests",
			JWTExpiry:     time.Hour,
			EncryptionKey: "an-encryption-key-for-tests",
			AppURL:        "https://75hard.example",
		},
	}
}

// A challenge token proves a password was right and nothing more. If the
// session key validated it, anyone who got past the first factor would hold a
// working session without ever presenting the second.
func TestChallengeTokenIsNotASessionToken(t *testing.T) {
	s := twoFactorServer(t)

	challenge, err := auth.GenerateToken(1, "a@b.c", s.challengeSecret(), time.Minute)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := auth.ValidateToken(challenge, s.cfg.JWTSecret); err == nil {
		t.Fatal("a two-factor challenge validated as a session token")
	}

	session, err := auth.GenerateToken(1, "a@b.c", s.cfg.JWTSecret, time.Minute)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := auth.ValidateToken(session, s.challengeSecret()); err == nil {
		t.Fatal("a session token validated as a two-factor challenge")
	}
}

func seedTwoFactorUser(t *testing.T, s *Server) (userID int64, totpSecret string) {
	t.Helper()
	ctx := context.Background()

	totpSecret, err := gologin.NewTOTPSecret()
	if err != nil {
		t.Fatalf("secret: %v", err)
	}
	cipher, err := secret.New(s.cfg.EncryptionKey)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	enc, err := cipher.Encrypt(totpSecret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO users (email, name, password_hash, totp_secret_enc, totp_enabled)
		VALUES ('a@b.c', 'A', 'x', ?, 1)`, enc)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, _ := res.LastInsertId()
	return id, totpSecret
}

func TestSecondFactorAcceptsACurrentCode(t *testing.T) {
	s := twoFactorServer(t)
	userID, totpSecret := seedTwoFactorUser(t, s)
	r := httptest.NewRequest("POST", "/", nil)

	code, err := gologin.TOTPCode(totpSecret, time.Now())
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	ok, err := s.consumeSecondFactor(r, userID, code)
	if err != nil || !ok {
		t.Fatalf("a current code was refused: ok=%v err=%v", ok, err)
	}

	if ok, _ := s.consumeSecondFactor(r, userID, "000000"); ok {
		t.Error("a wrong code was accepted")
	}
}

// A recovery code is for a lost phone, and works exactly once. Accepting one
// twice would turn a code read off a printout into a permanent second key.
func TestRecoveryCodeIsSpentOnUse(t *testing.T) {
	s := twoFactorServer(t)
	userID, _ := seedTwoFactorUser(t, s)
	r := httptest.NewRequest("POST", "/", nil)

	codes, hashes, err := gologin.NewRecoveryCodes()
	if err != nil {
		t.Fatalf("codes: %v", err)
	}
	for _, h := range hashes {
		if _, err := s.db.Exec(
			`INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, ?)`, userID, h); err != nil {
			t.Fatalf("seed codes: %v", err)
		}
	}

	ok, err := s.consumeSecondFactor(r, userID, codes[0])
	if err != nil || !ok {
		t.Fatalf("a recovery code was refused: ok=%v err=%v", ok, err)
	}
	if ok, _ := s.consumeSecondFactor(r, userID, codes[0]); ok {
		t.Error("the same recovery code worked twice")
	}
	if ok, _ := s.consumeSecondFactor(r, userID, codes[1]); !ok {
		t.Error("a second, unused recovery code was refused")
	}
}

// An account without two-factor must not be openable by a code, or a stolen
// challenge plus any guess would be a way in.
func TestSecondFactorRefusesAccountsWithoutIt(t *testing.T) {
	s := twoFactorServer(t)
	res, err := s.db.Exec(
		`INSERT INTO users (email, name, password_hash) VALUES ('c@d.e', 'C', 'x')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, _ := res.LastInsertId()

	if ok, _ := s.consumeSecondFactor(httptest.NewRequest("POST", "/", nil), id, "123456"); ok {
		t.Error("a code opened an account with no second factor")
	}
}

// A registration challenge belongs to one account, and a sign-in challenge to
// none. Crossing them would let one account finish another's enrolment.
func TestPasskeySessionsAreBoundToTheirOwner(t *testing.T) {
	s := twoFactorServer(t)
	ctx := context.Background()

	res, _ := s.db.Exec(`INSERT INTO users (email, name, password_hash) VALUES ('a@b.c','A','x')`)
	mine, _ := res.LastInsertId()
	res, _ = s.db.Exec(`INSERT INTO users (email, name, password_hash) VALUES ('b@c.d','B','x')`)
	theirs, _ := res.LastInsertId()

	id, err := s.storePasskeySession(ctx, &mine, []byte("challenge"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, ok := s.takePasskeySession(ctx, id, &theirs); ok {
		t.Error("another account finished a registration that was not theirs")
	}

	id, _ = s.storePasskeySession(ctx, &mine, []byte("challenge"))
	if _, ok := s.takePasskeySession(ctx, id, nil); ok {
		t.Error("a registration challenge was accepted as a sign-in")
	}

	id, _ = s.storePasskeySession(ctx, nil, []byte("challenge"))
	if _, ok := s.takePasskeySession(ctx, id, &mine); ok {
		t.Error("a sign-in challenge was accepted as a registration")
	}
}

// A challenge is what the signature is checked against, so it must not survive
// its own use.
func TestPasskeySessionCannotBeReplayed(t *testing.T) {
	s := twoFactorServer(t)
	ctx := context.Background()

	id, err := s.storePasskeySession(ctx, nil, []byte("challenge"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, ok := s.takePasskeySession(ctx, id, nil); !ok {
		t.Fatal("a fresh session was refused")
	}
	if _, ok := s.takePasskeySession(ctx, id, nil); ok {
		t.Error("the same challenge was accepted twice")
	}
}
