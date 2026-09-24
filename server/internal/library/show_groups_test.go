package library

import (
	"testing"
	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func TestBrowsingGroupsUseFoldersWithoutChangingParsedIdentity(t *testing.T) {
	store := New()
	lib := "tv"
	for n, name := range []string{"Samurai Jack", "Samurai Jack (2001)", "SamuraiChamploo"} {
		id := string(rune('a' + n))
		store.Upsert(&Item{MediaItem: api.MediaItem{ID: id, Kind: api.KindTVShow, LibraryID: &lib, ShowTitle: &name}, FilePath: "/TV/Samurai Jack (2001)/Season 01/" + id + ".mkv"})
	}
	groups := store.GroupedItems([]config.Library{{ID: lib, Path: "/TV", Kind: "tvShow"}})
	if len(groups) != 3 {
		t.Fatal("files dropped")
	}
	ids := map[string]bool{}
	for _, item := range groups {
		if item.ShowGroupID == nil || item.ShowGroupTitle == nil || *item.ShowGroupTitle != "Samurai Jack (2001)" {
			t.Fatalf("missing group: %+v", item)
		}
		ids[*item.ShowGroupID] = true
		original, _ := store.Get(item.ID)
		if *original.ShowTitle != *item.ShowTitle || original.ShowGroupID != nil {
			t.Fatal("parsed identity mutated")
		}
	}
	if len(ids) != 1 {
		t.Fatal("folder split into multiple groups")
	}
}

func TestBrowsingGroupsDoNotMergeRemakesOrFlatFiles(t *testing.T) {
	store := New()
	lib := "tv"
	name := "Show"
	for _, path := range []string{"/TV/Show (1990)/a.mkv", "/TV/Show (2020)/a.mkv", "/TV/flat.mkv", "/elsewhere/a.mkv"} {
		store.Upsert(&Item{MediaItem: api.MediaItem{ID: path, Kind: api.KindTVShow, LibraryID: &lib, ShowTitle: &name}, FilePath: path})
	}
	groups := store.GroupedItems([]config.Library{{ID: lib, Path: "/TV", Kind: "tvShow"}})
	ids := map[string]bool{}
	unmapped := 0
	for _, item := range groups {
		if item.ShowGroupID == nil {
			unmapped++
		} else {
			ids[*item.ShowGroupID] = true
		}
	}
	if len(ids) != 2 || unmapped != 2 {
		t.Fatalf("unsafe grouping: %v unmapped=%d", ids, unmapped)
	}
}

func TestBrowsingGroupsUseStoredRelativePathAcrossMountAliases(t *testing.T) {
	store := New()
	lib := "plex-tvShow-plex"
	show := "The X-Files"
	store.Upsert(&Item{
		MediaItem:          api.MediaItem{ID: "x-files", Kind: api.KindTVShow, LibraryID: &lib, ShowTitle: &show},
		FilePath:           "/Volumes/14tb/plex/tv_shows/The X-Files (1993)/Season 02/episode.mkv",
		SourceRelativePath: "The X-Files (1993)/Season 02/episode.mkv",
	})

	groups := store.GroupedItems([]config.Library{{
		ID: lib, Path: "/Users/seandolbec/NAS/plex/tv_shows", Kind: "tvShow",
	}})
	if groups[0].ShowGroupID == nil || groups[0].ShowGroupTitle == nil {
		t.Fatalf("stored relative path did not produce a show group: %+v", groups[0])
	}
	if *groups[0].ShowGroupTitle != "The X-Files (1993)" {
		t.Fatalf("wrong show folder: %q", *groups[0].ShowGroupTitle)
	}
}
