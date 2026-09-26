package library

import "testing"

func TestAudiobookAuthorDirFlatLayout(t *testing.T) {
	author, book, ok := audiobookAuthorDir("/audiobooks/Frank Herbert/Dune/Dune.m4b")
	if !ok || author != "Frank Herbert" || book != "Dune" {
		t.Errorf("got author=%q book=%q ok=%v", author, book, ok)
	}
}

func TestAudiobookAuthorDirInsideLegacyWrapper(t *testing.T) {
	author, book, ok := audiobookAuthorDir(
		"/audiobooks/M4B Forge Compact/compact-m4b-80k/Andy Weir/Artemis/Artemis.m4b")
	if !ok || author != "Andy Weir" || book != "Artemis" {
		t.Errorf("got author=%q book=%q ok=%v", author, book, ok)
	}
}

// The real shape: a collection between the author and the book. The
// grandparent is the collection, so a grandparent lookup published
// "Foundation - The Complete Series" as an author.
func TestAudiobookAuthorDirSkipsCollectionLevel(t *testing.T) {
	cases := []struct{ path, wantAuthor, wantBook string }{
		{
			"/audiobooks/Isaac Asimov/Foundation - The Complete Series/" +
				"Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}/" +
				"Isaac Asimov - Foundation (Jack Fox).m4b",
			"Isaac Asimov",
			"Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}",
		},
		{
			"/audiobooks/J. R. R. Tolkien/The Complete Tolkien/" +
				"J.R.R. Tolkien - The Hobbit (1937)/The Hobbit.m4b",
			"J. R. R. Tolkien",
			"J.R.R. Tolkien - The Hobbit (1937)",
		},
	}
	for _, c := range cases {
		author, book, ok := audiobookAuthorDir(c.path)
		if !ok {
			t.Errorf("%s: not resolved", c.path)
			continue
		}
		if author != c.wantAuthor {
			t.Errorf("%s: author = %q, want %q", c.path, author, c.wantAuthor)
		}
		if book != c.wantBook {
			t.Errorf("%s: book = %q, want %q", c.path, book, c.wantBook)
		}
	}
}

// The author is taken one level higher only on evidence, never by guessing
// that a name looks like a person.
func TestAudiobookAuthorDirDoesNotGuessWithoutEvidence(t *testing.T) {
	// "Dune" does not begin with "Frank Herbert", so the author stays put even
	// though the grandparent is also a plausible-looking folder.
	author, _, _ := audiobookAuthorDir("/audiobooks/Frank Herbert/Dune/Dune.m4b")
	if author != "Frank Herbert" {
		t.Errorf("author = %q", author)
	}
	// A book folder that merely shares a prefix fragment is not evidence.
	author2, _, _ := audiobookAuthorDir(
		"/audiobooks/Isaac/Isaac Newton's Isaacology/Isaac Newton's Isaacology.m4b")
	if author2 != "Isaac" {
		t.Errorf("prefix without a boundary should not count: author = %q", author2)
	}
}

func TestAudiobookAuthorDirRejectsUnusablePaths(t *testing.T) {
	for _, p := range []string{
		"/Dune.m4b",
		"/audiobooks/Dune.m4b", // directly under the root: no author folder
		"", "relative/Dune.m4b",
	} {
		if author, book, ok := audiobookAuthorDir(p); ok {
			t.Errorf("%q resolved to author=%q book=%q", p, author, book)
		}
	}
}

func TestStartsWithFolderName(t *testing.T) {
	cases := []struct {
		book, author string
		want         bool
	}{
		{"Isaac Asimov - Foundation [01]", "Isaac Asimov", true},
		{"J.R.R. Tolkien - The Hobbit", "J. R. R. Tolkien", true},
		{"Dune", "Frank Herbert", false},
		{"Dune", "Dune", false},             // identical is not evidence
		{"Isaacs and Sons", "Isaac", false}, // prefix must land on a boundary
		{"Foundation", "Foundation - The Complete Series", false},
		{"Anything", "", false},
	}
	for _, c := range cases {
		if got := startsWithFolderName(c.book, c.author); got != c.want {
			t.Errorf("startsWithFolderName(%q,%q) = %v, want %v",
				c.book, c.author, got, c.want)
		}
	}
}
