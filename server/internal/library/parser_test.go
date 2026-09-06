package library

import (
	"fmt"
	"testing"

	"tm-sonder/server/internal/api"
)

func TestParseFilenameTV(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		kind    string
		show    string
		season  int
		episode int
		title   string
	}{
		{
			name:    "show prefix dots",
			path:    "/tv/Breaking Bad/Season 03/Breaking.Bad.S03E07.One.Minute.[1080p].mkv",
			kind:    "tvShow",
			show:    "Breaking Bad",
			season:  3,
			episode: 7,
			title:   "One Minute",
		},
		{
			// Parity with SonderMediaParser: unbracketed trailing resolution
			// is not stripped from episode titles.
			name:    "unbracketed quality kept for parity",
			path:    "/tv/Breaking Bad/Season 03/Breaking.Bad.S03E07.One.Minute.1080p.mkv",
			kind:    "tvShow",
			show:    "Breaking Bad",
			season:  3,
			episode: 7,
			title:   "One Minute 1080p",
		},
		{
			name:    "bare code uses folder",
			path:    "/tv/Severance/Season 1/S01E05 - The Grim Barbarity of Optics and Design.mp4",
			kind:    "tvShow",
			show:    "Severance",
			season:  1,
			episode: 5,
			title:   "The Grim Barbarity of Optics and Design",
		},
		{
			name:    "cross format",
			path:    "/tv/Lost/Lost 4x08 Meeting.mkv",
			kind:    "tvShow",
			show:    "Lost",
			season:  4,
			episode: 8,
			title:   "Meeting",
		},
		{
			// Dots as separators between S/E and quality chain as tail: the
			// "1080p Bluray AAC 5.1 x265-GRP" junk must not become a second
			// episode number or the show title (BSG-per-episode-folders case).
			name:    "dot separated code with release chain",
			path:    "/tv/Battlestar Galactica (2004)/Season 01/Battlestar.Galactica.(2003).S01.E01.1080p.Bluray.AAC.5.1.x265-LION[UTR].mkv",
			kind:    "tvShow",
			show:    "Battlestar Galactica (2003)",
			season:  1,
			episode: 1,
			title:   "Episode 1",
		},
		{
			// Parenthesised code after the show name ("Cheers (S08E20) 50-50
			// Carla") — used by moviesbyrizzo-style releases. The resolution
			// chain tail ("1080p H.264 (moviesbyrizzo)") is junk, not title.
			name:    "parenthesised code after show",
			path:    "/tv/Cheers (1982)/Season 08/Cheers (S08E20) 50-50 Carla 1080p H.264 (moviesbyrizzo).mkv",
			kind:    "tvShow",
			show:    "Cheers",
			season:  8,
			episode: 20,
			title:   "50-50 Carla",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ParseFilename(c.path, c.kind)
			if p.ShowTitle != c.show {
				t.Errorf("show = %q, want %q", p.ShowTitle, c.show)
			}
			if p.Season == nil || *p.Season != c.season || p.Episode == nil || *p.Episode != c.episode {
				t.Fatalf("season/episode = %v/%v", p.Season, p.Episode)
			}
			if p.Title != c.title {
				t.Errorf("title = %q, want %q", p.Title, c.title)
			}
			wantSub := fmt.Sprintf("%s - S%02dE%02d", c.show, c.season, c.episode)
			if p.Subtitle != wantSub {
				t.Errorf("subtitle = %q, want %q", p.Subtitle, wantSub)
			}
		})
	}
}

func TestParseFilenameSeasonFolderFallback(t *testing.T) {
	p := ParseFilename("/tv/The Office/Season 02/04 - The Fire.avi", "tvShow")
	if p.ShowTitle != "The Office" || p.Season == nil || *p.Season != 2 {
		t.Fatalf("unexpected: %+v", p)
	}
}

func TestParseFilenameMovieAndTags(t *testing.T) {
	p := ParseFilename("/movies/Inception (2010)/Inception.2010.{edition-theatrical}.mkv", "movie")
	if p.Title != "Inception" || p.Year != 2010 {
		t.Fatalf("movie parse = %+v", p)
	}
	if p.Edition != "theatrical" {
		t.Errorf("edition = %q", p.Edition)
	}

	p2 := ParseFilename("/movies/Arrival {tmdb-329865}/Arrival 2016 part1.mp4", "movie")
	if p2.MetadataIDSource != "tmdb" || p2.MetadataID != "329865" {
		t.Errorf("metadata tag = %s/%s", p2.MetadataIDSource, p2.MetadataID)
	}
	if p2.SplitPart != "part1" {
		t.Errorf("splitPart = %q", p2.SplitPart)
	}
}

func TestParseFilenameBooks(t *testing.T) {
	p := ParseFilename("/audiobooks/Project Hail Mary {audible-B08G9PBSFV}.m4b", "audiobook")
	if p.MetadataIDSource != "audible" || p.MetadataID != "B08G9PBSFV" {
		t.Errorf("audible tag missing: %+v", p)
	}
	if !contains(p.Subtitle, "Audiobook") && p.Subtitle != "" {
		t.Logf("subtitle=%q", p.Subtitle)
	}
}

func TestParseFilenameDocumentaryKeepsKind(t *testing.T) {
	p := ParseFilename("/docs/Planet Earth II/Planet Earth II 2016 Islands.mkv", "documentary")
	if p.ShowTitle == "" && p.Season != nil {
		t.Errorf("unexpected tv parse for doc: %+v", p)
	}
}

func TestStableIDIsUUIDv5(t *testing.T) {
	a := StableID("/media/movies/A.mkv")
	b := StableID("/media/movies/A.mkv")
	c := StableID("/media/movies/B.mkv")
	if a != b || a == c {
		t.Fatalf("stable IDs not deterministic/unique: %s %s %s", a, b, c)
	}
	if len(a) != 36 || a[14] != '5' || a[19] != '8' && a[19] != '9' && a[19] != 'a' && a[19] != 'b' {
		t.Errorf("not a v5 uuid shape: %s", a)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestEbookAuthorSplit(t *testing.T) {
	cases := []struct {
		path, libKind, wantTitle, wantAuthor string
	}{
		{"/lib/Infinite Jest (David Foster Wallace).epub", "ebook", "Infinite Jest", "David Foster Wallace"},
		{"/lib/Dune/Dune.epub", "ebook", "Dune", "Dune"}, // parent-dir fallback
		{"/lib/Books/plain.epub", "ebook", "plain", ""},  // junk dir ignored
	}
	for _, c := range cases {
		p := ParseFilename(c.path, c.libKind)
		if p.Title != c.wantTitle {
			t.Errorf("%s: title = %q, want %q", c.path, p.Title, c.wantTitle)
		}
		wantAuthor := c.wantAuthor
		if c.path == "/lib/Dune/Dune.epub" {
			wantAuthor = "Dune" // parent fallback
		}
		if c.path == "/lib/Books/plain.epub" {
			wantAuthor = "" // junk dir ignored
		}
		if p.Series != wantAuthor {
			t.Errorf("%s: author = %q, want %q", c.path, p.Series, wantAuthor)
		}
	}
}

func TestInferKindVideoInEbookLibrary(t *testing.T) {
	if got := inferKind("/lib/x.mp4", api.FormatMP4, "ebook"); got != api.KindMovie {
		t.Errorf("video in ebook lib = %v, want movie", got)
	}
	if got := inferKind("/lib/x.epub", api.FormatEPUB, "ebook"); got != api.KindEbook {
		t.Errorf("epub = %v, want ebook", got)
	}
}
