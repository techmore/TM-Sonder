package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/config"
)

func TestLoginThrottleLocksUserAndExpires(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	th := &loginThrottle{now: func() time.Time { return now }}
	for i := 0; i < loginMaxPerUser-1; i++ {
		th.fail("10.0.0.1", "Owner")
		if th.check("10.0.0.1", "owner") != 0 {
			t.Fatalf("locked after only %d failures", i+1)
		}
	}
	th.fail("10.0.0.2", " owner ") // different peer, same account, case/space-insensitive
	if th.check("10.9.9.9", "OWNER") <= 0 {
		t.Fatal("account not locked after reaching the per-user limit")
	}
	if th.check("10.9.9.9", "someone-else") != 0 {
		t.Fatal("lockout leaked to an unrelated account")
	}
	now = now.Add(loginLockout + time.Second)
	if th.check("10.9.9.9", "owner") != 0 {
		t.Fatal("lockout did not expire")
	}
}

func TestLoginThrottleSuccessClearsUserOnly(t *testing.T) {
	th := &loginThrottle{}
	for i := 0; i < loginMaxPerUser-1; i++ {
		th.fail("10.0.0.1", "owner")
	}
	th.succeed("owner")
	th.fail("10.0.0.1", "owner")
	if th.check("10.0.0.1", "owner") != 0 {
		t.Fatal("success did not reset the per-user counter")
	}
}

func TestLoginThrottleBoundsMemory(t *testing.T) {
	th := &loginThrottle{}
	for i := 0; i < loginMaxTracked*2; i++ {
		th.fail("10.0.0.1", "user-"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+time.Duration(i).String())
	}
	if len(th.buckets) > loginMaxTracked {
		t.Fatalf("tracked %d buckets, cap is %d", len(th.buckets), loginMaxTracked)
	}
}

func TestLoginEndpointsReturn429AfterRepeatedFailures(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) {
		cfg.AllowLAN = true
		cfg.PairingToken = "pair-me"
	})
	if err := f.s.accounts.Setup("owner", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	post := func(path, body, password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Replace(body, "PW", password, 1)))
		req.RemoteAddr = "192.168.3.50:50123"
		req.Host = "sonder.example:8096"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		return rec
	}
	const browser = `{"username":"owner","password":"PW"}`
	for i := 0; i < loginMaxPerUser; i++ {
		if rec := post("/api/auth/login", browser, "wrong-password-123"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d", i+1, rec.Code)
		}
	}
	rec := post("/api/auth/login", browser, "a-long-test-password")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("correct password while locked: status=%d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	// The same account is locked on the media-server login paths too.
	if rec := post("/Users/AuthenticateByName", `{"Username":"owner","Pw":"PW"}`, "a-long-test-password"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("jellyfin login while locked = %d", rec.Code)
	}
}

func TestClientAddrOnlyTrustsForwardedHeaderFromInternalPeers(t *testing.T) {
	mk := func(remote, xff string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	cases := []struct{ remote, xff, want string }{
		{"127.0.0.1:1", "203.0.113.9", "203.0.113.9"},
		{"10.96.131.1:1", "1.1.1.1, 203.0.113.9", "203.0.113.9"}, // spoofed prefix ignored
		{"127.0.0.1:1", "not-an-ip", "127.0.0.1"},
		{"127.0.0.1:1", "", "127.0.0.1"},
		{"198.51.100.4:1", "203.0.113.9", "198.51.100.4"}, // public peer cannot choose its identity
	}
	for _, c := range cases {
		if got := clientAddr(mk(c.remote, c.xff)); got != c.want {
			t.Errorf("clientAddr(%s, %q) = %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}
