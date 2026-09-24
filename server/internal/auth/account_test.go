package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupAuthenticateAndSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.HasAccount() {
		t.Fatal("new store already has an account")
	}
	if err := store.Setup("sean", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if !store.HasAccount() || store.Username() != "sean" {
		t.Fatal("account was not installed")
	}
	if err := store.Authenticate("sean", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Authenticate("sean", "wrong-password"), ErrInvalidCredentials) {
		t.Fatal("wrong password was accepted")
	}
	token, _, err := store.CreateSession("sean")
	if err != nil {
		t.Fatal(err)
	}
	if user, ok := store.ValidSession(token); !ok || user != "sean" {
		t.Fatalf("session invalid: user=%q ok=%v", user, ok)
	}
	store.Revoke(token)
	if _, ok := store.ValidSession(token); ok {
		t.Fatal("revoked session remained valid")
	}
	persistedToken, _, err := store.CreateSession("sean")
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Authenticate("sean", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if user, ok := reloaded.ValidSession(persistedToken); !ok || user != "sean" {
		t.Fatalf("persisted session did not survive restart: user=%q ok=%v", user, ok)
	}
	reloaded.Revoke(persistedToken)
	final, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.ValidSession(persistedToken); ok {
		t.Fatal("revoked session survived restart")
	}
	mode, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode.Mode().Perm() != 0o600 {
		t.Fatalf("account permissions = %o, want 600", mode.Mode().Perm())
	}
}

func TestSetupIsOneTimeAndPasswordPolicy(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "account.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Setup("user", "short"); err == nil {
		t.Fatal("short password accepted")
	}
	if err := store.Setup("user", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(store.Setup("other", "another-long-password"), ErrAccountExists) {
		t.Fatal("second account setup was accepted")
	}
}
