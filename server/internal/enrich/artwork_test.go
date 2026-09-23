package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestArticleImageUsesArticlePathAndStaysInsideInfobox(t *testing.T) {
	path := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write([]byte(`<table class="infobox"><tr><td>No poster</td></tr></table><img src="https://upload.wikimedia.org/wikipedia/en/a/actor.jpg">`))
	}))
	defer server.Close()
	old := wikiBaseURL
	wikiBaseURL = server.URL + "/w/api.php"
	defer func() { wikiBaseURL = old }()
	got, genres := New(t.TempDir()).wikiInfobox(context.Background(), "Some Film")
	if path != "/wiki/Some_Film" {
		t.Fatalf("wrong article route: %s", path)
	}
	if got != "" {
		t.Fatalf("unrelated article image selected: %s", got)
	}
	if len(genres) != 0 {
		t.Fatalf("genres extracted from an infobox without one: %v", genres)
	}
}

func TestInfoboxGenresAndPosterFromOneFetch(t *testing.T) {
	page := `<html><body><table class="infobox">
	  <tr><th>Genre</th><td><a href="/wiki/Science_fiction_film">Science fiction</a>,
	    <a href="/wiki/Drama_(film_and_television)">Drama</a><sup>[1]</sup></td></tr>
	  <tr><th>Directed by</th><td><a href="/wiki/Denis_Villeneuve">Denis Villeneuve</a></td></tr>
	  <tr><td><img src="https://upload.wikimedia.org/wikipedia/en/a/poster.jpg"></td></tr>
	</table></body></html>`
	fetches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		w.Write([]byte(page))
	}))
	defer server.Close()
	old := wikiBaseURL
	wikiBaseURL = server.URL + "/w/api.php"
	defer func() { wikiBaseURL = old }()

	poster, genres := New(t.TempDir()).wikiInfobox(context.Background(), "Dune")
	if poster == "" {
		t.Fatal("poster not extracted")
	}
	if fetches != 1 {
		t.Errorf("page fetched %d times, want 1", fetches)
	}
	if want := []string{"Science fiction", "Drama"}; !reflect.DeepEqual(genres, want) {
		t.Errorf("genres = %v, want %v", genres, want)
	}
}

func TestParseGenreCellPlainAndLinked(t *testing.T) {
	got := parseGenreCell(`<td>Drama, Thriller &amp; Mystery[2], <a href="/wiki/Comedy">Comedy</a></td>`)
	want := []string{"Comedy", "Drama", "Thriller & Mystery"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseGenreCell = %q, want %q", got, want)
	}
	if genres := parseGenreCell(`<td><a href="/x">${"y"}</a></td>`); len(genres) != 1 {
		t.Errorf("single linked genre = %v", genres)
	}
}

func TestHTTPSWikimediaImageAccepted(t *testing.T) {
	if normalizeWikiImageURL("https://upload.wikimedia.org/wikipedia/en/a/poster.jpg") == "" {
		t.Fatal("valid HTTPS poster rejected")
	}
	if normalizeWikiImageURL("https://upload.wikimedia.org.evil.example/poster.jpg") != "" {
		t.Fatal("untrusted host accepted")
	}
}
