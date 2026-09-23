package enrich

import (
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
)

func TestApplyResultMergesAndPreservesLocalArt(t *testing.T) {
	cur := &library.Item{
		MediaItem:    api.MediaItem{ID: "x", Title: "X", Tags: []string{"Existing"}},
		PosterPath:   "/media/poster.jpg",
		PosterSource: "local",
	}
	result := &Enrichment{
		Summary:    "A summary.",
		Author:     "An Author",
		Narrator:   "A Narrator",
		Tags:       []string{"existing", "New"},
		Genres:     []string{"Drama", "drama", "Sci-Fi"},
		PosterPath: "/cache/x.jpg",
		Provider:   "tmdb",
	}
	if !applyResult(cur, result) {
		t.Fatal("expected applyResult to report a change")
	}
	if cur.Summary != "A summary." {
		t.Errorf("summary = %q", cur.Summary)
	}
	if cur.Author == nil || *cur.Author != "An Author" {
		t.Errorf("author = %v", cur.Author)
	}
	if cur.Narrator == nil || *cur.Narrator != "A Narrator" {
		t.Errorf("narrator = %v", cur.Narrator)
	}
	// Case-insensitive dedupe: "existing" must not be added twice.
	if len(cur.Tags) != 2 {
		t.Errorf("tags = %v, want 2 entries", cur.Tags)
	}
	// Genres are kept separate from free-form tags, deduped case-insensitively.
	if len(cur.Genres) != 2 {
		t.Errorf("genres = %v, want 2 entries", cur.Genres)
	}
	if len(cur.Tags) == len(cur.Genres) && cur.Tags[0] == cur.Genres[0] {
		t.Error("genres should be a distinct field from tags")
	}
	// Locally discovered artwork is never replaced.
	if cur.PosterPath != "/media/poster.jpg" || cur.PosterSource != "local" {
		t.Errorf("local art replaced: path=%q source=%q", cur.PosterPath, cur.PosterSource)
	}

	// A repeat pass over the same result changes nothing.
	if applyResult(cur, result) {
		t.Error("expected no change on a repeated result")
	}
}

func TestApplyResultUpgradesGeneratedThumbnail(t *testing.T) {
	cur := &library.Item{
		MediaItem:    api.MediaItem{ID: "y", Title: "Y"},
		PosterPath:   "/data/artwork/y.jpg",
		PosterSource: "thumbnail",
	}
	result := &Enrichment{PosterPath: "/cache/y.jpg", Provider: "wikipedia"}
	if !applyResult(cur, result) {
		t.Fatal("expected an official poster to replace a generated thumbnail")
	}
	if cur.PosterSource != "wikipedia" {
		t.Errorf("poster source = %q, want wikipedia", cur.PosterSource)
	}
	if cur.PosterURL == nil || *cur.PosterURL != "/artwork/poster/y" {
		t.Errorf("poster URL not set: %v", cur.PosterURL)
	}
}

func TestApplyResultIgnoresEmpty(t *testing.T) {
	cur := &library.Item{MediaItem: api.MediaItem{ID: "z", Title: "Z"}}
	if applyResult(cur, &Enrichment{}) {
		t.Error("an empty result must not count as a change")
	}
	if cur.Summary != "" || cur.Author != nil || cur.PosterURL != nil {
		t.Errorf("empty result mutated the item: %+v", cur.MediaItem)
	}
}
