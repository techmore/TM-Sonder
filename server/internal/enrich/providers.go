package enrich

import (
	"context"
	"encoding/json"
	"fmt"
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

// wikiInfoboxImage fetches the rendered article page and extracts the first
// image inside the infobox table — for films that is the release poster at
// native resolution. Returns "" when the page has no usable image.
func (e *Enricher) wikiInfoboxImage(ctx context.Context, pageTitle string) string {
	pageURL := wikiBaseURL + "/wiki/" + url.PathEscape(strings.ReplaceAll(pageTitle, " ", "_"))
	data, err := e.fetch(ctx, pageURL)
	if err != nil {
		return ""
	}
	html := string(data)

	infobox := strings.Index(html, `<table class="infobox`)
	if infobox < 0 {
		infobox = strings.Index(html, `class="infobox`) // mobile/alternate markup
	}
	if infobox < 0 {
		return ""
	}
	window := html[infobox:]
	if len(window) > 24<<10 {
		window = window[:24<<10] // infobox images appear well within this
	}

	imgIdx := strings.Index(window, "<img ")
	for imgIdx >= 0 {
		src := extractHTMLAttr(window[imgIdx:], "src")
		if u := normalizeWikiImageURL(src); u != "" {
			return u
		}
		window = window[imgIdx+5:]
		imgIdx = strings.Index(window, "<img ")
	}
	return ""
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
	tags := make([]string, 0, len(book.Genres)+len(book.Authors)+len(book.Narrators))
	for _, g := range book.Genres {
		tags = append(tags, g.Name)
	}
	for _, a := range book.Authors {
		tags = append(tags, a.Name)
	}
	for _, n := range book.Narrators {
		tags = append(tags, n.Name)
	}
	payload := &Enrichment{
		Summary:   book.Description,
		Publisher: book.Publisher,
		Tags:      tags,
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
	for _, s := range match.Subjects {
		if len(tags) >= 8 {
			break
		}
		tags = append(tags, strings.ToLower(s))
	}
	for _, a := range match.AuthorNames {
		tags = append(tags, strings.ToLower(a))
	}
	tags = append(tags, "open-library")

	payload := &Enrichment{
		Summary:   "",
		Publisher: firstString(match.AuthorNames),
		Tags:      tags,
		Provider:  "open-library",
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
	if infobox := e.wikiInfoboxImage(ctx, pageTitle); infobox != "" {
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
		Provider:  "wikipedia",
	}
	writeCache(cacheJSON, payload)
	return payload, nil
}
