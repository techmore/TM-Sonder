package library

import (
	"testing"
	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func TestHealthFlagsSplitFoldersButNotQualityCopies(t *testing.T) {
	store := New()
	lib := "tv"
	add := func(id, show, path string) {
		store.Upsert(&Item{MediaItem: api.MediaItem{ID: id, Kind: api.KindTVShow, ShowTitle: &show, LibraryID: &lib}, FilePath: path})
	}
	add("a", "Samurai Jack", "/TV/Samurai Jack (2001)/Season 01/a.mkv")
	add("b", "Samurai Jack", "/TV/Samurai Jack (2001)/Season 01/a-1080p.mkv")
	libs := []config.Library{{ID: lib, Path: "/TV", Kind: "tvShow"}}
	clean := store.Health(libs)
	if len(clean.Issues) != 0 || clean.ShowCount != 1 || clean.RepresentedFolders != 1 {
		t.Fatalf("quality copies flagged: %+v", clean)
	}
	add("c", "102 - The Samurai Called Jack", "/TV/Samurai Jack (2001)/Season 01/c.mkv")
	bad := store.Health(libs)
	if len(bad.Issues) != 2 || bad.Issues[0].Code != "split_show_folder" {
		t.Fatalf("split not detected: %+v", bad)
	}
	if store.Count() != 3 {
		t.Fatal("audit mutated catalog")
	}
}

func TestHealthReportsUnmappedWithoutFalseCountAlarm(t *testing.T) {
	store := New()
	name, lib := "Show", "tv"
	store.Upsert(&Item{MediaItem: api.MediaItem{ID: "a", Kind: api.KindTVShow, ShowTitle: &name, LibraryID: &lib}, FilePath: "/outside/a.mkv"})
	report := store.Health([]config.Library{{ID: lib, Path: "/TV"}})
	if report.UnmappedItems != 1 || len(report.Issues) != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}
