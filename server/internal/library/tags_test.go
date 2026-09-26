package library

import (
	"os"
	"path/filepath"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
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
		// Initials and dotted names must survive: cutting on any period turned
		// "R.C. Bray" into "R".
		{"Narrated by R.C. Bray", "R.C. Bray"},
		{"Narrated by R.C. Bray.", "R.C. Bray"},
		{"Narrated by R. C. Bray", "R. C. Bray"},
		{"Narrated by A.C. Bhaktivedanta Swami", "A.C. Bhaktivedanta Swami"},
		{"Narrated by J. R. R. Tolkien", "J. R. R. Tolkien"},
		// A real sentence boundary still ends the name.
		{"Narrated by Ray Porter. Recorded 2019.", "Ray Porter"},
		{"Narrated by Kate Reading; 2019 edition", "Kate Reading"},
		// Honorifics and titles belong to the name.
		{"Narrated by Dr. Smith", "Dr. Smith"},
		{"Narrated by St. Claire", "St. Claire"},
		{"Narrated by Mr. X", "Mr. X"},
		{"Narrated by Prof. Killian", "Prof. Killian"},
		{"Narrated by Miss Piggy", "Miss Piggy"},
		// A short capitalized word that is not an abbreviation still ends the
		// name, so the rule cannot be a blanket "short token".
		{"Narrated by Ray Porter. Smith agreed.", "Ray Porter"},
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

// A tag read must be able to correct an earlier tag read. Before provenance
// was tracked, "Narrated by R.C. Bray" was parsed to "R", and no amount of
// re-reading could ever repair it because a non-empty field was never
// overwritten.
func TestApplyFileTagsRefreshesItsOwnEarlierValue(t *testing.T) {
	stale := "R"
	it := &Item{MediaItem: api.MediaItem{Narrator: &stale}, NarratorFromTags: true}
	applyFileTags(it, probe.FileTags{Comment: "Narrated by R.C. Bray"})
	if it.Narrator == nil || *it.Narrator != "R.C. Bray" {
		t.Errorf("narrator = %v, want the corrected %q", it.Narrator, "R.C. Bray")
	}
	if !it.NarratorFromTags {
		t.Error("provenance was lost")
	}
}

// A provider's narrator must still win, or an enrichment pass would be undone
// by the next re-probe.
func TestApplyFileTagsNeverDisplacesProviderNarrator(t *testing.T) {
	provider := "Enriched Narrator"
	it := &Item{MediaItem: api.MediaItem{Narrator: &provider}}
	applyFileTags(it, probe.FileTags{Comment: "Narrated by Someone Else"})
	if it.Narrator == nil || *it.Narrator != "Enriched Narrator" {
		t.Errorf("narrator = %v, want the provider value to survive", it.Narrator)
	}
	if it.NarratorFromTags {
		t.Error("provenance was claimed for a value the tags did not set")
	}
}

// Provenance has to survive a rebuild, or the value looks provider-supplied on
// the next pass and becomes uncorrectable again.
func TestNarratorProvenanceSurvivesRebuild(t *testing.T) {
	root := t.TempDir()
	book := filepath.Join(root, "Audiobooks", "Andy Weir", "The Martian", "The Martian.m4b")
	if err := os.MkdirAll(filepath.Dir(book), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(book, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New()
	sc := NewScanner(store)
	lib := []config.Library{{ID: "books", Name: "Audiobooks", Path: filepath.Join(root, "Audiobooks"), Kind: "audiobook"}}
	fp := &fakeProber{}
	sc.SetProber(fp, 1)
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	// Simulate a tag read that produced a wrong value.
	id := store.Items()[0].ID
	store.Update(id, func(it *Item) bool {
		bad := "R"
		it.Narrator = &bad
		it.NarratorFromTags = true
		it.ProbedTagsRead = false
		return true
	})
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	it := store.InternalItems()[0]
	if !it.NarratorFromTags {
		t.Error("provenance was not carried through the rebuild")
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
