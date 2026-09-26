package library

import (
	"path/filepath"
	"strings"
)

// An audiobook normally lives at Author/Book/file. Some libraries put a
// collection between the two:
//
//	Isaac Asimov/Foundation - The Complete Series/
//	    Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}/
//	        Isaac Asimov - Foundation (Jack Fox).m4b
//
// A grandparent lookup -- which is what both the parser and the Jellyfin
// adapter used to do -- lands on "Foundation - The Complete Series", so a
// series name was published as an author and 20 books sat in the wrong author
// index entry.
//
// The depth cannot be guessed from the folder name alone, but it can be
// detected: in the nested layout the *book* folder already names the author
// ("Isaac Asimov - Foundation [01] ..."), because the book is one volume of a
// set. In the normal layout it does not ("Dune" under "Frank Herbert"). So the
// author is the ancestor whose name the book folder starts with.
// AudiobookAuthorDir is the exported form, for callers outside this package
// that must agree with the parser about where the author is.
func AudiobookAuthorDir(mediaPath string) (author, bookFolder string, ok bool) {
	return audiobookAuthorDir(mediaPath)
}

func audiobookAuthorDir(mediaPath string) (author, bookFolder string, ok bool) {
	bookFolder = strings.TrimSpace(filepath.Base(filepath.Dir(mediaPath)))
	if bookFolder == "" || bookFolder == "." || bookFolder == string(filepath.Separator) {
		return "", "", false
	}
	// The normal Author/Book/file case.
	parent := strings.TrimSpace(filepath.Base(filepath.Dir(filepath.Dir(mediaPath))))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return "", "", false
	}
	// If the book folder already names the author, the folder above it is a
	// collection and the real author is one level higher again.
	grandparent := ""
	if gp := filepath.Dir(filepath.Dir(filepath.Dir(mediaPath))); gp != "" {
		grandparent = strings.TrimSpace(filepath.Base(gp))
	}
	if grandparent != "" &&
		grandparent != "." && grandparent != string(filepath.Separator) &&
		startsWithFolderName(bookFolder, grandparent) {
		return grandparent, bookFolder, true
	}
	return parent, bookFolder, true
}

// startsWithFolderName reports whether a book folder name begins with an author
// folder name. Comparison ignores case, punctuation and the author's own
// " - " separator, so "Isaac Asimov - Foundation [01]" matches "Isaac Asimov"
// and "J.R.R. Tolkien - The Hobbit" matches "J. R. R. Tolkien".
func startsWithFolderName(bookFolder, author string) bool {
	a := foldName(author)
	if a == "" {
		return false
	}
	b := foldName(bookFolder)
	if b == a {
		return false // a book folder identical to the author is not evidence
	}
	if !strings.HasPrefix(b, a) {
		return false
	}
	// The remainder must begin at a boundary, so "Isaacs" does not match
	// "Isaac".
	rest := b[len(a):]
	if rest == "" {
		return true
	}
	switch rest[0] {
	case ' ', '-', '_', '.', ',', ':', '/', '[':
		return true
	}
	return false
}

// foldName reduces a name to comparable letters and digits.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
