package library

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

// A book delivered as many files (a multi-part recording) is one book, not N
// books. Before this grouping existed a 148-file audiobook appeared as 148
// rows in the catalog and 148 rows on the browse page, which is the single
// worst thing that can happen to an alphabetical listing.
//
// The group key is the book's folder relative to the library root, so every
// file in one Author/Book folder groups together. Identity is derived rather
// than stored, so a folder rename re-derives the grouping without rewriting
// every item, and the opaque ID never reveals an absolute filesystem path.

// bookRoots returns the per-library root for audiobook libraries so the book
// folder can be derived from a library-relative path.
func bookRoots(libraries []config.Library) map[string]browsingRoot {
	roots := map[string]browsingRoot{}
	for _, lib := range libraries {
		if lib.Kind == "audiobook" {
			root := browsingRoot{path: filepath.Clean(lib.Path), realPath: filepath.Clean(lib.Path)}
			// A library may point at a symlink (~/NAS -> /Volumes/14tb) while
			// stored paths keep the resolved mount. Resolve once per request
			// rather than once per file.
			if resolved, err := filepath.EvalSymlinks(root.path); err == nil {
				root.realPath = filepath.Clean(resolved)
			}
			roots[lib.ID] = root
		}
	}
	return roots
}

// bookFolderKey returns the library-relative book folder for an audiobook item.
// Unlike the TV case it is the *parent* directory: a book's identity lives in
// Author/Book/ while a show's lives in Show/.
func bookFolderKey(item *Item, root browsingRoot) (string, bool) {
	relative := ""
	if item.SourceRelativePath != "" {
		relative = filepath.Clean(filepath.FromSlash(item.SourceRelativePath))
		if filepath.IsAbs(relative) || relative == "." || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			relative = ""
		}
	}
	if relative == "" && item.FilePath != "" {
		for _, base := range []string{root.path, root.realPath} {
			if base == "" {
				continue
			}
			candidate, err := filepath.Rel(base, item.FilePath)
			if err == nil && candidate != "." && candidate != ".." &&
				!strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
				relative = candidate
				break
			}
		}
	}
	if relative == "" {
		return "", false
	}
	dir := filepath.Dir(relative)
	if dir == "." || dir == "" || dir == ".." ||
		strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
		return "", false
	}
	return dir, true
}

// BookGrouping is the derived position of one item within its book.
type BookGrouping struct {
	ID    string
	Title string
	Index int
	Count int
}

// BookConflict describes why a book's files were not simply summed, so a
// client can surface the problem instead of presenting a wrong total.
type BookConflict struct {
	// Kind is "duplicate_copies" (several files are each the whole book) or
	// "whole_book_sibling" (one file is the whole book sitting beside its own
	// segments).
	Kind string
	// ExcludedIDs are the item IDs left out of the book's parts.
	ExcludedIDs []string
	Detail      string
}

// BookGroups returns per-book grouping plus, for folders whose files are not a
// clean set of segments, a description of the conflict.
func BookGroups(items []*Item, libraries []config.Library) (map[string]BookGrouping, map[string]BookConflict) {
	roots := bookRoots(libraries)
	byFolder := map[string][]*Item{}
	libraryOf := map[string]string{}

	for _, item := range items {
		if item.Kind != api.KindAudiobook || item.LibraryID == nil {
			continue
		}
		root, ok := roots[*item.LibraryID]
		if !ok {
			continue
		}
		folder, ok := bookFolderKey(item, root)
		if !ok {
			continue
		}
		byFolder[folder] = append(byFolder[folder], item)
		libraryOf[folder] = *item.LibraryID
	}

	groupings := make(map[string]BookGrouping)
	conflicts := map[string]BookConflict{}
	for folder, group := range byFolder {
		parts, conflict := selectBookParts(group)
		// The id is derived from the folder, not from the parts, so a conflict
		// is reported even when no book was created for the folder.
		id := fmt.Sprintf("book-%x", sha256.Sum256([]byte(libraryOf[folder]+"\x00"+folder)))
		if conflict != nil {
			conflicts[id] = *conflict
		}
		if len(parts) == 0 {
			continue
		}
		sortParts(parts)
		title := filepath.Base(folder)
		for i, item := range parts {
			groupings[item.ID] = BookGrouping{ID: id, Title: title, Index: i + 1, Count: len(parts)}
		}
	}
	return groupings, conflicts
}

