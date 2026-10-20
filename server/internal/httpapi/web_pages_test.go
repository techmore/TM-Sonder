package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every embedded browser page is a full document: a declared language for
// screen readers, exactly one main landmark (library.html used to carry
// three), a title, and a viewport so phones do not render the desktop width.
func TestEmbeddedPagesAreWellFormedDocuments(t *testing.T) {
	pages := []string{
		"web/library.html", "web/audiobooks.html", "web/audiobooks-beta.html",
		"web/ebooks.html", "web/ebook-reader.html", "web/youtube.html",
	}
	for _, name := range pages {
		raw := string(mustReadWeb(name))
		if !strings.Contains(raw, `<html lang="`) {
			t.Errorf("%s: <html> has no lang attribute", name)
		}
		if n := strings.Count(raw, "<main"); n != 1 {
			t.Errorf("%s: %d <main> landmarks, want exactly 1", name, n)
		}
		if !strings.Contains(raw, "<title>") {
			t.Errorf("%s: missing <title>", name)
		}
		if !strings.Contains(raw, `name="viewport"`) {
			t.Errorf("%s: missing viewport meta", name)
		}
	}
}

// The library browser must not treat an auth failure as an empty catalog: the
// script checks response.ok and offers a sign-in link or a retry control.
func TestLibraryScriptHandlesCatalogLoadFailure(t *testing.T) {
	raw := string(mustReadWeb("web/library.js"))
	for _, needle := range []string{
		`data-action="retry-library-load"`,
		`/account/login?next=`,
		`function loadLibrary()`,
	} {
		if !strings.Contains(raw, needle) {
			t.Errorf("library.js missing %q", needle)
		}
	}
	for _, name := range []string{"web/audiobooks.html", "web/audiobooks-beta.html", "web/ebooks.html"} {
		raw := string(mustReadWeb(name))
		if !strings.Contains(raw, "function renderLoadError(") || !strings.Contains(raw, "response.ok") {
			t.Errorf("%s: catalog fetch has no failure state", name)
		}
	}
}

// The YouTube page is a browser page: anonymous visitors behind the web
// listener are sent to sign in, instead of receiving the API's JSON error.
func TestYouTubePageRedirectsAnonymousBrowsers(t *testing.T) {
	f := newFixture(t, nil)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, f.ts.URL+"/youtube", nil)
	// The LAN web listener proxies to the private API with this header set,
	// which is what distinguishes a real loopback operator from a proxied one.
	req.Header.Set("X-Forwarded-Host", "sonder.example")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /youtube (proxied, anonymous) = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/account/login?next=%2Fyoutube" {
		t.Errorf("Location = %q", loc)
	}

	direct := httptest.NewRecorder()
	local := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/youtube", nil)
	local.RemoteAddr = "127.0.0.1:50000" // a real loopback operator, not the LAN proxy
	f.s.Handler().ServeHTTP(direct, local)
	if direct.Code != http.StatusOK || !strings.Contains(direct.Header().Get("Content-Type"), "text/html") {
		t.Errorf("direct loopback GET /youtube = %d %s, want 200 text/html", direct.Code, direct.Header().Get("Content-Type"))
	}
}
