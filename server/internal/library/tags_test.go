package library

import (
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/probe"
)

func TestApplyFileTagsFillsEmptyFields(t *testing.T) {
	it := &Item{}
	applyFileTags(it, probe.FileTags{
		Title:       "Project Hail Mary (2021)",
		AlbumArtist: "Andy Weir",
		Comment:     "Narrated by Ray Porter",
		Date:        "2021",
		Genre:       "Science Fiction",
		Description: "Ryland Grace is the sole survivor.",
	})
	if it.Author == nil || *it.Author != "Andy Weir" {
		t.Errorf("author = %v", it.Author)
	}
	if it.Narrator == nil || *it.Narrator != "Ray Porter" {
		t.Errorf("narrator = %v", it.Narrator)
	}
	if it.Year != 2021 {
		t.Errorf("year = %d", it.Year)
	}
	if len(it.Genres) != 1 || it.Genres[0] != "Science Fiction" {
		t.Errorf("genres = %v", it.Genres)
	}
	if it.Summary != "Ryland Grace is the sole survivor." {
		t.Errorf("summary = %q", it.Summary)
	}
}

// album_artist is the audiobook author credit; artist is the fallback for
// files that only carry the per-track credit.
func TestApplyFileTagsAuthorPreference(t *testing.T) {
	it := &Item{}
	applyFileTags(it, probe.FileTags{Artist: "Per-Track", AlbumArtist: "Book Author"})
	if it.Author == nil || *it.Author != "Book Author" {
		t.Errorf("author = %v, want the album_artist", it.Author)
	}
	it2 := &Item{}
	applyFileTags(it2, probe.FileTags{Artist: "Only Artist"})
	if it2.Author == nil || *it2.Author != "Only Artist" {
		t.Errorf("author = %v, want the artist fallback", it2.Author)
	}
}

// A metadata provider or manual edit outranks the tag, and a tag must never
// blank out something already established.
func TestApplyFileTagsNeverOverwritesExisting(t *testing.T) {
	existingAuthor := "Enriched Author"
	existingNarrator := "Enriched Narrator"
	it := &Item{MediaItem: api.MediaItem{
		Author:   &existingAuthor,
		Narrator: &existingNarrator,
		Summary:  "Enriched summary",
		Year:     1999,
		Genres:   []string{"Existing"},
	}}
	applyFileTags(it, probe.FileTags{
		AlbumArtist: "Tag Author",
		Comment:     "Narrated by Tag Narrator",
		Description: "Tag summary",
		Date:        "2001",
		Genre:       "Tag Genre",
	})
	if *it.Author != "Enriched Author" {
		t.Errorf("author overwritten = %q", *it.Author)
	}
	if *it.Narrator != "Enriched Narrator" {
		t.Errorf("narrator overwritten = %q", *it.Narrator)
	}
	if it.Summary != "Enriched summary" {
		t.Errorf("summary overwritten = %q", it.Summary)
	}
	if it.Year != 1999 {
		t.Errorf("year overwritten = %d", it.Year)
	}
	if len(it.Genres) != 1 || it.Genres[0] != "Existing" {
		t.Errorf("genres overwritten = %v", it.Genres)
	}
}

func TestNarratorFromTags(t *testing.T) {
	cases := []struct {
		comment, want string
	}{
		{"Narrated by Ray Porter", "Ray Porter"},
		{"Narrated by: Kate Reading", "Kate Reading"},
		{"narrated by Ray Porter.", "Ray Porter"},
		{"Read by Simon Vance", "Simon Vance"},
		{"Narrated and read by Scott Brick", "Scott Brick"},
		{"With John Lee Mahoney as narrator", "John Lee Mahoney"},
		{"A great book with no credit", ""},
		{"", ""},
		{"Narrated by   ", ""},
	}
	for _, c := range cases {
		if got := narratorFromTags(probe.FileTags{Comment: c.comment}); got != c.want {
			t.Errorf("narratorFromTags(%q) = %q, want %q", c.comment, got, c.want)
		}
	}
}

// A summary comment that merely contains the word "narrator" must not be
// mistaken for a narrator credit.
func TestNarratorFromTagsDoesNotInventNames(t *testing.T) {
	if got := narratorFromTags(probe.FileTags{Comment: "The narrator changes in part two"}); got != "" {
		t.Errorf("invented narrator %q", got)
	}
}

func TestYearFromTags(t *testing.T) {
	cases := map[string]int{
		"2021":        2021,
		"2021-05-04":  2021,
		"2021/05/04":  2021,
		"2003":        2003,
		"19 Mar 2021": 0, // no leading 4-digit run
		"":            0,
		"unknown":     0,
		"12345":       1234, // only the leading 4-digit run is read
		"87":          0,
		"0001":        0, // below the plausible range
	}
	for in, want := range cases {
		if got := yearFromTags(in); got != want {
			t.Errorf("yearFromTags(%q) = %d, want %d", in, got, want)
		}
	}
}