// selectBookParts decides which of a folder's files are genuine segments of one
// book, and reports the rest rather than quietly counting them.
//
// A multi-part book has several files of broadly comparable length that sum to
// the book's runtime. Two other shapes show up in this library and must not be
// summed into one row:
//
//   - an empty leftover (a 0-byte file, or a .sonder-retag temp artifact),
//     which is not a part of anything
//   - a whole-book file sitting beside its own segments, so summing doubles
//     the runtime (Echopraxia: one 12.65 h file plus ten 1.25 h segments)
//   - two or three complete copies of the same book, which is a duplicate, not
//     a part set
func selectBookParts(group []*Item) ([]*Item, *BookConflict) {
	var usable []*Item
	var empty []*Item
	for _, it := range group {
		if it.SizeBytes <= 0 || it.DurationSeconds <= 0 {
			empty = append(empty, it)
			continue
		}
		usable = append(usable, it)
	}
	if len(usable) == 0 {
		return nil, nil
	}
	if len(usable) == 1 {
		if len(empty) > 0 {
			return usable, &BookConflict{
				Kind:        "empty_leftovers",
				ExcludedIDs: itemIDs(empty),
				Detail: fmt.Sprintf("%d empty file(s) in this book's folder were "+
					"left out of its parts", len(empty)),
			}
		}
		return usable, nil
	}

	// Two comparable files cannot be a reliable segment set: they are far more
	// likely to be two complete copies (this library has such a pair). Three or
	// more are treated as a part set, which covers 3-CD releases.
	if len(usable) < 3 {
		return nil, &BookConflict{
			Kind:        "duplicate_copies",
			ExcludedIDs: itemIDs(usable),
			Detail: fmt.Sprintf("%d files of comparable length look like separate "+
				"complete copies of this book, not parts of one; not merged",
				len(usable)),
		}
	}

	median := medianDuration(usable)
	// A file far longer than its siblings is the whole book sitting beside its
	// own segments.
	threshold := 2 * median
	var outliers []*Item
	var parts []*Item
	for _, it := range usable {
		if it.DurationSeconds > threshold {
			outliers = append(outliers, it)
			continue
		}
		parts = append(parts, it)
	}
	if len(outliers) == 1 && len(parts) >= 2 {
		excluded := append([]*Item{}, outliers...)
		excluded = append(excluded, empty...)
		return parts, &BookConflict{
			Kind:        "whole_book_sibling",
			ExcludedIDs: itemIDs(excluded),
			Detail: fmt.Sprintf("one file (%.2f h) is the whole book sitting "+
				"beside its own %.2f h segments and was not counted as a part",
				outliers[0].DurationSeconds/3600, sumDuration(parts)/3600),
		}
	}
	if len(outliers) > 1 {
		// Several long files among shorter ones: not a shape we can resolve.
		return nil, &BookConflict{
			Kind:        "duplicate_copies",
			ExcludedIDs: itemIDs(usable),
			Detail: fmt.Sprintf("%d of %d files are far longer than the rest; "+
				"this folder is not a simple part set and was not merged",
				len(outliers), len(usable)),
		}
	}
	if len(empty) > 0 {
		return parts, &BookConflict{
			Kind:        "empty_leftovers",
			ExcludedIDs: itemIDs(empty),
			Detail: fmt.Sprintf("%d empty file(s) in this book's folder were "+
				"left out of its parts", len(empty)),
		}
	}
	return parts, nil
}

func itemIDs(items []*Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func sumDuration(items []*Item) float64 {
	var total float64
	for _, it := range items {
		total += it.DurationSeconds
	}
	return total
}

func medianDuration(items []*Item) float64 {
	d := make([]float64, 0, len(items))
	for _, it := range items {
		d = append(d, it.DurationSeconds)
	}
	sort.Float64s(d)
	return d[len(d)/2]
}

// sortParts orders a book's segments the way a track list reads.
func sortParts(parts []*Item) {
	sort.Slice(parts, func(a, b int) bool {
		an := filepath.Base(parts[a].FilePath)
		bn := filepath.Base(parts[b].FilePath)
		if naturalLess(an, bn) {
			return true
		}
		if naturalLess(bn, an) {
			return false
		}
		return parts[a].ID < parts[b].ID
	})
}

// BookGroupings derives book identity for every audiobook in the store. Items
// whose book folder cannot be determined (a library root that is itself the
// book, an unreadable path) are absent from the result rather than grouped
// under a placeholder, so a client can fall back to per-file listing for them.
func BookGroupings(items []*Item, libraries []config.Library) map[string]BookGrouping {
	g, _ := BookGroups(items, libraries)
	return g
}

// naturalLess orders filenames the way a human reads them. Digits compare
// numerically and sort before letters, so "01 - Title" precedes "Bonus track"
// the way a track list reads.
func naturalLess(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	for i := 0; i < len(ar) && i < len(br); {
		ad, bd := unicode.IsDigit(ar[i]), unicode.IsDigit(br[i])
		switch {
		case ad && bd:
			// Both digit runs start at the same index. Measuring the second
			// run from the already-advanced cursor would compare the wrong
			// characters, so measure each from `start` and then re-align.
			start := i
			ai := start
			for ai < len(ar) && unicode.IsDigit(ar[ai]) {
				ai++
			}
			bi := start
			for bi < len(br) && unicode.IsDigit(br[bi]) {
				bi++
			}
			an, aerr := parseUint(ar, start, ai)
			bn, berr := parseUint(br, start, bi)
			if aerr == nil && berr == nil {
				if an != bn {
					return an < bn
				}
				// Equal runs: skip past both so the comparison resumes
				// aligned even when one was zero-padded ("01" vs "1").
				i = ai
				if bi > i {
					i = bi
				}
				continue
			}
			// A run too wide for uint64 falls through to a literal rune
			// comparison rather than wrapping and mis-ordering.
			i = ai
			if bi > i {
				i = bi
			}
		case ad != bd:
			return ad
		default:
			al, bl := unicode.ToLower(ar[i]), unicode.ToLower(br[i])
			if al != bl {
				return al < bl
			}
			i++
		}
	}
	return len(ar) < len(br)
}

// parseUint reads s[from:to] as a number. Values wider than 64 bits return an
// error so the caller falls through to the next comparison rather than
// silently wrapping and ordering a 36-part book by arithmetic accident.
func parseUint(s []rune, from, to int) (uint64, error) {
	if from >= to || to > len(s) {
		return 0, fmt.Errorf("empty or out of range")
	}
	var n uint64
	for _, r := range s[from:to] {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a digit")
		}
		d := uint64(r - '0')
		if n > (^uint64(0)-d)/10 {
			return 0, fmt.Errorf("overflow")
		}
		n = n*10 + d
	}
	return n, nil
}
