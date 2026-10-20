package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Login throttling. The public site exposes three password endpoints
// (/api/auth/login and the Jellyfin and Audiobookshelf compatibility logins),
// each backed by 210k-round PBKDF2. Without a limit they are both a
// brute-force oracle and a cheap CPU-exhaustion lever.
//
// Failures are counted per username and per peer address. Both are tracked
// because behind a reverse proxy every client shares one peer address (so the
// per-address bucket is generous), while a distributed attack on one account
// is caught by the per-username bucket.
const (
	loginFailWindow   = 15 * time.Minute
	loginLockout      = 15 * time.Minute
	loginMaxPerUser   = 10
	loginMaxPerPeer   = 30
	loginMaxTracked   = 4096
	loginThrottleBody = "Too many failed sign-in attempts. Try again later."
)

var errLoginThrottled = errors.New("httpapi: too many failed sign-in attempts")

type loginBucket struct {
	failures    int
	windowStart time.Time
	lockedUntil time.Time
}

type loginThrottle struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
	now     func() time.Time
}

func (t *loginThrottle) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func loginKeys(peer, username string) (string, string) {
	return "u:" + strings.ToLower(strings.TrimSpace(username)), "p:" + peer
}

// check reports how long the caller must wait, or zero when sign-in may proceed.
func (t *loginThrottle) check(peer, username string) time.Duration {
	now := t.clock()
	t.mu.Lock()
	defer t.mu.Unlock()
	var wait time.Duration
	userKey, peerKey := loginKeys(peer, username)
	for _, key := range []string{userKey, peerKey} {
		if b := t.buckets[key]; b != nil && now.Before(b.lockedUntil) {
			if d := b.lockedUntil.Sub(now); d > wait {
				wait = d
			}
		}
	}
	return wait
}

func (t *loginThrottle) fail(peer, username string) {
	now := t.clock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.buckets == nil {
		t.buckets = make(map[string]*loginBucket)
	}
	if len(t.buckets) >= loginMaxTracked {
		t.pruneLocked(now)
	}
	userKey, peerKey := loginKeys(peer, username)
	t.record(userKey, loginMaxPerUser, now)
	t.record(peerKey, loginMaxPerPeer, now)
}

func (t *loginThrottle) record(key string, limit int, now time.Time) {
	b := t.buckets[key]
	if b == nil {
		if len(t.buckets) >= loginMaxTracked {
			return // table full of live lockouts: fail open rather than grow
		}
		b = &loginBucket{}
		t.buckets[key] = b
	}
	if now.Sub(b.windowStart) > loginFailWindow {
		b.failures, b.windowStart = 0, now
	}
	b.failures++
	if b.failures >= limit {
		b.lockedUntil = now.Add(loginLockout)
		b.failures, b.windowStart = 0, now
	}
}

// succeed clears the per-username bucket; the peer bucket is left alone so one
// valid account cannot be used to reset a scan of other usernames.
func (t *loginThrottle) succeed(username string) {
	userKey, _ := loginKeys("", username)
	t.mu.Lock()
	delete(t.buckets, userKey)
	t.mu.Unlock()
}

func (t *loginThrottle) pruneLocked(now time.Time) {
	for key, b := range t.buckets {
		if !now.Before(b.lockedUntil) && now.Sub(b.windowStart) > loginFailWindow {
			delete(t.buckets, key)
		}
	}
}

// clientAddr is the address the throttle attributes a request to. Behind the
// reverse proxy every connection arrives from the proxy's own loopback/private
// address, so using the socket peer would let any one client lock every user
// out. When the peer is loopback or private, the last X-Forwarded-For entry
// (the one appended by the nearest proxy, never a client-supplied value) is
// used instead. Public peers are never trusted to supply it.
func clientAddr(r *http.Request) string {
	peer := peerHost(r)
	ip := net.ParseIP(peer)
	if ip == nil || !(ip.IsLoopback() || ip.IsPrivate()) {
		return peer
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if last := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(last) != nil {
		return last
	}
	return peer
}
