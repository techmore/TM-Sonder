package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/httpapi"
	"tm-sonder/server/internal/library"
)

// TestRunEnrichmentPassLive exercises the actual pass function against live
// Wikipedia with one well-known title. It requires network access, so it is
// skipped unless SONDER_LIVE_ENRICH_TEST=1 is set (kept out of make test).
func TestRunEnrichmentPassLive(t *testing.T) {
	if os.Getenv("SONDER_LIVE_ENRICH_TEST") != "1" {
		t.Skip("set SONDER_LIVE_ENRICH_TEST=1 to run the live-network enrichment test")
	}
	store := library.New()
	store.Upsert(&library.Item{
		MediaItem: api.MediaItem{
			ID: "jp", Title: "Jurassic Park", Kind: api.KindMovie,
			Year: 1993, Format: api.FormatMP4, Tags: []string{},
		},
		FilePath: filepath.Join(t.TempDir(), "jp.mp4"),
	})

	logger := log.New(log.Writer(), "", 0)
	updated := httpapi.RunEnrichmentPass(logger, store, filepath.Join(t.TempDir(), "cache"))
	if updated == 0 {
		t.Fatal("enrichment pass updated nothing for a well-known title")
	}
	it, _ := store.Get("jp")
	s := it.Summary
	if len(s) > 50 {
		s = s[:50]
	}
	t.Logf("summary=%q source=%q poster=%v backdrop=%v",
		s, it.PosterSource, it.PosterPath != "", it.BackdropPath != "")
}
