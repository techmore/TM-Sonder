package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestOpenMigratesLegacyAccountWithoutLosingPasswordOrSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.json")
	password := "a-long-test-password"
	salt, err := randomBytes(saltSize)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", sessionSize*2)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	created := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	legacy := legacyAccountFile{
		Version:      passwordVersion,
		Username:     "sean",
		Salt:         base64.RawStdEncoding.EncodeToString(salt),
		PasswordHash: base64.RawStdEncoding.EncodeToString(deriveKey(password, salt)),
		CreatedAt:    created,
		Sessions: map[string]sessionRecord{
			token: {Username: "sean", ExpiresAt: expires},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Authenticate("sean", password); err != nil {
		t.Fatalf("legacy password was not retained: %v", err)
	}
	if username, ok := store.ValidSession(token); !ok || username != "sean" {
		t.Fatalf("legacy session was not retained: username=%q valid=%v", username, ok)
	}

	var upgraded accountFile
	upgradedData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(upgradedData, &upgraded); err != nil {
		t.Fatal(err)
	}
	if upgraded.Version != storeVersion || upgraded.Owner != "sean" {
		t.Fatalf("migrated account metadata = version %d, owner %q", upgraded.Version, upgraded.Owner)
	}
	if upgraded.Accounts["sean"].CreatedAt != created {
		t.Fatalf("account creation time changed during migration: got %s want %s", upgraded.Accounts["sean"].CreatedAt, created)
	}
}

func TestOpenRejectsUnknownVersionWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account.json")
	original := []byte(`{"version":99,"preserve":"untouched"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("unsupported account version was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("unsupported account store was modified: %s", after)
	}
}
