package library

import (
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func TestTVArtStampsShowAndSeasonPosters(t *testing.T) {
	store := New()
	lib := "tv"
	season1 := 1
	season2 := 2
	ep1 := 1
	ep2 := 2
	show := "Show (2020)"
	mk := func(id, rel, poster string, season *int, ep *int) {
		u := "/artwork/poster/" + id
		store.Upsert(&Item{
			MediaItem: api.MediaItem{ID: id, Kind: api.KindTVShow, LibraryID: &lib, ShowTitle: &show,
				Title: "Ep " + id, PosterURL: &u, SeasonNumber: season, EpisodeNumber: ep},
			FilePath:           "/TV/" + rel,
			SourceRelativePath: rel,
			PosterPath:         poster,
			PosterSource:       "local",
		})
	}
	// Show-folder art, season-folder art, and an extras folder whose art must
	// never represent the show or the regular season.
	mk("show-ep", "Show (2020)/Season 01/s01e01.mkv", "/TV/Show (2020)/poster.jpg", &season1, &ep1)
	mk("s2-ep", "Show (2020)/Season 02/s02e01.mkv", "/TV/Show (2020)/Season 02/poster.jpg", &season2, &ep1)
	mk("extra-ep", "Show (2020)/Extras/Season 01 Extras/x.mkv", "/TV/Show (2020)/Extras/Season 01 Extras/poster.jpg", &season1, &ep2)

	groups := store.GroupedItems([]config.Library{{ID: lib, Path: "/TV", Kind: "tvShow"}})
	byID := map[string]api.MediaItem{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	if len(byID) != 3 {
		t.Fatalf("items dropped: %d", len(byID))
	}
	showURL := "/artwork/poster/show-ep"
	for _, id := range []string{"show-ep", "s2-ep", "extra-ep"} {
		got := byID[id].ShowPosterURL
		if got == nil || *got != showURL {
			t.Fatalf("%s show poster = %v, want %s", id, got, showURL)
		}
	}
	s1 := "/artwork/poster/show-ep"
	if got := byID["show-ep"].SeasonPosterURL; got == nil || *got != s1 {
		t.Fatalf("s1 season poster = %v, want %s", got, s1)
	}
	s2 := "/artwork/poster/s2-ep"
	if got := byID["s2-ep"].SeasonPosterURL; got == nil || *got != s2 {
		t.Fatalf("s2 season poster = %v, want %s", got, s2)
	}
	// The extras episode shares season 1's number but its art must not leak
	// into the regular season or the show.
	if got := byID["extra-ep"].SeasonPosterURL; got == nil || *got != s1 {
		t.Fatalf("extras season poster = %v, want %s", got, s1)
	}
}

func TestTVArtFallsBackToSeasonWhenShowHasNoArt(t *testing.T) {
	store := New()
	lib := "tv"
	season1 := 1
	ep1 := 1
	show := "Show"
	u := "/artwork/poster/only"
	store.Upsert(&Item{
		MediaItem: api.MediaItem{ID: "only", Kind: api.KindTVShow, LibraryID: &lib, ShowTitle: &show,
			Title: "Ep", PosterURL: &u, SeasonNumber: &season1, EpisodeNumber: &ep1},
		FilePath:           "/TV/Show/Season 01/e.mkv",
		SourceRelativePath: "Show/Season 01/e.mkv",
		PosterPath:         "/TV/Show/Season 01/poster.jpg",
		PosterSource:       "local",
	})
	groups := store.GroupedItems([]config.Library{{ID: lib, Path: "/TV", Kind: "tvShow"}})
	if len(groups) != 1 {
		t.Fatalf("items dropped: %d", len(groups))
	}
	if groups[0].ShowPosterURL == nil || *groups[0].ShowPosterURL != u {
		t.Fatalf("show fallback = %v, want %s", groups[0].ShowPosterURL, u)
	}
	if groups[0].SeasonPosterURL == nil || *groups[0].SeasonPosterURL != u {
		t.Fatalf("season fallback = %v, want %s", groups[0].SeasonPosterURL, u)
	}
}
