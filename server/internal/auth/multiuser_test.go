package auth

import (
	"errors"
	"path/filepath"
	"testing"
)

func newOwnerStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "account.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Setup("owner", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestSignupRequiresValidInviteAndRecordsReferrer(t *testing.T) {
	s, path := newOwnerStore(t)
	code, count, ok := s.InviteInfo("owner")
	if !ok || code == "" || count != 0 {
		t.Fatalf("InviteInfo = %q,%d,%v", code, count, ok)
	}
	if !s.HasInviteCode(code) || s.HasInviteCode("") || s.HasInviteCode("nope") {
		t.Fatal("HasInviteCode wrong")
	}
	if err := s.Signup("guest", "another-long-password", "nope"); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("bad invite error = %v", err)
	}
	if err := s.Signup("guest", "short", code); err == nil {
		t.Fatal("weak password accepted")
	}
	if err := s.Signup("guest", "another-long-password", code); err != nil {
		t.Fatal(err)
	}
	if err := s.Signup("guest", "another-long-password", code); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("duplicate signup error = %v", err)
	}
	if _, count, _ := s.InviteInfo("owner"); count != 1 {
		t.Fatalf("invite count = %d", count)
	}
	if err := s.Authenticate("guest", "another-long-password"); err != nil {
		t.Fatal(err)
	}
	// Accounts persist across a reopen.
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Authenticate("guest", "another-long-password"); err != nil {
		t.Fatalf("guest lost after reopen: %v", err)
	}
}

func TestConnectedProfilesAndSharingAreScoped(t *testing.T) {
	s, _ := newOwnerStore(t)
	code, _, _ := s.InviteInfo("owner")
	for _, u := range []string{"a", "b"} {
		if err := s.Signup(u, "another-long-password", code); err != nil {
			t.Fatal(err)
		}
	}
	// Siblings invited by the same owner are not connected to each other.
	for _, p := range s.ConnectedProfiles("a") {
		if p.Username == "b" {
			t.Fatal("sibling exposed in connected profiles")
		}
	}
	if got := s.ConnectedProfiles("owner"); len(got) != 2 {
		t.Fatalf("owner sees %d profiles, want 2", len(got))
	}
	no := false
	p, ok, err := s.UpdateSharing("a", &no, &no)
	if err != nil || !ok || p.ShareProgress || p.ShareBookmarks {
		t.Fatalf("UpdateSharing = %+v %v %v", p, ok, err)
	}
	if _, ok, _ := s.UpdateSharing("ghost", &no, nil); ok {
		t.Fatal("UpdateSharing succeeded for unknown user")
	}
	if _, ok := s.Profile("ghost"); ok {
		t.Fatal("Profile for unknown user")
	}
}

func TestPublicQueueTokenLifecycle(t *testing.T) {
	s, _ := newOwnerStore(t)
	token, ok, err := s.SetPublicQueue("owner", true)
	if err != nil || !ok || token == "" {
		t.Fatalf("enable = %q %v %v", token, ok, err)
	}
	if owner, ok := s.PublicQueueOwner(token); !ok || owner != "owner" {
		t.Fatalf("owner lookup = %q %v", owner, ok)
	}
	if _, ok := s.PublicQueueOwner(""); ok {
		t.Fatal("empty token resolved")
	}
	if _, ok := s.PublicQueueOwner(token + "x"); ok {
		t.Fatal("wrong token resolved")
	}
	if _, _, err := s.SetPublicQueue("owner", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.PublicQueueOwner(token); ok {
		t.Fatal("token still valid after disabling the public queue")
	}
}

func TestGoogleLinkingIsExclusive(t *testing.T) {
	s, _ := newOwnerStore(t)
	code, _, _ := s.InviteInfo("owner")
	if err := s.Signup("guest", "another-long-password", code); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkGoogle("owner", ""); err == nil {
		t.Fatal("empty subject accepted")
	}
	if err := s.LinkGoogle("owner", "sub-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkGoogle("guest", "sub-1"); err == nil {
		t.Fatal("one Google identity linked to two users")
	}
	if err := s.LinkGoogle("owner", "sub-2"); err == nil {
		t.Fatal("linked identity silently replaced")
	}
	if u, ok := s.GoogleUsername("sub-1"); !ok || u != "owner" {
		t.Fatalf("GoogleUsername = %q %v", u, ok)
	}
	if !s.GoogleLinked("owner") || s.GoogleLinked("guest") {
		t.Fatal("GoogleLinked wrong")
	}
	if _, ok := s.GoogleUsername(""); ok {
		t.Fatal("empty subject resolved")
	}
}

func TestRevokeEndsSession(t *testing.T) {
	s, _ := newOwnerStore(t)
	token, _, err := s.CreateSession("owner")
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := s.ValidSession(token); !ok || u != "owner" {
		t.Fatal("fresh session invalid")
	}
	s.Revoke(token)
	if _, ok := s.ValidSession(token); ok {
		t.Fatal("revoked session still valid")
	}
	if _, _, err := s.CreateSession("ghost"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("session for unknown user error = %v", err)
	}
}
