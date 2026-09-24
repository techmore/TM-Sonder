package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMovieMetadataUsesWikipediaAndWikidataCast(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/w/api.php" && r.URL.Query().Get("action") == "query":
			_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"title":"Some Film"}}}}`))
		case strings.HasPrefix(r.URL.Path, "/page/summary/"):
			_, _ = w.Write([]byte(`{"title":"Some Film","extract":"A compact primer.","description":"2020 film","wikibase_item":"Q1"}`))
		case r.URL.Path == "/wiki/Some_Film":
			_, _ = w.Write([]byte(`<table class="infobox"><tr><th>Directed by</th><td><a href="/wiki/Director_Name">Director Name</a></td></tr><tr><th>Starring</th><td><ul><li><a href="/wiki/Actor_Name">Actor Name</a></li></ul></td></tr></table>`))
		case r.URL.Path == "/w/api.php" && r.URL.Query().Get("action") == "wbgetentities":
			ids := r.URL.Query().Get("ids")
			if ids == "Q1" {
				_, _ = w.Write([]byte(`{"entities":{"Q1":{"claims":{"P161":[{"mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q2"}}},"qualifiers":{"P453":[{"snaktype":"value","datavalue":{"value":{"id":"Q3"}}}]}}],"P57":[{"mainsnak":{"snaktype":"value","datavalue":{"value":{"id":"Q4"}}}}],"P444":[{"mainsnak":{"snaktype":"value","datavalue":{"value":"88%"}},"qualifiers":{"P447":[{"snaktype":"value","datavalue":{"value":{"id":"Q5"}}}],"P459":[{"snaktype":"value","datavalue":{"value":{"id":"Q6"}}}]}}]}}}}`))
			} else {
				_, _ = w.Write([]byte(`{"entities":{"Q2":{"labels":{"en":{"value":"Actor Name"}},"claims":{"P18":[{"mainsnak":{"snaktype":"value","datavalue":{"value":"Actor_Name.jpg"}}}]}},"Q3":{"labels":{"en":{"value":"The Character"}}},"Q4":{"labels":{"en":{"value":"Director Name"}}},"Q5":{"labels":{"en":{"value":"Rotten Tomatoes"}}},"Q6":{"labels":{"en":{"value":"Tomatometer score"}}}}}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	savedWiki, savedREST, savedWikidata, savedCommons := wikiBaseURL, wikiRESTBaseURL, wikidataBaseURL, wikimediaFileBaseURL
	wikiBaseURL, wikiRESTBaseURL, wikidataBaseURL, wikimediaFileBaseURL = ts.URL+"/w/api.php", ts.URL, ts.URL+"/w/api.php", ts.URL+"/file"
	defer func() {
		wikiBaseURL, wikiRESTBaseURL, wikidataBaseURL, wikimediaFileBaseURL = savedWiki, savedREST, savedWikidata, savedCommons
	}()

	e := New(t.TempDir())
	e.Pacing = time.Nanosecond
	got, err := e.MovieMetadata(context.Background(), Input{Title: "Some Film", Kind: "movie", Year: 2020})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Primer != "A compact primer." || len(got.Cast) != 1 {
		t.Fatalf("metadata = %+v", got)
	}
	if got.Cast[0].Name != "Actor Name" || got.Cast[0].Character != "The Character" ||
		got.Cast[0].ImageURL != ts.URL+"/file/Actor_Name.jpg?width=160" {
		t.Errorf("cast = %+v", got.Cast)
	}
	if len(got.Directors) != 1 || got.Directors[0] != "Director Name" {
		t.Errorf("directors = %v", got.Directors)
	}
	if len(got.Ratings) != 1 || got.Ratings[0].Source != "Rotten Tomatoes" || got.Ratings[0].Method != "Tomatometer score" {
		t.Errorf("ratings = %+v", got.Ratings)
	}

	before := hits
	cached, err := e.MovieMetadata(context.Background(), Input{Title: "Some Film", Kind: "movie", Year: 2020})
	if err != nil || cached == nil || cached.Cast[0].Character != "The Character" {
		t.Fatalf("cached metadata = %+v, err=%v", cached, err)
	}
	if hits != before {
		t.Fatalf("cache miss made %d additional provider requests", hits-before)
	}
}

func TestMovieMetadataJSONShape(t *testing.T) {
	payload, err := json.Marshal(MovieMetadata{
		PageTitle: "A Film",
		Cast:      []MovieCastMember{{Name: "Actor", Character: "Role"}},
		Ratings:   []MovieRating{{Source: "Metacritic", Value: "76/100"}},
	})
	if err != nil || !strings.Contains(string(payload), `"character":"Role"`) {
		t.Fatalf("payload = %s, err=%v", payload, err)
	}
}
