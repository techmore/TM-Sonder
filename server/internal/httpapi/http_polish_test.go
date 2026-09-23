package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
)

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"gzip, deflate, br", true},
		{"deflate", false},
		{"gzip;q=0", false},
		{"gzip;q=0.0", false},
		{"gzip;q=0.5", true},
		{"br;q=1, gzip;q=0", false},
		{"br, gzip", true},
		{"GZIP", true},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest("GET", "/", nil)
		if tc.header != "" {
			req.Header.Set("Accept-Encoding", tc.header)
		}
		if got := acceptsGzip(req); got != tc.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestEtagMatches(t *testing.T) {
	const tag = `"sonder-library-7"`
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{tag, true},
		{"W/" + tag, true},
		{"*", true},
		{`"other", ` + tag, true},
		{`"other"`, false},
		{`  ` + tag + `  `, true},
	}
	for _, tc := range cases {
		if got := etagMatches(tc.header, tag); got != tc.want {
			t.Errorf("etagMatches(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestGzipQZeroDisablesCompression(t *testing.T) {
	f := newFixture(t, nil)
	get := func(ae string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", f.ts.URL+"/api/health", nil)
		req.Header.Set("Accept-Encoding", ae)
		resp, err := f.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	if ce := get("gzip").Header.Get("Content-Encoding"); ce != "gzip" {
		t.Errorf("gzip accepted: Content-Encoding = %q, want gzip", ce)
	}
	if ce := get("gzip;q=0").Header.Get("Content-Encoding"); ce == "gzip" {
		t.Error("gzip;q=0 must not be compressed")
	}
}

func TestLibraryETagVariantsAndVary(t *testing.T) {
	f := newFixture(t, nil)
	get := func(inm string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", f.ts.URL+"/api/library", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		resp, err := f.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	first := get("")
	tag := first.Header.Get("ETag")
	if tag == "" {
		t.Fatal("missing ETag")
	}
	if vary := first.Header.Values("Vary"); len(vary) == 0 {
		t.Error("missing Vary: Accept-Encoding")
	}

	for _, inm := range []string{tag, "W/" + tag, "*", `"other", ` + tag} {
		if got := get(inm).StatusCode; got != http.StatusNotModified {
			t.Errorf("If-None-Match %q -> %d, want 304", inm, got)
		}
	}
	if got := get(`"other"`).StatusCode; got != http.StatusOK {
		t.Errorf("non-matching ETag -> %d, want 200", got)
	}
}

func TestPanicRecoveryReturns500(t *testing.T) {
	f := newFixture(t, nil)
	f.s.mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})
	resp, err := f.ts.Client().Get(f.ts.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestEmbeddedPagesServeETag304(t *testing.T) {
	f := newFixture(t, nil)
	get := func(path, inm string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", f.ts.URL+path, nil)
		if inm != "" {
			req.Header.Set("If-None-Match", inm)
		}
		resp, err := f.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	for _, path := range []string{"/", "/library.css", "/library.js", "/audiobooks", "/ebooks", "/shared.js"} {
		first := get(path, "")
		if first.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, first.StatusCode)
			continue
		}
		tag := first.Header.Get("ETag")
		if tag == "" {
			t.Errorf("GET %s returned no ETag", path)
			continue
		}
		if got := get(path, tag).StatusCode; got != http.StatusNotModified {
			t.Errorf("GET %s with If-None-Match -> %d, want 304", path, got)
		}
	}
}

func TestMediaCatalogFiltersKindAndSearchesAuthor(t *testing.T) {
	f := newFixture(t, nil)
	author := "Octavia Butler"
	f.store.Upsert(
		&library.Item{MediaItem: api.MediaItem{ID: "a1", Title: "Dawn", Kind: api.KindAudiobook, Author: &author}},
		&library.Item{MediaItem: api.MediaItem{ID: "a2", Title: "Other Book", Kind: api.KindAudiobook}},
		&library.Item{MediaItem: api.MediaItem{ID: "e1", Title: "Dawn Notes", Kind: api.KindEbook}},
	)

	get := func(path string) map[string]any {
		t.Helper()
		resp, err := f.ts.Client().Get(f.ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if got := int(get("/api/audiobooks")["count"].(float64)); got != 2 {
		t.Errorf("audiobook count = %d, want 2", got)
	}
	if got := int(get("/api/ebooks")["count"].(float64)); got != 1 {
		t.Errorf("ebook count = %d, want 1", got)
	}
	found := get("/api/audiobooks?q=butler")
	if got := int(found["count"].(float64)); got != 1 {
		t.Fatalf("author search count = %d, want 1", got)
	}
	items := found["items"].([]any)
	if id := items[0].(map[string]any)["id"]; id != "a1" {
		t.Errorf("author search returned %v, want a1", id)
	}
}
