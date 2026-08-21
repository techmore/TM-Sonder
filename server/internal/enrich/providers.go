package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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

// Base URLs are vars so tests can point them at mock servers.
var (
	wikiBaseURL     = "https://en.wikipedia.org/w/api.php"
	audnexusBaseURL = "https://api.audnex.us"
	openLibBaseURL  = "https://openlibrary.org"
	coversBaseURL   = "https://covers.openlibrary.org"
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

type wikiResponse struct {
	Query struct {
		Pages map[string]struct {
			Extract     string `json:"extract"`
			Description string `json:"description"`
			Categories  []struct {
				Title string `json:"title"`
			} `json:"categories"`
			Thumbnail struct {
				Source string `json:"source"`
			} `json:"thumbnail"`
			OriginalImage struct {
				Source string `json:"source"`
			} `json:"originalimage"`
		} `json:"pages"`
	} `json:"query"`
}

func (e *Enricher) wikipediaSearch(ctx context.Context, query, cacheJSON, cachePoster, cacheBackdrop string) (*Enrichment, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("generator", "search")
	q.Set("gsrsearch", query)
	q.Set("gsrlimit", "1")
	q.Set("prop", "extracts|pageimages|info|categories")
	q.Set("exintro", "1")
	q.Set("explaintext", "1")
	q.Set("inprop", "url")
	q.Set("piprop", "thumbnail|original")
	q.Set("pithumbsize", "800")
	q.Set("cllimit", "10")
	q.Set("format", "json")
	q.Set("origin", "*")

	data, err := e.fetch(ctx, wikiBaseURL+"?"+q.Encode())
	if err != nil {
		return nil, nil
	}
	var decoded wikiResponse
	if json.Unmarshal(data, &decoded) != nil {
		return nil, nil
	}
	var page *struct {
		Extract     string `json:"extract"`
		Description string `json:"description"`
		Categories  []struct {
			Title string `json:"title"`
		} `json:"categories"`
		Thumbnail struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		OriginalImage struct {
			Source string `json:"source"`
		} `json:"originalimage"`
	}
	for i := range decoded.Query.Pages {
		p := decoded.Query.Pages[i]
		page = &p
		break
	}
	if page == nil {
		return nil, nil
	}
	if page.Thumbnail.Source != "" {
		e.downloadTo(ctx, page.Thumbnail.Source, cachePoster)
	}
	if page.OriginalImage.Source != "" {
		e.downloadTo(ctx, page.OriginalImage.Source, cacheBackdrop)
	}
	var tags []string
	for _, c := range page.Categories {
		if len(tags) >= 6 {
			break
		}
		parts := strings.SplitN(c.Title, ":", 2)
		tag := parts[len(parts)-1]
		tags = append(tags, strings.ToLower(tag))
	}
	payload := &Enrichment{
		Summary:   page.Extract,
		Publisher: page.Description,
		Tags:      tags,
	}
	writeCache(cacheJSON, payload)
	return payload, nil
}
