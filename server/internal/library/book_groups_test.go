package library

import (
	"path/filepath"
	"sort"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func audiobookLibraries(root string) []config.Library {
	return []config.Library{{ID: "books", Name: "Audiobooks", Path: root, Kind: "audiobook"}}
}

func mkBookItem(t *testing.T, root, rel string) *Item {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	author := filepath.Base(filepath.Dir(filepath.Dir(full)))
	return &Item{
		MediaItem: api.MediaItem{
			ID:              rel,
			Kind:            api.KindAudiobook,
			Title:           filepath.Base(filepath.Dir(full)),
			LibraryID:       strPtr("books"),
			DurationSeconds: 600,
			Author:          &author,
		},
		FilePath:           full,
		SourceRelativePath: filepath.ToSlash(rel),
		// A real file has a size; grouping treats a zero size as an empty
		// leftover, so the fixture must not look like one.
		SizeBytes: 5_000_000,
	}
}

func strPtr(s string) *string { return &s }

func TestBookGroupingCollapsesOneBookFromManyFiles(t *testing.T) {
	root := t.TempDir()
	items := []*Item{
		mkBookItem(t, root, "Atul Gawande/Complications/01.mp3"),
		mkBookItem(t, root, "Atul Gawande/Complications/02.mp3"),
		mkBookItem(t, root, "Atul Gawande/Complications/03.mp3"),
		mkBookItem(t, root, "Andy Weir/Dune/Dune.m4b"),
	}
	g := BookGroupings(items, audiobookLibraries(root))

	compl := g["Atul Gawande/Complications/01.mp3"]
	if compl.Count != 3 {
		t.Errorf("part count = %d, want 3", compl.Count)
	}
	if compl.Title != "Complications" {
		t.Errorf("title = %q", compl.Title)
	}
	if g["Andy Weir/Dune/Dune.m4b"].Count != 1 {
		t.Errorf("single-file book count = %d, want 1", g["Andy Weir/Dune/Dune.m4b"].Count)
	}
	// Books must not share an identity even at the same path shape.
	if compl.ID == g["Andy Weir/Dune/Dune.m4b"].ID {
		t.Error("distinct books share a group id")
	}
}

// Playback order must follow the book's own numbering. Lexical ordering puts
// "10" before "2", which would shuffle a 36-file book.
func TestBookGroupingOrdersPartsNaturally(t *testing.T) {
	root := t.TempDir()
	var items []*Item
	for i := 1; i <= 12; i++ {
		items = append(items, mkBookItem(t, root,
			"Larry Niven/The Ringworld Engineers/part"+pad(i)+".mp3"))
	}
	g := BookGroupings(items, audiobookLibraries(root))
	byID := map[string]int{}
	for id, v := range g {
		byID[id] = v.Index
	}
	for i := 1; i <= 12; i++ {
		want := i
		if got := byID["Larry Niven/The Ringworld Engineers/part"+pad(i)+".mp3"]; got != want {
			t.Errorf("part %d index = %d, want %d", i, got, want)
		}
	}
}

// naturalLess must behave like a real sort, not just pass pairwise examples:
// antisymmetric, and transitive enough that sort.Slice does not silently
// scramble a 36-file book. This is the check that catches cursor misalignment
// between strings of different digit-run lengths.
func TestNaturalLessSortsRealisticTrackLists(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "zero padded two digit",
			input: []string{"10.mp3", "02.mp3", "01.mp3", "11.mp3"},
			want:  []string{"01.mp3", "02.mp3", "10.mp3", "11.mp3"},
		},
		{
			name:  "mixed padding",
			input: []string{"1.mp3", "10.mp3", "2.mp3", "20.mp3", "3.mp3"},
			want:  []string{"1.mp3", "2.mp3", "3.mp3", "10.mp3", "20.mp3"},
		},
		{
			name:  "disc and track",
			input: []string{"cd02-01.mp3", "cd01-20.mp3", "cd01-02.mp3", "cd02-10.mp3"},
			want:  []string{"cd01-02.mp3", "cd01-20.mp3", "cd02-01.mp3", "cd02-10.mp3"},
		},
		{
			name:  "digits before letters",
			input: []string{"Bonus.mp3", "01 - Title.mp3", "Afterword.mp3"},
			want:  []string{"01 - Title.mp3", "Afterword.mp3", "Bonus.mp3"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := append([]string(nil), c.input...)
			sort.Slice(got, func(a, b int) bool { return naturalLess(got[a], got[b]) })
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("sorted = %v, want %v", got, c.want)
				}
			}
			// Antisymmetry: the comparator must never claim both orders.
			for _, x := range got {
				for _, y := range got {
					if x != y && naturalLess(x, y) && naturalLess(y, x) {
						t.Errorf("naturalLess claims both %q<%q and %q<%q", x, y, y, x)
					}
				}
			}
		})
	}
}

