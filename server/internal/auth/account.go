// Package auth provides local accounts and browser sessions.
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
	storeVersion    = 2
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
	ErrInvalidInvite      = errors.New("auth: invalid invite link")
	ErrUsernameExists     = errors.New("auth: username already exists")
)

// accountFile is version 2 of the local account store. The first account is
// retained as Owner for the legacy media-client compatibility login.
type accountFile struct {
	Version  int                      `json:"version"`
	Owner    string                   `json:"owner"`
	Accounts map[string]accountRecord `json:"accounts"`
	Sessions map[string]sessionRecord `json:"sessions,omitempty"`
}

type accountRecord struct {
	Salt         string    `json:"salt"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
	InviteCode   string    `json:"inviteCode"`
	ReferredBy   string    `json:"referredBy,omitempty"`
}

// legacyAccountFile is the original single-account on-disk format.
type legacyAccountFile struct {
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

// Store keeps local accounts on disk and short-lived sessions in memory.
// Passwords are never retained after Setup, Signup, or Authenticate returns.
type Store struct {
	path string

	mu       sync.RWMutex
	account  *accountFile
	sessions map[string]sessionRecord
}

// Open loads an account store. A missing file means first-run setup is needed.
// Existing single-account stores are upgraded in place without changing their
// password or valid sessions.
func Open(path string) (*Store, error) {
	s := &Store{path: path, sessions: make(map[string]sessionRecord)}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read account store: %w", err)
	}
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("parse account store: %w", err)
	}

	now := time.Now().UTC()
	switch version.Version {
	case passwordVersion:
		var legacy legacyAccountFile
		if err := json.Unmarshal(data, &legacy); err != nil {
			return nil, fmt.Errorf("parse account store: %w", err)
		}
		if err := validateLegacyAccount(&legacy); err != nil {
			return nil, err
		}
		inviteCode, err := newInviteCode()
		if err != nil {
			return nil, fmt.Errorf("auth: generate invite code: %w", err)
		}
		upgraded := &accountFile{
			Version: storeVersion,
			Owner:   legacy.Username,
			Accounts: map[string]accountRecord{
				legacy.Username: {
					Salt: legacy.Salt, PasswordHash: legacy.PasswordHash,
					CreatedAt: legacy.CreatedAt, InviteCode: inviteCode,
				},
			},
			Sessions: make(map[string]sessionRecord),
		}
		for token, item := range legacy.Sessions {
			if validStoredSession(token, item, upgraded.Accounts, now) {
				upgraded.Sessions[token] = item
				s.sessions[token] = item
			}
		}
		if err := writeAccount(path, upgraded); err != nil {
			return nil, fmt.Errorf("auth: upgrade account store: %w", err)
		}
		s.account = upgraded
	case storeVersion:
		var account accountFile
		if err := json.Unmarshal(data, &account); err != nil {
			return nil, fmt.Errorf("parse account store: %w", err)
		}
		if err := validateAccount(&account); err != nil {
			return nil, err
		}
		s.account = &account
		for token, item := range account.Sessions {
			if validStoredSession(token, item, account.Accounts, now) {
				s.sessions[token] = item
			}
		}
	default:
		return nil, fmt.Errorf("auth: unsupported account version %d", version.Version)
	}
	return s, nil
}

func validStoredSession(token string, item sessionRecord, accounts map[string]accountRecord, now time.Time) bool {
	if len(token) != sessionSize*2 || accounts[item.Username].PasswordHash == "" || !now.Before(item.ExpiresAt) {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func validateLegacyAccount(account *legacyAccountFile) error {
	if account.Version != passwordVersion {
		return fmt.Errorf("auth: unsupported account version %d", account.Version)
	}
	if !validUsername(account.Username) {
		return errors.New("auth: invalid stored username")
	}
	return validatePasswordRecord(account.Salt, account.PasswordHash)
}

func validateAccount(account *accountFile) error {
	if account.Version != storeVersion {
		return fmt.Errorf("auth: unsupported account version %d", account.Version)
	}
	if len(account.Accounts) == 0 {
		return errors.New("auth: account store has no accounts")
	}
	if !validUsername(account.Owner) {
		return errors.New("auth: invalid stored owner")
	}
	if _, ok := account.Accounts[account.Owner]; !ok {
		return errors.New("auth: stored owner account is missing")
	}
	inviteCodes := make(map[string]struct{}, len(account.Accounts))
	for username, record := range account.Accounts {
		if !validUsername(username) {
			return errors.New("auth: invalid stored username")
		}
		if err := validatePasswordRecord(record.Salt, record.PasswordHash); err != nil {
			return err
		}
		if len(record.InviteCode) < 24 || len(record.InviteCode) > 128 {
			return errors.New("auth: invalid stored invite code")
		}
		if _, exists := inviteCodes[record.InviteCode]; exists {
			return errors.New("auth: duplicate stored invite code")
		}
		inviteCodes[record.InviteCode] = struct{}{}
		if record.ReferredBy != "" {
			if record.ReferredBy == username {
				return errors.New("auth: account cannot refer itself")
			}
			if _, ok := account.Accounts[record.ReferredBy]; !ok {
				return errors.New("auth: stored referrer account is missing")
			}
		}
	}
	return nil
}

func validatePasswordRecord(saltText, hashText string) error {
	salt, err := base64.RawStdEncoding.DecodeString(saltText)
	if err != nil {
		return fmt.Errorf("auth: invalid stored salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(hashText)
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

func makeAccountRecord(password string) (accountRecord, error) {
	salt, err := randomBytes(saltSize)
	if err != nil {
		return accountRecord{}, fmt.Errorf("auth: generate password salt: %w", err)
	}
	inviteCode, err := newInviteCode()
	if err != nil {
		return accountRecord{}, fmt.Errorf("auth: generate invite code: %w", err)
	}
	return accountRecord{
		Salt:         base64.RawStdEncoding.EncodeToString(salt),
		PasswordHash: base64.RawStdEncoding.EncodeToString(deriveKey(password, salt)),
		CreatedAt:    time.Now().UTC(),
		InviteCode:   inviteCode,
	}, nil
}

func newInviteCode() (string, error) {
	raw, err := randomBytes(24)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// HasAccount reports whether first-run account setup has been completed.
func (s *Store) HasAccount() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.account != nil && len(s.account.Accounts) > 0
}

// Username returns the first account name, which remains the owner identity
// for the media-client compatibility login.
func (s *Store) Username() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.account == nil {
		return ""
	}
	return s.account.Owner
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
	record, err := makeAccountRecord(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account != nil && len(s.account.Accounts) > 0 {
		return ErrAccountExists
	}
	account := &accountFile{
		Version: storeVersion,
		Owner:   username,
		Accounts: map[string]accountRecord{
			username: record,
		},
		Sessions: make(map[string]sessionRecord),
	}
	if err := writeAccount(s.path, account); err != nil {
		return err
	}
	s.account = account
	return nil
}

// Signup creates an account from a valid user's reusable invite link and
// permanently records which account referred the new user.
func (s *Store) Signup(username, password, inviteCode string) error {
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return errors.New("auth: username must be 1-64 characters without whitespace")
	}
	if !validPassword(password) {
		return errors.New("auth: password must be between 12 and 1024 characters")
	}
	if !s.HasAccount() {
		return ErrNoAccount
	}
	if !s.HasInviteCode(inviteCode) {
		return ErrInvalidInvite
	}
	record, err := makeAccountRecord(password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil || len(s.account.Accounts) == 0 {
		return ErrNoAccount
	}
	referrer, ok := findReferrer(s.account.Accounts, inviteCode)
	if !ok {
		return ErrInvalidInvite
	}
	if _, exists := s.account.Accounts[username]; exists {
		return ErrUsernameExists
	}
	record.ReferredBy = referrer
	updated := cloneAccount(s.account)
	updated.Accounts[username] = record
	if err := writeAccount(s.path, updated); err != nil {
		return err
	}
	s.account = updated
	return nil
}

func findReferrer(accounts map[string]accountRecord, inviteCode string) (string, bool) {
	if inviteCode == "" {
		return "", false
	}
	referrer := ""
	for username, record := range accounts {
		if subtle.ConstantTimeCompare([]byte(record.InviteCode), []byte(inviteCode)) == 1 {
			referrer = username
		}
	}
	return referrer, referrer != ""
}

// Authenticate verifies credentials without exposing whether the username or
// password was the incorrect part.
func (s *Store) Authenticate(username, password string) error {
	username = strings.TrimSpace(username)
	s.mu.RLock()
	if s.account == nil {
		s.mu.RUnlock()
		return ErrNoAccount
	}
	record, exists := s.account.Accounts[username]
	s.mu.RUnlock()
	if !exists {
		return ErrInvalidCredentials
	}
	salt, saltErr := base64.RawStdEncoding.DecodeString(record.Salt)
	want, hashErr := base64.RawStdEncoding.DecodeString(record.PasswordHash)
	if saltErr != nil || hashErr != nil || len(want) != keySize {
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
	if s.account == nil {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if _, ok := s.account.Accounts[username]; !ok {
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

// InviteInfo returns the user's invite code and the number of accounts
// registered through it.
func (s *Store) InviteInfo(username string) (string, int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.account == nil {
		return "", 0, false
	}
	owner, ok := s.account.Accounts[username]
	if !ok {
		return "", 0, false
	}
	count := 0
	for _, record := range s.account.Accounts {
		if record.ReferredBy == username {
			count++
		}
	}
	return owner.InviteCode, count, true
}

// HasInviteCode reports whether an invite code belongs to an existing account.
func (s *Store) HasInviteCode(inviteCode string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.account == nil {
		return false
	}
	_, ok := findReferrer(s.account.Accounts, inviteCode)
	return ok
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
	updated := cloneAccount(s.account)
	updated.Sessions = make(map[string]sessionRecord, len(s.sessions))
	for token, item := range s.sessions {
		updated.Sessions[token] = item
	}
	if err := writeAccount(s.path, updated); err != nil {
		return err
	}
	s.account = updated
	return nil
}

func cloneAccount(account *accountFile) *accountFile {
	updated := *account
	updated.Accounts = make(map[string]accountRecord, len(account.Accounts))
	for username, record := range account.Accounts {
		updated.Accounts[username] = record
	}
	updated.Sessions = make(map[string]sessionRecord, len(account.Sessions))
	for token, item := range account.Sessions {
		updated.Sessions[token] = item
	}
	return &updated
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
