package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	h := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	plain := httptest.NewRecorder()
	h.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if plain.Header().Get(k) == "" {
			t.Errorf("missing %s", k)
		}
	}
	if plain.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent over plain HTTP")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	secure := httptest.NewRecorder()
	h.ServeHTTP(secure, req)
	if secure.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS missing over HTTPS")
	}
}

// The LAN web listener proxies to the private API listener and both hops run
// this middleware; the proxy copies upstream headers with Add. Each security
// header must still appear exactly once in the browser's reply.
func TestSecurityHeadersAreNotDuplicatedThroughProxy(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate httputil.ReverseProxy copying an upstream reply that already
		// carried the headers.
		for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
			w.Header().Add(k, w.Header().Get(k))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	rec := httptest.NewRecorder()
	withSecurityHeaders(upstream).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Referrer-Policy"} {
		if got := rec.Header().Values(k); len(got) != 1 {
			t.Errorf("%s sent %d times (%q), want once", k, len(got), got)
		}
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("wrapped reply = %d %q", rec.Code, rec.Body.String())
	}
}

// Streaming handlers flush before the body completes; the wrapper must keep
// http.Flusher reachable and still emit the headers on the implicit 200.
func TestSecurityHeadersWrapperFlushes(t *testing.T) {
	rec := httptest.NewRecorder()
	withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("wrapper hides http.Flusher")
		}
		_, _ = w.Write([]byte("chunk"))
		f.Flush()
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !rec.Flushed {
		t.Error("flush did not reach the underlying writer")
	}
	if rec.Header().Get("X-Frame-Options") == "" {
		t.Error("headers missing after implicit WriteHeader")
	}
}
