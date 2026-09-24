// Package auth provides the local account and browser-session store.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	passwordVersion = 1
	passwordRounds  = 210_000
	saltSize        = 16
	keySize         = 32
	sessionSize     = 32
	sessionTTL      = 30 * 24 * time.Hour
)

var (
	ErrAccountExists      = errors.New("auth: account already exists")
	ErrNoAccount          = errors.New("auth: no account has been created")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
)

type accountFile struct {
	Version      int                      `json:"version"`
	Username     string                   `json:"username"`
	Salt         string                   `json:"salt"`
	PasswordHash string                   `json:"passwordHash"`
	CreatedAt    time.Time                `json:"createdAt"`
	Sessions     map[string]sessionRecord `json:"sessions,omitempty"`
}

type sessionRecord struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Store keeps one local account on disk and short-lived sessions in memory.
// Passwords are never retained after Setup or Authenticate returns; sessions
// are intentionally invalidated when the server restarts.
type Store struct {
	path string

	mu       sync.RWMutex
	account  *accountFile
	sessions map[string]sessionRecord
}

// Open loads an account store. A missing file means first-run setup is needed.
func Open(path string) (*Store, error) {
	s := &Store{path: path, sessions: make(map[string]sessionRecord)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read account store: %w", err)
	}
	var account accountFile
	if err := json.Unmarshal(data, &account); err != nil {
		return nil, fmt.Errorf("parse account store: %w", err)
	}
	if err := validateAccount(&account); err != nil {
		return nil, err
	}
	s.account = &account
	now := time.Now().UTC()
	for token, item := range account.Sessions {
		if len(token) != sessionSize*2 || item.Username != account.Username || !now.Before(item.ExpiresAt) {
			continue
		}
		if _, err := hex.DecodeString(token); err != nil {
			continue
		}
		s.sessions[token] = item
	}
	return s, nil
}

func validateAccount(account *accountFile) error {
	if account.Version != passwordVersion {
		return fmt.Errorf("auth: unsupported account version %d", account.Version)
	}
	if !validUsername(account.Username) {
		return errors.New("auth: invalid stored username")
	}
	salt, err := base64.RawStdEncoding.DecodeString(account.Salt)
	if err != nil {
		return fmt.Errorf("auth: invalid stored salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(account.PasswordHash)
	if err != nil || len(hash) != keySize {
		return errors.New("auth: invalid stored password hash")
	}
	if len(salt) != saltSize {
		return errors.New("auth: invalid stored salt length")
	}
	return nil
}

func validUsername(username string) bool {
	return username != "" && len(username) <= 64 && !strings.ContainsAny(username, "\r\n\t ")
}

func validPassword(password string) bool { return len(password) >= 12 && len(password) <= 1024 }

// HasAccount reports whether first-run account setup has been completed.
func (s *Store) HasAccount() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.account != nil
}

// Username returns the configured account name, if setup is complete.
func (s *Store) Username() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.account == nil {
		return ""
	}
	return s.account.Username
}

// Setup creates the first account and persists only its salted password hash.
func (s *Store) Setup(username, password string) error {
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return errors.New("auth: username must be 1-64 characters without whitespace")
	}
	if !validPassword(password) {
		return errors.New("auth: password must be between 12 and 1024 characters")
	}
	salt, err := randomBytes(saltSize)
	if err != nil {
		return fmt.Errorf("auth: generate password salt: %w", err)
	}
	account := &accountFile{
		Version:      passwordVersion,
		Username:     username,
		Salt:         base64.RawStdEncoding.EncodeToString(salt),
		PasswordHash: base64.RawStdEncoding.EncodeToString(deriveKey(password, salt)),
		CreatedAt:    time.Now().UTC(),
		Sessions:     make(map[string]sessionRecord),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account != nil {
		return ErrAccountExists
	}
	if err := writeAccount(s.path, account); err != nil {
		return err
	}
	s.account = account
	return nil
}

// Authenticate verifies credentials without exposing whether the username or
// password was the incorrect part.
func (s *Store) Authenticate(username, password string) error {
	username = strings.TrimSpace(username)
	s.mu.RLock()
	account := s.account
	if account == nil {
		s.mu.RUnlock()
		return ErrNoAccount
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(account.Salt)
	want, hashErr := base64.RawStdEncoding.DecodeString(account.PasswordHash)
	storedUsername := account.Username
	s.mu.RUnlock()
	if saltErr != nil || hashErr != nil || len(want) != keySize || username != storedUsername {
		return ErrInvalidCredentials
	}
	got := deriveKey(password, salt)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrInvalidCredentials
	}
	return nil
}

// CreateSession issues an in-memory session token after successful login.
func (s *Store) CreateSession(username string) (string, time.Time, error) {
	username = strings.TrimSpace(username)
	raw, err := randomBytes(sessionSize)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: generate session: %w", err)
	}
	token := hex.EncodeToString(raw)
	expires := time.Now().UTC().Add(sessionTTL)
	item := sessionRecord{Username: username, ExpiresAt: expires}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil || s.account.Username != username {
		return "", time.Time{}, ErrInvalidCredentials
	}
	s.sessions[token] = item
	if err := s.persistSessionsLocked(); err != nil {
		delete(s.sessions, token)
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// ValidSession validates a session token and returns its account name.
func (s *Store) ValidSession(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.sessions[token]
	if !ok {
		return "", false
	}
	if !now.Before(item.ExpiresAt) {
		delete(s.sessions, token)
		_ = s.persistSessionsLocked()
		return "", false
	}
	return item.Username, true
}

// Revoke removes a session token.
func (s *Store) Revoke(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	item, ok := s.sessions[token]
	delete(s.sessions, token)
	if ok && s.persistSessionsLocked() != nil {
		s.sessions[token] = item
	}
	s.mu.Unlock()
}

func (s *Store) persistSessionsLocked() error {
	if s.account == nil {
		return ErrNoAccount
	}
	updated := *s.account
	updated.Sessions = make(map[string]sessionRecord, len(s.sessions))
	for token, item := range s.sessions {
		updated.Sessions[token] = item
	}
	if err := writeAccount(s.path, &updated); err != nil {
		return err
	}
	s.account = &updated
	return nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

// deriveKey is PBKDF2-HMAC-SHA256. Keeping it here avoids making the server's
// small local deployment depend on a second crypto module.
func deriveKey(password string, salt []byte) []byte {
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = mac.Write(salt)
	var counter [4]byte
	binary.BigEndian.PutUint32(counter[:], 1)
	_, _ = mac.Write(counter[:])
	u := mac.Sum(nil)
	t := append([]byte(nil), u...)
	for i := 1; i < passwordRounds; i++ {
		mac = hmac.New(sha256.New, []byte(password))
		_, _ = mac.Write(u)
		u = mac.Sum(nil)
		for j := range t {
			t[j] ^= u[j]
		}
	}
	return t[:keySize]
}

func writeAccount(path string, account *accountFile) error {
	data, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return fmt.Errorf("auth: encode account: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("auth: create account directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".account-*.tmp")
	if err != nil {
		return fmt.Errorf("auth: create account temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("auth: write account: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("auth: sync account: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("auth: close account: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("auth: install account: %w", err)
	}
	return nil
}