func pad(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"part2.mp3", "part10.mp3", true},
		{"part10.mp3", "part2.mp3", false},
		{"01 - Title.mp3", "02 - Title.mp3", true},
		{"01 - Title.mp3", "Bonus.mp3", true}, // digits before letters
		{"A.mp3", "B.mp3", true},
		{"a.mp3", "B.mp3", true}, // case-insensitive
		{"03 - cd01-01.mp3", "03 - cd01-02.mp3", true},
		{"same.mp3", "same.mp3", false}, // equal is not less
		{"x9.mp3", "x10.mp3", true},
	}
	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// A file that is not under an author/book folder has no book identity. It must
// still be listed, so the caller keeps it rather than dropping it.
func TestBookGroupingSkipsUnresolvableFolders(t *testing.T) {
	root := t.TempDir()
	items := []*Item{mkBookItem(t, root, "Loose Book.m4b")}
	if g := BookGroupings(items, audiobookLibraries(root)); len(g) != 0 {
		t.Errorf("grouped an unresolvable path: %+v", g)
	}
}

func TestBookGroupingIgnoresOtherKinds(t *testing.T) {
	root := t.TempDir()
	it := mkBookItem(t, root, "Author/Book/Book.mp3")
	it.Kind = api.KindEbook
	if g := BookGroupings([]*Item{it}, audiobookLibraries(root)); len(g) != 0 {
		t.Errorf("grouped a non-audiobook: %+v", g)
	}
}

func TestBookGroupingIsStableAcrossCalls(t *testing.T) {
	root := t.TempDir()
	items := []*Item{
		mkBookItem(t, root, "Author/Book/a.mp3"),
		mkBookItem(t, root, "Author/Book/b.mp3"),
	}
	first := BookGroupings(items, audiobookLibraries(root))
	// Shuffle input order: the grouping must not depend on map or input order.
	reordered := []*Item{items[1], items[0]}
	second := BookGroupings(reordered, audiobookLibraries(root))
	for id, v := range first {
		if second[id] != v {
			t.Errorf("%s: first %+v, second %+v", id, v, second[id])
		}
	}
}

// A book folder that survives a rename must produce a different group, so the
// client shows the new title rather than a stale one.
func TestBookGroupingFollowsFolderName(t *testing.T) {
	root := t.TempDir()
	items := make([]*Item, 0, 4)
	for i := 1; i <= 4; i++ {
		items = append(items, mkBookItem(t, root, "Author/Old Name/"+pad(i)+".mp3"))
	}
	g := BookGroupings(items, audiobookLibraries(root))
	if got := g["Author/Old Name/01.mp3"].Title; got != "Old Name" {
		t.Errorf("title = %q, want %q", got, "Old Name")
	}
	renamed := make([]*Item, 0, 4)
	for i := 1; i <= 4; i++ {
		renamed = append(renamed, mkBookItem(t, root, "Author/New Name/"+pad(i)+".mp3"))
	}
	g2 := BookGroupings(renamed, audiobookLibraries(root))
	if got := g2["Author/New Name/01.mp3"].Title; got != "New Name" {
		t.Errorf("renamed title = %q, want %q", got, "New Name")
	}
	if g2["Author/New Name/01.mp3"].ID == g["Author/Old Name/01.mp3"].ID {
		t.Error("group id survived a folder rename")
	}
}

// A 0-byte file is a leftover, not a part. Counting one inflated a book's part
// list and produced a book that looked like two books.
func TestBookGroupingExcludesEmptyLeftovers(t *testing.T) {
	root := t.TempDir()
	var items []*Item
	for i := 1; i <= 4; i++ {
		items = append(items, mkBookItem(t, root, "PKD/Stories/"+pad(i)+".m4b"))
	}
	leftover := mkBookItem(t, root, "PKD/Stories/Stories.m4b.sonder-retag.m4b")
	leftover.SizeBytes = 0
	leftover.DurationSeconds = 0
	items = append(items, leftover)

	g, conflicts := BookGroups(items, audiobookLibraries(root))
	for _, it := range items[:4] {
		v, ok := g[it.ID]
		if !ok {
			t.Fatalf("real part not grouped: %s", it.ID)
		}
		if v.Count != 4 {
			t.Errorf("count = %d, want 4 (the empty file must not be a part)", v.Count)
		}
	}
	if _, ok := g[leftover.ID]; ok {
		t.Error("empty leftover was grouped as a part")
	}
	var found *BookConflict
	for _, c := range conflicts {
		if c.Kind == "empty_leftovers" {
			cc := c
			found = &cc
		}
	}
	if found == nil {
		t.Fatalf("no empty_leftovers conflict reported: %+v", conflicts)
	}
	if len(found.ExcludedIDs) != 1 || found.ExcludedIDs[0] != leftover.ID {
		t.Errorf("excluded = %v, want the leftover", found.ExcludedIDs)
	}
}

