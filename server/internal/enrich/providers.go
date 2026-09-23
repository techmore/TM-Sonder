package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"strings"
	"unicode"
)

// --- Audnexus (audiobooks) ---

type audnexusBook struct {
	Description string `json:"description"`
	Publisher   string `json:"publisher"`
	Image       string `json:"image"`
	Genres      []struct {
		Name string `json:"name"`
	} `json:"genres"`
	Authors []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Narrators []struct {
		Name string `json:"name"`
	} `json:"narrators"`
}

// firstNamed returns the first contributor name from an audnexus list.
func firstNamed(xs []struct {
	Name string `json:"name"`
}) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0].Name
}

// wikiInfobox fetches the rendered article page and extracts the infobox
// poster (native resolution) plus the Genre/Genres row. Genres make the web
// UI's genre facets meaningful for films and series, which have no other
// genre source. Returns empty values when the page has no usable infobox.
func (e *Enricher) wikiInfobox(ctx context.Context, pageTitle string) (string, []string) {
	base, err := url.Parse(wikiBaseURL)
	if err != nil {
		return "", nil
	}
	base.Path, base.RawPath, base.RawQuery = "", "", ""
	pageURL := strings.TrimRight(base.String(), "/") + "/wiki/" + url.PathEscape(strings.ReplaceAll(pageTitle, " ", "_"))
	data, err := e.fetch(ctx, pageURL)
	if err != nil {
		return "", nil
	}
	doc := string(data)

	infobox := strings.Index(doc, `<table class="infobox`)
	if infobox < 0 {
		infobox = strings.Index(doc, `class="infobox`) // mobile/alternate markup
	}
	if infobox < 0 {
		return "", nil
	}
	window := doc[infobox:]
	if end := strings.Index(window, "</table>"); end >= 0 {
		window = window[:end]
	}
	if len(window) > 24<<10 {
		window = window[:24<<10] // infobox content appears well within this
	}
	genres := infoboxGenres(window)

	// Scan a copy for the image so the genre row's position is unaffected.
	imgWindow := window
	imgIdx := strings.Index(imgWindow, "<img ")
	for imgIdx >= 0 {
		src := extractHTMLAttr(imgWindow[imgIdx:], "src")
		if u := normalizeWikiImageURL(src); u != "" {
			return u, genres
		}
		imgWindow = imgWindow[imgIdx+5:]
		imgIdx = strings.Index(imgWindow, "<img ")
	}
	return "", genres
}

// infoboxGenres extracts the Genre/Genres row from an infobox table window.
func infoboxGenres(window string) []string {
	for _, label := range []string{">Genre<", ">Genres<", ">Genre ", ">Genres "} {
		idx := strings.Index(window, label)
		if idx < 0 {
			continue
		}
		rest := window[idx:]
		td := strings.Index(rest, "<td")
		if td < 0 {
			continue
		}
		cell := rest[td:]
		if end := strings.Index(cell, "</td>"); end >= 0 {
			cell = cell[:end]
		}
		if genres := parseGenreCell(cell); len(genres) > 0 {
			return genres
		}
	}
	return nil
}

// parseGenreCell pulls genre names from an infobox cell: anchor text first,
// then comma/semicolon separated plain text for unlinked values. Caps the list
// and drops footnote markers and over-long fragments.
func parseGenreCell(cell string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		v = stripFootnotes(html.UnescapeString(strings.TrimSpace(v)))
		v = strings.Trim(v, " \t\n\r,;·")
		if v == "" || len(v) > 40 {
			return
		}
		key := strings.ToLower(v)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, v)
	}

	rest := cell
	for {
		i := strings.Index(rest, "<a ")
		if i < 0 {
			break
		}
		gt := strings.Index(rest[i:], ">")
		if gt < 0 {
			break
		}
		closing := strings.Index(rest[i+gt:], "</a>")
		if closing < 0 {
			break
		}
		add(rest[i+gt+1 : i+gt+closing])
		rest = rest[i+gt+closing+4:]
	}
	// Unescape before splitting: HTML entities such as "&amp;" contain a
	// semicolon that would otherwise be read as a separator.
	plain := html.UnescapeString(stripTags(cell))
	for _, part := range strings.FieldsFunc(plain, func(r rune) bool {
		return r == ',' || r == ';' || r == '·'
	}) {
		add(part)
	}
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}

// stripTags removes markup, keeping text outside angle brackets.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripFootnotes removes bracketed footnote markers such as "[1]".
func stripFootnotes(s string) string {
	for {
		i := strings.Index(s, "[")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "]")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+j+1:]
	}
}

