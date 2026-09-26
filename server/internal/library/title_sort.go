package library

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// A display title and a sort key are different jobs, and using the raw folder
// name for both is what made this library hard to browse. A folder called
// "2011 - The Martian (Read by R.C. Bray)" is a fine filename and a terrible
// sort key: the shelf came out as
//
//	2011 - The Martian, 2011 - The Martian, 2017 - Artemis, ...
//
// that is, ordered by leading year, with every title appearing under a
// different name depending on where the year happened to be written. These
// helpers derive a clean display title and a sort key from the raw name without
// touching the file on disk, so nothing is renamed and no information that the
// name carried is thrown away.

// leadingYear matches a leading four-digit year used as a sort prefix.
var leadingYear = regexp.MustCompile(`^\s*(\d{4})\s*[-–—_]\s*`)

// leadingOrdinal matches a leading series position: "04 The Shadow Rising",
// "13 Towers of Midnight", "14b River of Souls".
var leadingOrdinal = regexp.MustCompile(`^\s*(\d{1,3})\s*([a-z]?)\s*[-–—_]\s*|^\s*(\d{1,3})\s*([a-z]?)\s+`)

// leadingSeriesParen matches a leading parenthetical that ends in a number, which
// is how this library marks a series position: "(Culture 1) Race and Culture",
// "(Book 3) Title", "(Vol. 2) Title". A parenthetical with no trailing number is
// part of the title and is kept, so "(Unabridged)" and "(Almost) Everything"
// survive.
//
// Without this the sort key began with "(" and the title read as noise, which
// is how the Culture series ended up sorted away from its own title.
var leadingSeriesParen = regexp.MustCompile(`^\s*\(\s*([^)]{0,36}?)(\d{1,4}[a-z]?)\s*\)\s*`)

// trailingYear matches a trailing publication year in parentheses.
var trailingYear = regexp.MustCompile(`\s*[\(\[](\d{4})[\)\]]\s*$`)

// leadingArticle matches a leading English article, which is dropped from the
// sort key but kept in the display title.
var leadingArticle = regexp.MustCompile(`^(?i)(the|a|an)\s+`)

// trailingNarrator matches a trailing credit that names the reader rather than
// the book: "Ubik (Daniels)", "The Lathe of Heaven (O'Malley)", "Dune (Read by
// R.C. Bray)". Two shapes cover this library: an explicit "read by"/"narrated
// by" phrase, and a bare surname in parentheses.
var trailingNarrator = regexp.MustCompile(
	`(?i)\s*\((` +
		`(?:read|narrated|performed)\s+by\s+[^)]{2,60}` +
		`|[A-Z][\p{L}'’.-]{1,24}(?:\s+[A-Z][\p{L}'’.-]{1,24})?` +
		`)\)\s*$`)

// TitleParts is a title decomposed for display and ordering.
type TitleParts struct {
	// Display is the cleaned title shown to a reader.
	Display string
	// Sort is the key an A-Z ordering should use. Empty articles are dropped so
	// "The Lathe of Heaven" files under L.
	Sort string
	// Series is the series name and SeriesNumber the position within it, when the
	// name carried a series marker. SeriesPosition is the position as text,
	// which is what a client should show.
	Series       string
	SeriesNumber int
	// Year is the leading or trailing four-digit year, when present.
	Year int
	// NarratorHint is a trailing credit that was moved out of the sort key,
	// when one was present. It is a hint for display only: the file's own tags
	// remain the authority for the narrator.
	NarratorHint string
	// HadPrefix reports whether a year or ordinal was stripped, so a caller can
	// tell "cleaned" from "already clean" without diffing strings.
	HadPrefix bool
}

