package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCuratedArtworkIsReversibleAndCannotUseTraversal(t *testing.T) {
	f := newFixture(t, nil)
	original := f.addItem(t, "film", "Film")
	dir := filepath.Join(f.s.cfg().DataDir, "curated-posters")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := strings.Repeat("a", 64) + ".jpg"
	if err := os.WriteFile(filepath.Join(dir, file), []byte("poster"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]curatedPoster{"film": {Filename: file, License: "Fair use"}, "bad": {Filename: "../secret.jpg"}}
	data, _ := json.Marshal(manifest)
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	item := f.s.wireItems()[0]
	if item.PosterURL == nil || !strings.HasPrefix(*item.PosterURL, "/artwork/curated/film") {
		t.Fatal("override missing")
	}
	if _, ok := f.s.curatedPosters()["bad"]; ok {
		t.Fatal("unsafe filename accepted")
	}
	current, _ := f.store.Get("film")
	if current.PosterPath != original.PosterPath {
		t.Fatal("source artwork mutated")
	}
	response, body := get(t, f.ts.URL+"/artwork/curated/film")
	if response.StatusCode != 200 || body != "poster" {
		t.Fatal("override not served")
	}
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if f.s.wireItems()[0].PosterURL != original.PosterURL {
		t.Fatal("removing override did not restore source")
	}
}
