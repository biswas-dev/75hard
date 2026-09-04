// Command admin performs operator tasks directly against the database.
//
// It exists so a locked-out account can be recovered on the server without
// hand-writing SQL and a bcrypt hash — the thing most likely to go wrong under
// pressure. Run it inside the container, where DB_PATH already points at the
// live database:
//
//	docker compose exec 75hard /app/admin reset-password you@example.com
//	docker compose exec 75hard /app/admin reset-password you@example.com 'a-password'
//	docker compose exec 75hard /app/admin list-users
package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
	"github.com/anchoo2kewl/75hard/api/internal/secret"
)

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	cfg := config.Load()
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fail("could not open the database at %s: %v", cfg.DBPath, err)
	}
	defer database.Close()

	switch args[0] {
	case "reset-password":
		if len(args) < 2 {
			fail("usage: admin reset-password <email> [new-password]")
		}
		password := ""
		if len(args) >= 3 {
			password = args[2]
		}
		if err := resetPassword(database, args[1], password); err != nil {
			fail("%v", err)
		}
	case "list-users":
		if err := listUsers(database); err != nil {
			fail("%v", err)
		}
	case "set-ai-key":
		if len(args) < 6 {
			fail("usage: admin set-ai-key <email> <slot> <provider> <model> <api-key>")
		}
		slot, err := strconv.Atoi(args[2])
		if err != nil {
			fail("slot must be a number: %v", err)
		}
		if err := setAIKey(database, cfg, args[1], slot, args[3], args[4], args[5]); err != nil {
			fail("%v", err)
		}
	case "delete-workout":
		if len(args) < 2 {
			fail("usage: admin delete-workout <id> [<id>...]")
		}
		if err := deleteWorkouts(database, args[1:]); err != nil {
			fail("%v", err)
		}
	case "list-workouts":
		if err := listWorkouts(database); err != nil {
			fail("%v", err)
		}
	case "list-ai-keys":
		if len(args) < 2 {
			fail("usage: admin list-ai-keys <email>")
		}
		if err := listAIKeys(database, args[1]); err != nil {
			fail("%v", err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

// resetPassword sets a new password for one account, generating one when none
// is supplied so an operator is never tempted to pick something weak.
func resetPassword(database *db.DB, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	var id int64
	var name string
	err := database.QueryRow(
		`SELECT id, name FROM users WHERE lower(email) = ? AND deleted_at IS NULL`, email).
		Scan(&id, &name)
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return fmt.Errorf("no active account for %s", email)
	}

	generated := false
	if strings.TrimSpace(password) == "" {
		password, err = generatePassword()
		if err != nil {
			return err
		}
		generated = true
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("could not hash the password: %w", err)
	}

	if _, err := database.Exec(
		`UPDATE users SET password_hash = ?, auth_provider = 'password', updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, hash, id); err != nil {
		return fmt.Errorf("could not update the password: %w", err)
	}

	// Any outstanding reset link is retired: the password just changed by
	// another route, so a live link is now an unwanted second key.
	if _, err := database.Exec(
		`UPDATE password_resets SET used_at = CURRENT_TIMESTAMP
		 WHERE user_id = ? AND used_at IS NULL`, id); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not retire outstanding reset links: %v\n", err)
	}

	fmt.Printf("Password updated for %s (id %d).\n", email, id)
	if generated {
		fmt.Printf("\n    %s\n\n", password)
		fmt.Println("That is the only time it is shown. Change it after signing in.")
	}
	return nil
}

func listUsers(database *db.DB) error {
	rows, err := database.Query(
		`SELECT id, email, name, timezone, is_admin, created_at
		 FROM users WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	fmt.Printf("%-4s %-32s %-18s %-20s %s\n", "ID", "EMAIL", "NAME", "TIMEZONE", "CREATED")
	for rows.Next() {
		var (
			id      int64
			email   string
			name    string
			tz      string
			admin   bool
			created string
		)
		if err := rows.Scan(&id, &email, &name, &tz, &admin, &created); err != nil {
			return err
		}
		if admin {
			name += " (admin)"
		}
		fmt.Printf("%-4d %-32s %-18s %-20s %s\n", id, email, name, tz, created)
	}
	return rows.Err()
}

// generatePassword returns a random password that is long enough not to need a
// complexity rule.
func generatePassword() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("could not generate a password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func usage() {
	fmt.Fprint(os.Stderr, `75hard admin

  admin reset-password <email> [new-password]
      Sets a password for an account. Generates a strong one when none is
      given, and prints it once.

  admin list-users
      Lists active accounts.

  admin set-ai-key <email> <slot> <provider> <model> <api-key>
      Stores an AI provider key for one account, encrypted with the server's
      own ENCRYPTION_KEY. Slot 1 is tried first, then 2, then 3.

  admin list-workouts
      Lists logged workouts with their ids.

  admin delete-workout <id> [<id>...]
      Removes logged workouts. The day is re-scored on the next sync.

  admin list-ai-keys <email>
      Shows which slots are configured. Keys are never decrypted.

Reads DB_PATH from the environment, as the server does.
`)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// setAIKey stores an encrypted provider key against one account's slot.
//
// It exists so the operator can seed an account's keys without pasting them
// through the UI or, worse, writing ciphertext by hand: the key is encrypted
// here with the same cipher the server uses, from the same ENCRYPTION_KEY, so
// what lands in the table is exactly what the app would have written.
func setAIKey(database *db.DB, cfg *config.Config, email string, slot int, provider, model, apiKey string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if slot < 1 || slot > 3 {
		return fmt.Errorf("slot must be 1, 2 or 3")
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("the api key is empty")
	}
	if strings.TrimSpace(cfg.EncryptionKey) == "" {
		return fmt.Errorf("ENCRYPTION_KEY is not set, so the key cannot be stored")
	}

	cipher, err := secret.New(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("could not build the cipher: %w", err)
	}
	enc, err := cipher.Encrypt(apiKey)
	if err != nil {
		return fmt.Errorf("could not encrypt the key: %w", err)
	}

	var id int64
	if err := database.QueryRow(
		`SELECT id FROM users WHERE lower(email) = ? AND deleted_at IS NULL`, email).
		Scan(&id); err != nil {
		return fmt.Errorf("no active account for %s", email)
	}

	if _, err := database.Exec(
		`INSERT INTO user_ai_providers (user_id, slot, provider, model, api_key_enc, key_hint, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, 1)
		 ON CONFLICT(user_id, slot) DO UPDATE SET
		     provider    = excluded.provider,
		     model       = excluded.model,
		     api_key_enc = excluded.api_key_enc,
		     key_hint    = excluded.key_hint,
		     enabled     = 1,
		     updated_at  = CURRENT_TIMESTAMP`,
		id, slot, provider, model, enc, secret.Hint(apiKey)); err != nil {
		return fmt.Errorf("could not store the key: %w", err)
	}

	fmt.Printf("slot %d for %s: %s (%s) key %s\n", slot, email, provider, model, secret.Hint(apiKey))
	return nil
}

// listAIKeys shows what is configured, without ever decrypting anything.
func listAIKeys(database *db.DB, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	rows, err := database.Query(
		`SELECT p.slot, p.provider, p.model, p.key_hint, p.enabled
		   FROM user_ai_providers p JOIN users u ON u.id = p.user_id
		  WHERE lower(u.email) = ? ORDER BY p.slot`, email)
	if err != nil {
		return err
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var slot, enabled int
		var provider, model, hint string
		if err := rows.Scan(&slot, &provider, &model, &hint, &enabled); err != nil {
			return err
		}
		state := "enabled"
		if enabled == 0 {
			state = "disabled"
		}
		fmt.Printf("slot %d  %-10s %-32s %-10s %s\n", slot, provider, model, hint, state)
		found = true
	}
	if !found {
		fmt.Printf("no provider keys stored for %s\n", email)
	}
	return rows.Err()
}

// listWorkouts prints logged sessions with their ids.
func listWorkouts(database *db.DB) error {
	rows, err := database.Query(`
		SELECT w.id, d.day_number, w.kind, w.activity, w.minutes,
		       COALESCE(w.started_at, ''), w.created_at
		  FROM workouts w JOIN days d ON d.id = w.day_id
		 ORDER BY d.day_number, w.id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var day, minutes int
		var kind, activity, started, created string
		if err := rows.Scan(&id, &day, &kind, &activity, &minutes, &started, &created); err != nil {
			return err
		}
		if started == "" {
			started = "-"
		}
		fmt.Printf("%4d  day %-3d %-8s %-20s %3d min  started %s\n",
			id, day, kind, activity, minutes, started)
	}
	return rows.Err()
}

// deleteWorkouts removes logged sessions by id.
//
// Deliberately narrow: it takes explicit ids and reports what each one was, so
// an operator can see what is going before it goes.
func deleteWorkouts(database *db.DB, ids []string) error {
	for _, raw := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return fmt.Errorf("%q is not a workout id", raw)
		}

		var activity, kind string
		var minutes int
		if err := database.QueryRow(
			`SELECT activity, kind, minutes FROM workouts WHERE id = ?`, id).
			Scan(&activity, &kind, &minutes); err != nil {
			return fmt.Errorf("no workout %d", id)
		}
		if _, err := database.Exec(`DELETE FROM workouts WHERE id = ?`, id); err != nil {
			return fmt.Errorf("could not delete workout %d: %w", id, err)
		}
		fmt.Printf("deleted %d: %s %s %d min\n", id, kind, activity, minutes)
	}
	return nil
}
