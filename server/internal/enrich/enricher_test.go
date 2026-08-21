package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"movie", Input{Title: "Arrival", Kind: "movie", Year: 2016}, "Arrival 2016 film Wikipedia"},
		{"episode", Input{Title: "Meeting", Kind: "tvShow", ShowTitle: "Lost", Season: 4, Episode: 8},
			"Lost S04E08 Meeting episode Wikipedia"},
		{"series", Input{Title: "x", Kind: "tvShow", ShowTitle: "Lost"}, "Lost television series Wikipedia"},
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
	var wikiHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			wikiHits++
		}
		w.Write([]byte(`{"query":{"pages":{"12345":{"extract":"An extract.","description":"film","categories":[{"title":"Category:Science fiction"}],"thumbnail":{"source":"http://` + r.Host + `/thumb.jpg"}}}}}`))
	}))
	defer ts.Close()
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fakejpeg"))
	}))
	defer ts2.Close()

	savedWiki, savedCovers := wikiBaseURL, coversBaseURL
	wikiBaseURL = ts.URL
	coversBaseURL = ts2.URL // image downloads land on a stub too
	defer func() { wikiBaseURL, coversBaseURL = savedWiki, savedCovers }()

	e := New(t.TempDir())
	in := Input{Title: "Some Film", Kind: "movie", Year: 2020}
	got, err := e.Enrich(context.Background(), in)
	if err != nil || !wikiCalled(t, &wikiHits) {
		t.Fatalf("enrich err=%v hits=%d", err, wikiHits)
	}
	if got.Summary != "An extract." || len(got.Tags) != 1 || got.Tags[0] != "science fiction" {
		t.Errorf("wiki payload wrong: %+v", got)
	}

	got2, _ := e.Enrich(context.Background(), in)
	if got2.Summary != got.Summary || wikiHits != 1 {
		t.Errorf("second call hit network (hits=%d)", wikiHits)
	}
}

func wikiCalled(t *testing.T, hits *int) bool {
	t.Helper()
	return *hits > 0
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
		len(got.Tags) != 3 || got.Publisher != "Audible Studios" {
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
	if got.Publisher != "Frank Herbert" || len(got.Tags) < 3 {
		t.Errorf("openlibrary payload wrong: %+v", got)
	}
	if !foundCover {
		t.Error("unexpected provider call count")
	}
}