// Summing a whole-book file with its own segments doubles the runtime. This is
// the real Echopraxia shape: one 12.65 h file beside ten 1.25 h segments.
func TestBookGroupingExcludesWholeBookSibling(t *testing.T) {
	root := t.TempDir()
	var items []*Item
	for i := 1; i <= 10; i++ {
		it := mkBookItem(t, root, "Peter Watts/Echopraxia/"+pad(i)+".m4b")
		it.DurationSeconds = 1.25 * 3600
		items = append(items, it)
	}
	whole := mkBookItem(t, root, "Peter Watts/Echopraxia/Echopraxia.m4b")
	whole.DurationSeconds = 12.65 * 3600
	items = append(items, whole)

	g, conflicts := BookGroups(items, audiobookLibraries(root))
	part := g["Peter Watts/Echopraxia/01.m4b"]
	if part.Count != 10 {
		t.Errorf("count = %d, want 10 segments", part.Count)
	}
	if _, ok := g[whole.ID]; ok {
		t.Error("the whole-book file was counted as a segment")
	}
	var kinds []string
	for _, c := range conflicts {
		kinds = append(kinds, c.Kind)
	}
	if len(kinds) != 1 || kinds[0] != "whole_book_sibling" {
		t.Errorf("conflicts = %v, want one whole_book_sibling", kinds)
	}
}

// Two or three comparable files are competing complete copies, not a part set.
// Merging them would invent a book that does not exist.
func TestBookGroupingRefusesToMergeFewComparableFiles(t *testing.T) {
	root := t.TempDir()
	var items []*Item
	for i, name := range []string{"A.m4b", "B.m4b"} {
		it := mkBookItem(t, root, "Iain M. Banks/The State of the Art/"+name)
		it.DurationSeconds = float64(6+i) * 3600
		items = append(items, it)
	}
	g, conflicts := BookGroups(items, audiobookLibraries(root))
	if len(g) != 0 {
		t.Errorf("merged duplicate copies into a book: %+v", g)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want exactly one", conflicts)
	}
	for _, c := range conflicts {
		if c.Kind != "duplicate_copies" {
			t.Errorf("kind = %q, want duplicate_copies", c.Kind)
		}
		if len(c.ExcludedIDs) != 2 {
			t.Errorf("excluded = %v, want both files", c.ExcludedIDs)
		}
	}
}

// A folder whose only file is empty must not vanish from the catalog.
func TestBookGroupingAllEmptyFolderIsNotGrouped(t *testing.T) {
	root := t.TempDir()
	it := mkBookItem(t, root, "Author/Book/Book.m4b")
	it.SizeBytes = 0
	it.DurationSeconds = 0
	if g := BookGroupings([]*Item{it}, audiobookLibraries(root)); len(g) != 0 {
		t.Errorf("grouped an empty folder: %+v", g)
	}
}

func TestMedianDuration(t *testing.T) {
	mk := func(d ...float64) []*Item {
		out := make([]*Item, 0, len(d))
		for _, v := range d {
			out = append(out, &Item{MediaItem: api.MediaItem{DurationSeconds: v}})
		}
		return out
	}
	if got := medianDuration(mk(1, 2, 3)); got != 2 {
		t.Errorf("median(1,2,3) = %v, want 2", got)
	}
	if got := medianDuration(mk(5)); got != 5 {
		t.Errorf("median(5) = %v, want 5", got)
	}
}

func TestBookGroupingSortsGroupsForDeterministicOutput(t *testing.T) {
	root := t.TempDir()
	var titles []string
	for _, name := range []string{"Zebra", "Apple", "Mango"} {
		items := []*Item{mkBookItem(t, root, "Author/"+name+"/book.mp3")}
		for _, g := range BookGroupings(items, audiobookLibraries(root)) {
			titles = append(titles, g.Title)
		}
	}
	sort.Strings(titles)
	if titles[0] != "Apple" || titles[2] != "Zebra" {
		t.Errorf("titles = %v", titles)
	}
}
