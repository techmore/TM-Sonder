package library

import (
	"os"
	"path/filepath"
	"testing"

	"tm-sonder/server/internal/api"
)

func TestRebuildKeepsProviderPosterOverOrphanFrame(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Movie.mkv")
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	store := New()
	scanner := NewScanner(store)
	scanner.SetThumbnailDir(root)
	if err := os.WriteFile(filepath.Join(root, "item.jpg"), []byte("frame"), 0600); err != nil {
		t.Fatal(err)
	}
	old := scanner.buildItem(path, "item", st, api.MediaFormat("mkv"), "movies", "movie")
	old.PosterPath = "/provider/official.jpg"
	old.PosterSource = "wikipedia"
	store.Upsert(old)
	rebuilt := scanner.buildItem(path, "item", st, api.MediaFormat("mkv"), "movies", "movie")
	if rebuilt.PosterPath != old.PosterPath || rebuilt.PosterSource != old.PosterSource {
		t.Fatalf("official artwork replaced by frame: %s (%s)", rebuilt.PosterPath, rebuilt.PosterSource)
	}
}

func TestStructuredSeasonUsesConsistentShowName(t *testing.T) {
	a := ParseFilename("/TV/Lie to Me (2009)/Season 03/Lie to Me Season 3 Episode 03 - Dirty Loyal.mkv", "tvShow")
	b := ParseFilename("/TV/Lie to Me (2009)/Season 03/Lie.to.Me.S03E03.1080p.mkv", "tvShow")
	if a.ShowTitle != b.ShowTitle || a.ShowTitle == "Lie to Me Season" {
		t.Fatalf("inconsistent show identity: %q vs %q", a.ShowTitle, b.ShowTitle)
	}
}

func TestCatalogUsesStableMoviePosterEndpoint(t *testing.T) {
	store := New()
	frame := "/artwork/poster/movie"
	store.Upsert(&Item{MediaItem: api.MediaItem{ID: "movie", Kind: api.KindMovie, PosterURL: &frame}, PosterSource: "thumbnail"})
	if got := store.Items()[0].PosterURL; got == nil || *got != "/artwork/poster/movie" {
		t.Fatalf("movie poster endpoint = %v, want /artwork/poster/movie", got)
	}
	original, _ := store.Get("movie")
	if original.PosterURL == nil {
		t.Fatal("internal preview reference was removed")
	}
	original.PosterSource = "local"
	store.Upsert(original)
	if got := store.Items()[0].PosterURL; got == nil || *got != "/artwork/poster/movie" {
		t.Fatalf("local cover endpoint = %v, want /artwork/poster/movie", got)
	}
}

func TestPackedEpisodeDoesNotBecomeShow(t *testing.T) {
	for _, name := range []string{"210 - Jack versus Demongo-1.mkv", "102 - The Samurai Called Jack.mkv", "313 - Jack and the Labyrinth.mkv"} {
		got := ParseFilename("/TV/Samurai Jack (2001)/Season 02/"+name, "tvShow")
		if got.ShowTitle != "Samurai Jack (2001)" {
			t.Fatalf("%s became show %q", name, got.ShowTitle)
		}
		if got.Episode != nil {
			t.Fatalf("ambiguous packed code guessed for %s", name)
		}
	}
}

func TestLoadRepairsEpisodeTitleShow(t *testing.T) {
	store := New()
	name := "210 - Jack versus Demongo"
	store.Upsert(&Item{MediaItem: api.MediaItem{ID: "jack", Kind: api.KindTVShow, ShowTitle: &name, ProgressSeconds: 45}, FilePath: "/TV/Samurai Jack (2001)/Season 02/210 - Jack versus Demongo-1.mkv"})
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := store.Flush(path); err != nil {
		t.Fatal(err)
	}
	restored := New()
	if err := restored.Load(path); err != nil {
		t.Fatal(err)
	}
	item, _ := restored.Get("jack")
	if item.ShowTitle == nil || *item.ShowTitle != "Samurai Jack (2001)" || item.ProgressSeconds != 45 {
		t.Fatalf("repair failed: %+v", item)
	}
}