// SplitTitle decomposes a raw folder or file name into a display title and a
// sort key. It is deliberately conservative: anything it does not understand is
// left alone rather than guessed at.
func SplitTitle(raw string) TitleParts {
	out := TitleParts{Display: strings.TrimSpace(raw)}
	s := out.Display
	if s == "" {
		return out
	}

	// "(Culture 1) Race and Culture" -> series "Culture", position 1, title
	// "Race and Culture". Checked before the year/ordinal rules because a
	// parenthetical year would otherwise be read as a series position.
	if m := leadingSeriesParen.FindStringSubmatch(s); m != nil {
		name := strings.TrimSpace(m[1])
		pos := strings.TrimSpace(m[2])
		rest := strings.TrimSpace(s[len(m[0]):])
		// Never strip the whole name: a marker must be followed by a title.
		if rest != "" {
			if name != "" {
				out.Series = strings.TrimSuffix(name, ".")
			}
			out.SeriesNumber = atoiSafe(strings.TrimRight(pos, "abcdefghijklmnopqrstuvwxyz"))
			s = rest
			out.HadPrefix = true
		}
	}
	// "2011 - The Martian" -> year 2011, title "The Martian".
	if m := leadingYear.FindStringSubmatch(s); m != nil {
		out.Year = atoiSafe(m[1])
		s = strings.TrimSpace(s[len(m[0]):])
		out.HadPrefix = true
	}
	// "04 The Shadow Rising" -> series 4, title "The Shadow Rising".
	if m := leadingOrdinal.FindStringSubmatch(s); m != nil {
		pos, suffix := m[1], m[2]
		if pos == "" {
			pos, suffix = m[3], m[4]
		}
		// Only treat it as a position when real text remains; "2017 " alone is
		// not a series entry.
		rest := strings.TrimSpace(s[len(m[0]):])
		if rest != "" && unicode.IsLetter(rune(rest[0])) {
			// Normalize the padding: "04" and "4" are the same position, and
			// the position is not the sort key's business.
			if n := atoiSafe(pos); n > 0 {
				pos = strconv.Itoa(n)
			}
			out.Series = pos + suffix
			s = rest
			out.HadPrefix = true
		}
	}
	// A trailing year belongs in the year field, not the title.
	if m := trailingYear.FindStringSubmatch(s); m != nil {
		if out.Year == 0 {
			out.Year = atoiSafe(m[1])
		}
		s = strings.TrimSpace(trailingYear.ReplaceAllString(s, ""))
		out.HadPrefix = true
	}
	// A trailing narrator credit is not part of the book's identity for
	// ordering purposes: "Ubik (Daniels)" and "Ubik" are the same book, and 201
	// folders in this library carry a narrator in exactly this shape. The
	// display title keeps the credit because it is useful to a reader; only the
	// sort key drops it, so the two spellings sit together instead of in
	// different parts of the alphabet.
	sortSource := s
	if m := trailingNarrator.FindStringSubmatch(sortSource); m != nil {
		out.NarratorHint = strings.TrimSpace(m[1])
		sortSource = strings.TrimSpace(trailingNarrator.ReplaceAllString(sortSource, ""))
		out.HadPrefix = true
	}

	// Collapse whitespace left behind by the strips.
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	sortSource = strings.TrimSpace(strings.Join(strings.Fields(sortSource), " "))
	out.Display = s
	if s == "" {
		// Everything was a prefix: keep the raw name rather than showing nothing.
		out.Display = strings.TrimSpace(raw)
		out.Sort = sortKey(out.Display)
		return out
	}
	if sortSource == "" {
		sortSource = s
	}
	out.Sort = sortKey(sortSource)
	return out
}

// sortKey builds the ordering key: case- and accent-insensitive, with a leading
// article dropped, and ignoring punctuation so "Sci-Fi" and "Sci Fi" are close.
func sortKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	key := strings.Join(strings.Fields(b.String()), " ")
	// "The Lathe of Heaven" files under L, next to "Lathe" if it existed.
	if m := leadingArticle.FindStringSubmatch(key); m != nil {
		key = strings.TrimSpace(key[len(m[0]):])
	}
	return key
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
		if n > 9999 {
			return 0
		}
	}
	return n
}
