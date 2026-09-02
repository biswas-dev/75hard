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
	"strings"

	"github.com/anchoo2kewl/75hard/api/internal/auth"
	"github.com/anchoo2kewl/75hard/api/internal/config"
	"github.com/anchoo2kewl/75hard/api/internal/db"
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

Reads DB_PATH from the environment, as the server does.
`)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