func extractHTMLAttr(fragment, attr string) string {
	idx := strings.Index(fragment, attr+`="`)
	if idx < 0 {
		return ""
	}
	rest := fragment[idx+len(attr)+2:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// normalizeWikiImageURL upgrades protocol-relative/thumb URLs to a direct
// https upload.wikimedia.org URL and strips tracking params.
func normalizeWikiImageURL(src string) string {
	if src == "" {
		return ""
	}
	src = strings.NewReplacer("&amp;", "&", "&#38;", "&").Replace(src)
	switch {
	case strings.HasPrefix(src, "//upload.wikimedia.org/"):
		src = wikiUploadBaseURL + strings.TrimPrefix(src, "//upload.wikimedia.org")
	case strings.Contains(src, "/wikipedia/") && !strings.HasPrefix(src, "http"):
		i := strings.Index(src, "/wikipedia/")
		src = wikiUploadBaseURL + src[i:]
	case strings.HasPrefix(src, "http://upload.wikimedia.org/"):
		src = wikiUploadBaseURL + strings.TrimPrefix(src, "http://upload.wikimedia.org")
	case strings.HasPrefix(src, "https://upload.wikimedia.org/"):
		src = wikiUploadBaseURL + strings.TrimPrefix(src, "https://upload.wikimedia.org")
	default:
		return "" // only accept Wikimedia-hosted images
	}
	if i := strings.Index(src, "?"); i >= 0 {
		src = src[:i]
	}
	return src
}

func debugStage(format string, args ...any) {
	if os.Getenv("SONDER_ENRICH_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "[wiki] "+format+"\n", args...)
	}
}

// Base URLs are vars so tests can point them at mock servers.
var (
	wikiBaseURL       = "https://en.wikipedia.org/w/api.php"
	wikiRESTBaseURL   = "https://en.wikipedia.org/api/rest_v1"
	wikiUploadBaseURL = "https://upload.wikimedia.org"
	audnexusBaseURL   = "https://api.audnex.us"
	openLibBaseURL    = "https://openlibrary.org"
	coversBaseURL     = "https://covers.openlibrary.org"
)

func (e *Enricher) audnexusLookup(ctx context.Context, in Input, cacheJSON, cachePoster string) (*Enrichment, error) {
	source := strings.ToLower(in.MetadataIDSource)
	if source != "audible" && source != "audnexus" {
		return nil, nil
	}
	asin := strings.TrimSpace(in.MetadataID)
	if asin == "" {
		return nil, nil
	}
	data, err := e.fetch(ctx, audnexusBaseURL+"/books/"+url.PathEscape(asin)+"?region=us")
	if err != nil {
		return nil, nil // provider miss is not an error for callers
	}
	var book audnexusBook
	if json.Unmarshal(data, &book) != nil {
		return nil, nil
	}
	if book.Image != "" {
		e.downloadTo(ctx, book.Image, cachePoster)
	}
	tags := make([]string, 0, len(book.Genres))
	genres := make([]string, 0, len(book.Genres))
	for _, g := range book.Genres {
		tags = append(tags, g.Name)
		genres = append(genres, g.Name)
	}
	payload := &Enrichment{
		Summary:   book.Description,
		Publisher: book.Publisher,
		Author:    firstNamed(book.Authors),
		Narrator:  firstNamed(book.Narrators),
		Tags:      tags,
		Genres:    genres,
		Provider:  "audnexus",
	}
	writeCache(cacheJSON, payload)
	return payload, nil
}

// --- Open Library (ebooks) ---

type openLibrarySearch struct {
	Docs []struct {
		Title            string   `json:"title"`
		AuthorNames      []string `json:"author_name"`
		FirstPublishYear int      `json:"first_publish_year"`
		CoverID          int      `json:"cover_i"`
		Subjects         []string `json:"subject"`
	} `json:"docs"`
}

func (e *Enricher) openLibraryLookup(ctx context.Context, in Input, cacheJSON, cachePoster string) (*Enrichment, error) {
	q := url.Values{}
	q.Set("title", in.Title)
	q.Set("limit", "5")
	q.Set("fields", "title,author_name,first_publish_year,cover_i,subject")
	data, err := e.fetch(ctx, openLibBaseURL+"/search.json?"+q.Encode())
	if err != nil {
		return nil, nil
	}
	var res openLibrarySearch
	if json.Unmarshal(data, &res) != nil {
		return nil, nil
	}
	var match *struct {
		Title            string   `json:"title"`
		AuthorNames      []string `json:"author_name"`
		FirstPublishYear int      `json:"first_publish_year"`
		CoverID          int      `json:"cover_i"`
		Subjects         []string `json:"subject"`
	}
	for i := range res.Docs {
		if normalizeBookTitle(res.Docs[i].Title) == normalizeBookTitle(in.Title) {
			match = &res.Docs[i]
			break
		}
	}
	if match == nil {
		return nil, nil
	}
	if match.CoverID != 0 {
		e.downloadTo(ctx,
			fmt.Sprintf(coversBaseURL+"/b/id/%d-L.jpg", match.CoverID),
			cachePoster)
	}
	var tags []string
	var genres []string
	for _, s := range match.Subjects {
		if len(tags) >= 8 {
			break
		}
		tags = append(tags, strings.ToLower(s))
		genres = append(genres, strings.ToLower(s))
	}
	for _, a := range match.AuthorNames {
		tags = append(tags, strings.ToLower(a))
	}
	tags = append(tags, "open-library")

	payload := &Enrichment{
		Summary:  "",
		Author:   firstString(match.AuthorNames),
		Tags:     tags,
		Genres:   genres,
		Provider: "open-library",
	}
	writeCache(cacheJSON, payload)
	return payload, nil
}

// normalizeBookTitle folds case/diacritics and strips non-alphanumerics.
// Diacritic folding uses a small Latin map to keep the project dependency-free.
func normalizeBookTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		r = foldDiacritic(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var diacriticFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ñ': 'n', 'ç': 'c', 'ý': 'y', 'ÿ': 'y', 'š': 's', 'ž': 'z', 'œ': 'o', 'æ': 'a',
	'ß': 's', 'đ': 'd', 'ł': 'l', 'ř': 'r', 'ť': 't', 'ĺ': 'l',
}

func foldDiacritic(r rune) rune {
	if f, ok := diacriticFold[r]; ok {
		return f
	}
	return r
}

func firstString(vals []string) string {
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// --- Wikipedia fallback (movies/documentaries/TV) ---

// wikipediaSearch finds the best-matching page via the search API, then
// pulls extract + poster art from the REST summary endpoint (the action API's
// pageimages prop hides non-free lead images like film posters; REST returns
// them). A title-containment guard rejects implausible matches so generic
// words don't grab unrelated covers.
func (e *Enricher) wikipediaSearch(ctx context.Context, query, itemTitle, cacheJSON, cachePoster, cacheBackdrop string) (*Enrichment, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("generator", "search")
	q.Set("gsrsearch", query)
	q.Set("gsrlimit", "1")
	q.Set("prop", "info")
	q.Set("format", "json")

	data, err := e.fetch(ctx, wikiBaseURL+"?"+q.Encode())
	if err != nil {
		return nil, nil
	}
	var found struct {
		Query struct {
			Pages map[string]struct {
				Title string `json:"title"`
			} `json:"pages"`
		} `json:"query"`
	}
	if json.Unmarshal(data, &found) != nil {
		return nil, nil
	}
	pageTitle := ""
	for _, p := range found.Query.Pages {
		pageTitle = p.Title
		break
	}
	if pageTitle == "" {
		debugStage("search: no page for %q", query)
		return nil, nil
	}
	debugStage("search matched %q", pageTitle)

	normItem := normalizeBookTitle(itemTitle)
	normPage := normalizeBookTitle(pageTitle)
	// Ultra-short or digit-only item titles ("01", "2019") are too generic
	// to trust with containment matching.
	if len(normItem) < 3 || !strings.ContainsFunc(normItem, unicode.IsLetter) {
		return nil, nil
	}
	if normItem != "" && normPage != "" &&
		!strings.Contains(normPage, normItem) && !strings.Contains(normItem, normPage) {
		debugStage("rejected implausible match %q for item %q", pageTitle, itemTitle)
		return nil, nil // implausible match: do not grab a wrong cover
	}

	sdata, err := e.fetch(ctx, wikiRESTBaseURL+"/page/summary/"+url.PathEscape(pageTitle))
	if err != nil {
		return nil, nil
	}
	var summary struct {
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Thumbnail   struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		OriginalImage struct {
			Source string `json:"source"`
		} `json:"originalimage"`
	}
	if json.Unmarshal(sdata, &summary) != nil {
		debugStage("REST summary parse failed")
		return nil, nil
	}
	debugStage("summary ok: extract=%d thumb=%v", len(summary.Extract), summary.Thumbnail.Source != "")

	// Poster preference: the article's infobox image scraped from the
	// rendered page (native resolution, exactly what Plex-style UIs want),
	// falling back to the REST summary's images.
	infobox, genres := e.wikiInfobox(ctx, pageTitle)
	if infobox != "" {
		summary.Thumbnail.Source = infobox
	}
	if summary.Thumbnail.Source != "" {
		e.downloadTo(ctx, summary.Thumbnail.Source, cachePoster)
	}
	if summary.OriginalImage.Source != "" && summary.OriginalImage.Source != summary.Thumbnail.Source {
		e.downloadTo(ctx, summary.OriginalImage.Source, cacheBackdrop)
	}
	payload := &Enrichment{
		Summary:   summary.Extract,
		Publisher: summary.Description,
		Tags:      []string{"wikipedia"},
		Genres:    genres,
		Provider:  "wikipedia",
	}
	writeCache(cacheJSON, payload)
	return payload, nil
}
