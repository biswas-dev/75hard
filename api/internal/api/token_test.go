package api

import (
	"net/http"
	"strings"
	"testing"

	goapi "github.com/anchoo2kewl/go-api"
)

func TestTokenSchemeIsThisApplication(t *testing.T) {
	// The prefix is what makes a token recognisable as 75hard's rather than
	// another app's, so it is worth pinning.
	cred, err := TokenScheme.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(cred.Plaintext, "75h_") {
		t.Errorf("token %q does not carry the 75hard prefix", cred.Prefix)
	}
	if !TokenScheme.Issued(cred.Plaintext) {
		t.Error("the scheme does not recognise its own token")
	}
	// folioworth's tokens must not authenticate here.
	other, _ := goapi.NewScheme("fw_").Generate()
	if TokenScheme.Issued(other.Plaintext) {
		t.Error("a folioworth token was accepted as a 75hard one")
	}
	// A session JWT shares the header and must not be routed to the token path.
	if TokenScheme.Issued("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig") {
		t.Error("a JWT was mistaken for an API token")
	}
}

func TestReadScopeBlocksWrites(t *testing.T) {
	// Enforcement lives in go-api; this pins the behaviour this app depends on,
	// so an upgrade that loosened it would fail here rather than in production.
	read := goapi.Scopes{goapi.ScopeRead}

	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if err := read.AllowsMethod(m); err != nil {
			t.Errorf("%s refused for a read token: %v", m, err)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		if err := read.AllowsMethod(m); err == nil {
			t.Errorf("%s allowed for a read-only token", m)
		}
	}

	write := goapi.Scopes{goapi.ScopeWrite}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		if err := write.AllowsMethod(m); err != nil {
			t.Errorf("%s refused for a write token: %v", m, err)
		}
	}
}
