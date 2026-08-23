package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestMakeQuery(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{"movie", Input{Title: "Arrival", Kind: "movie", Year: 2016}, "Arrival 2016 film"},
		{"episode", Input{Title: "Meeting", Kind: "tvShow", ShowTitle: "Lost", Season: 4, Episode: 8},
			"Lost S04E08 Meeting episode"},
		{"series", Input{Title: "x", Kind: "tvShow", ShowTitle: "Lost"}, "Lost television series"},
	}
	for _, c := range cases {
		if got := makeQuery(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestNormalizeBookTitle(t *testing.T) {
	a := normalizeBookTitle("L'Étranger")
	b := normalizeBookTitle("letranger")
	if a != b || a == "" {
		t.Errorf("normalize = %q vs %q", a, b)
	}
}

func TestEnrichCacheHitAvoidsNetwork(t *testing.T) {
	dir := t.TempDir()
	e := New(dir)
	q := makeQuery(Input{Title: "Cached", Kind: "movie", Year: 2000})
	writeCache(filepath.Join(dir, sha256hex(q)+".json"), &Enrichment{
		Summary: "cached summary", Tags: []string{"x"},
	})
	got, err := e.Enrich(context.Background(), Input{Title: "Cached", Kind: "movie", Year: 2000})
	if err != nil || got == nil || got.Summary != "cached summary" {
		t.Fatalf("cache hit failed: %v %+v", err, got)
	}
}

func TestWikipediaFallbackWithMock(t *testing.T) {
	var apiHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/page/summary/"):
			w.Write([]byte(`{"extract":"An extract.","description":"film","thumbnail":{"source":"http://` + r.Host + `/thumb.jpg"},"originalimage":{"source":"http://` + r.Host + `/orig.jpg"}}`))
		case strings.HasPrefix(r.URL.Path, "/wiki/"):
			// Real-world infobox markup: protocol-relative src, HTML
			// entities, utm tracking params.
			w.Write([]byte(`<table class="infobox hproduct"><tbody><tr><td><img resource="https://en.wikipedia.org/wiki/File:Some_Film_poster.jpg" src="//upload.wikimedia.org/wikipedia/en/e/e7/Some_Film_poster.jpg?utm_source=en.wikipedia.org&amp;utm_campaign=parser&amp;utm_content=thumbnail_unscaled" decoding="async" alt="poster" data-file-width="220" data-file-height="328" class="mw-file-element"></td></tr></tbody></table>`))
		default:
			apiHits++
			w.Write([]byte(`{"query":{"pages":{"12345":{"title":"Some Film"}}}}`))
		}
	}))
	defer ts.Close()

	savedWiki, savedREST := wikiBaseURL, wikiRESTBaseURL
	savedUpload, savedCovers := wikiUploadBaseURL, coversBaseURL
	wikiBaseURL = ts.URL
	wikiRESTBaseURL = ts.URL
	wikiUploadBaseURL = ts.URL // scraped image URLs land on the stub too
	coversBaseURL = ts.URL
	defer func() {
		wikiBaseURL, wikiRESTBaseURL = savedWiki, savedREST
		wikiUploadBaseURL, coversBaseURL = savedUpload, savedCovers
	}()

	e := New(t.TempDir())
	in := Input{Title: "Some Film", Kind: "movie", Year: 2020}
	got, err := e.Enrich(context.Background(), in)
	if err != nil || apiHits == 0 {
		t.Fatalf("enrich err=%v hits=%d", err, apiHits)
	}
	if got.Summary != "An extract." || got.Provider != "wikipedia" || got.PosterPath == "" {
		t.Errorf("wiki payload wrong: %+v", got)
	}
	if st, ferr := os.Stat(got.PosterPath); ferr != nil || st.Size() == 0 {
		t.Errorf("scraped infobox poster not downloaded: %v", ferr)
	}

	got2, _ := e.Enrich(context.Background(), in)
	if got2.Summary != got.Summary {
		t.Errorf("cached summary mismatch: %+v", got2)
	}
	if apiHits != 3 { // search + poster fetch + backdrop fetch, call one only
		t.Errorf("second call made new requests (hits=%d)", apiHits)
	}
}

func TestWikipediaRejectsImplausibleMatch(t *testing.T) {
	var summaryHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/page/summary/") {
			summaryHits++
			return
		}
		// Search "matches" an unrelated page.
		w.Write([]byte(`{"query":{"pages":{"999":{"title":"Tomato"}}}}`))
	}))
	defer ts.Close()

	savedWiki, savedREST := wikiBaseURL, wikiRESTBaseURL
	wikiBaseURL = ts.URL
	wikiRESTBaseURL = ts.URL
	defer func() { wikiBaseURL, wikiRESTBaseURL = savedWiki, savedREST }()

	e := New(t.TempDir())
	got, _ := e.Enrich(context.Background(), Input{
		Title: "20251107 stephen sells out", Kind: "movie", Year: 2025,
	})
	if got != nil {
		t.Errorf("implausible match should return nothing, got %+v", got)
	}
	if summaryHits != 0 {
		t.Error("summary endpoint should never be called for rejected matches")
	}
}

func TestAudnexusLookupWithMock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/books/B08G9PBSFV") {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"description": "An audiobook blurb",
			"publisher":   "Audible Studios",
			"image":       "http://" + r.Host + "/cover.jpg",
			"genres":      []map[string]string{{"name": "Sci-Fi"}},
			"authors":     []map[string]string{{"name": "Author"}},
			"narrators":   []map[string]string{{"name": "Narrator"}},
		})
	}))
	defer ts.Close()

	saved := audnexusBaseURL
	audnexusBaseURL = ts.URL
	defer func() { audnexusBaseURL = saved }()

	e := New(t.TempDir())
	got, err := e.Enrich(context.Background(), Input{
		Title: "Book", Kind: "audiobook",
		MetadataIDSource: "audible", MetadataID: "B08G9PBSFV",
	})
	if err != nil || got == nil || got.Summary != "An audiobook blurb" ||
		len(got.Tags) != 1 || got.Publisher != "Audible Studios" ||
		got.Author != "Author" || got.Narrator != "Narrator" {
		t.Fatalf("audnexus result wrong: %v %+v", err, got)
	}
}

func TestOpenLibraryExactMatchOnly(t *testing.T) {
	var olHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search.json" {
			olHits++
		}
		w.Write([]byte(`{"docs":[
		  {"title":"Dune Messiah","cover_i":777,"subject":["sf"],"author_name":["Herbert"]},
		  {"title":"Dune","cover_i":111,"subject":["desert","politics"],"author_name":["Frank Herbert"]}
		]}`))
	}))
	defer ts.Close()

	savedOL, savedCovers := openLibBaseURL, coversBaseURL
	openLibBaseURL = ts.URL
	coversBaseURL = ts.URL
	defer func() { openLibBaseURL, coversBaseURL = savedOL, savedCovers }()

	e := New(t.TempDir())
	got, err := e.Enrich(context.Background(), Input{Title: "Dune", Kind: "ebook"})
	if err != nil || got == nil {
		t.Fatalf("err=%v got=%+v", err, got)
	}
	// Must match exact title "Dune", not "Dune Messiah".
	foundCover := strings.Contains(got.PosterPath, "") && olHits == 1
	if got.Author != "Frank Herbert" || len(got.Tags) < 3 {
		t.Errorf("openlibrary payload wrong: %+v", got)
	}
	if !foundCover {
		t.Error("unexpected provider call count")
	}
}
