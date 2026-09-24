package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/url"
	"os"
	"sort"
	"strings"
	"unicode"
)

// MovieMetadata is the lazy, presentation-ready metadata used by the movie
// Rails detail panel. It is deliberately separate from MediaItem so a normal
// library load does not grow by the size of every film's cast and ratings.
type MovieMetadata struct {
	PageTitle   string            `json:"pageTitle,omitempty"`
	WikiURL     string            `json:"wikiURL,omitempty"`
	Primer      string            `json:"primer,omitempty"`
	Description string            `json:"description,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	Directors   []string          `json:"directors,omitempty"`
	Cast        []MovieCastMember `json:"cast,omitempty"`
	Ratings     []MovieRating     `json:"ratings,omitempty"`
	Provider    string            `json:"provider,omitempty"`
}

// MovieCastMember keeps the role and portrait together so clients can render
// a compact cast strip without making one request per person.
type MovieCastMember struct {
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	ImageURL  string `json:"imageURL,omitempty"`
}

type MovieRating struct {
	Source string `json:"source,omitempty"`
	Value  string `json:"value"`
	Method string `json:"method,omitempty"`
}

type wikipediaMovieSummary struct {
	Title        string `json:"title"`
	Extract      string `json:"extract"`
	Description  string `json:"description"`
	WikibaseItem string `json:"wikibase_item"`
}

type wikidataResponse struct {
	Entities map[string]wikidataEntity `json:"entities"`
}

type wikidataEntity struct {
	Labels map[string]wikidataLabel       `json:"labels"`
	Claims map[string][]wikidataStatement `json:"claims"`
}

type wikidataLabel struct {
	Value string `json:"value"`
}

type wikidataStatement struct {
	MainSnak   wikidataSnak              `json:"mainsnak"`
	Qualifiers map[string][]wikidataSnak `json:"qualifiers"`
}

type wikidataSnak struct {
	SnakType  string           `json:"snaktype"`
	DataValue *json.RawMessage `json:"datavalue"`
}

type wikidataEntityValue struct {
	ID string `json:"id"`
}

type wikidataDataValue struct {
	Value json.RawMessage `json:"value"`
}

type wikiInfoboxData struct {
	Poster    string
	Genres    []string
	Directors []string
	Cast      []MovieCastMember
}

// These variables are package vars so the metadata path can be tested without
// touching the public providers or making tests depend on the network.
var wikidataBaseURL = "https://www.wikidata.org/w/api.php"
var wikimediaFileBaseURL = "https://commons.wikimedia.org/wiki/Special:FilePath/"

// MovieMetadata resolves one movie on demand and caches the compact result on
// disk. A miss is intentionally returned as an empty metadata object: a film
// with only local metadata should remain completely usable.
func (e *Enricher) MovieMetadata(ctx context.Context, in Input) (*MovieMetadata, error) {
	if in.Kind != "movie" {
		return nil, nil
	}
	query := makeQuery(in)
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	cachePath := e.movieMetadataCachePath(query)
	if data, err := os.ReadFile(cachePath); err == nil {
		var cached MovieMetadata
		if json.Unmarshal(data, &cached) == nil {
			return &cached, nil
		}
	}

	pageTitle, summary, err := e.wikipediaMoviePage(ctx, query, in.Title)
	if err != nil || pageTitle == "" {
		return nil, err
	}
	infobox, _ := e.wikiInfoboxDetails(ctx, pageTitle)
	result := &MovieMetadata{
		PageTitle:   pageTitle,
		WikiURL:     "https://en.wikipedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(pageTitle, " ", "_")),
		Primer:      summary.Extract,
		Description: summary.Description,
		Genres:      append([]string(nil), infobox.Genres...),
		Directors:   append([]string(nil), infobox.Directors...),
		Cast:        append([]MovieCastMember(nil), infobox.Cast...),
		Provider:    "Wikipedia / Wikidata",
	}

	if summary.WikibaseItem != "" {
		e.applyWikidataMovie(ctx, summary.WikibaseItem, result)
	}
	if result.Primer == "" {
		result.Primer = in.Summary
	}
	if len(result.Genres) == 0 {
		result.Genres = nil
	}
	if len(result.Directors) == 0 {
		result.Directors = nil
	}
	writeCache(cachePath, result)
	return result, nil
}

func (e *Enricher) movieMetadataCachePath(query string) string {
	sum := sha256.Sum256([]byte("movie-meta-v1:" + query))
	return filepathJoin(e.CacheRoot, hex.EncodeToString(sum[:])+".movie.json")
}

// filepathJoin is kept tiny here so this file does not need to expose cache
// layout details to callers. It also makes an empty CacheRoot safe in tests.
func filepathJoin(root, name string) string {
	if strings.TrimSpace(root) == "" {
		return name
	}
	return strings.TrimRight(root, "/") + "/" + name
}

func (e *Enricher) wikipediaMoviePage(ctx context.Context, query, itemTitle string) (string, wikipediaMovieSummary, error) {
	q := url.Values{}
	q.Set("action", "query")
	q.Set("generator", "search")
	q.Set("gsrsearch", query)
	q.Set("gsrlimit", "1")
	q.Set("prop", "info")
	q.Set("format", "json")
	data, err := e.fetch(ctx, wikiBaseURL+"?"+q.Encode())
	if err != nil {
		return "", wikipediaMovieSummary{}, err
	}
	var found struct {
		Query struct {
			Pages map[string]struct {
				Title string `json:"title"`
			} `json:"pages"`
		} `json:"query"`
	}
	if json.Unmarshal(data, &found) != nil {
		return "", wikipediaMovieSummary{}, nil
	}
	pageTitle := ""
	for _, page := range found.Query.Pages {
		pageTitle = page.Title
		break
	}
	if pageTitle == "" {
		return "", wikipediaMovieSummary{}, nil
	}
	normItem := normalizeBookTitle(itemTitle)
	normPage := normalizeBookTitle(pageTitle)
	if len(normItem) < 3 || !strings.ContainsFunc(normItem, unicode.IsLetter) {
		return "", wikipediaMovieSummary{}, nil
	}
	if normItem != "" && normPage != "" &&
		!strings.Contains(normPage, normItem) && !strings.Contains(normItem, normPage) {
		return "", wikipediaMovieSummary{}, nil
	}

	sdata, err := e.fetch(ctx, wikiRESTBaseURL+"/page/summary/"+url.PathEscape(pageTitle))
	if err != nil {
		return pageTitle, wikipediaMovieSummary{}, nil
	}
	var summary wikipediaMovieSummary
	if json.Unmarshal(sdata, &summary) != nil {
		return pageTitle, wikipediaMovieSummary{}, nil
	}
	return pageTitle, summary, nil
}

func (e *Enricher) wikiInfoboxDetails(ctx context.Context, pageTitle string) (wikiInfoboxData, error) {
	base, err := url.Parse(wikiBaseURL)
	if err != nil {
		return wikiInfoboxData{}, err
	}
	base.Path, base.RawPath, base.RawQuery = "", "", ""
	pageURL := strings.TrimRight(base.String(), "/") + "/wiki/" + url.PathEscape(strings.ReplaceAll(pageTitle, " ", "_"))
	data, err := e.fetch(ctx, pageURL)
	if err != nil {
		return wikiInfoboxData{}, err
	}
	doc := string(data)
	infobox := strings.Index(doc, `<table class="infobox`)
	if infobox < 0 {
		infobox = strings.Index(doc, `class="infobox`)
	}
	if infobox < 0 {
		return wikiInfoboxData{}, nil
	}
	window := doc[infobox:]
	if end := strings.Index(window, "</table>"); end >= 0 {
		window = window[:end]
	}
	if len(window) > 24<<10 {
		window = window[:24<<10]
	}
	result := wikiInfoboxData{Genres: infoboxGenres(window)}
	if cell := infoboxRowCell(window, "directed by", "director"); cell != "" {
		result.Directors = parseWikiPeople(cell, 4)
	}
	if cell := infoboxRowCell(window, "starring", "cast"); cell != "" {
		result.Cast = parseWikiCast(cell, 10)
	}
	imgWindow := window
	for imgIdx := strings.Index(imgWindow, "<img "); imgIdx >= 0; {
		src := extractHTMLAttr(imgWindow[imgIdx:], "src")
		if u := normalizeWikiImageURL(src); u != "" {
			result.Poster = u
			break
		}
		imgWindow = imgWindow[imgIdx+5:]
		imgIdx = strings.Index(imgWindow, "<img ")
	}
	return result, nil
}

func infoboxRowCell(window string, labels ...string) string {
	lower := strings.ToLower(window)
	for offset := 0; offset < len(window); {
		th := strings.Index(lower[offset:], "<th")
		if th < 0 {
			break
		}
		th += offset
		gt := strings.Index(lower[th:], ">")
		if gt < 0 {
			break
		}
		gt += th
		end := strings.Index(lower[gt:], "</th>")
		if end < 0 {
			break
		}
		end += gt
		label := strings.TrimSpace(stripFootnotes(html.UnescapeString(stripTags(window[gt+1 : end]))))
		for _, want := range labels {
			if strings.EqualFold(label, want) {
				rest := window[end+len("</th>"):]
				restLower := strings.ToLower(rest)
				td := strings.Index(restLower, "<td")
				if td < 0 {
					return ""
				}
				rest = rest[td:]
				if cellEnd := strings.Index(strings.ToLower(rest), "</td>"); cellEnd >= 0 {
					return rest[:cellEnd]
				}
				return rest
			}
		}
		offset = end + len("</th>")
	}
	return ""
}

type wikiPersonLink struct {
	Title string
	Name  string
}

func parseWikiPeople(cell string, limit int) []string {
	links := parseWikiPersonLinks(cell, limit)
	out := make([]string, 0, len(links))
	for _, link := range links {
		out = append(out, link.Name)
	}
	return out
}

func parseWikiCast(cell string, limit int) []MovieCastMember {
	links := parseWikiPersonLinks(cell, limit)
	out := make([]MovieCastMember, 0, len(links))
	for _, link := range links {
		out = append(out, MovieCastMember{Name: link.Name})
	}
	return out
}

func parseWikiPersonLinks(cell string, limit int) []wikiPersonLink {
	var out []wikiPersonLink
	seen := map[string]bool{}
	for rest := cell; len(out) < limit; {
		i := strings.Index(strings.ToLower(rest), "<a ")
		if i < 0 {
			break
		}
		fragment := rest[i:]
		href := extractHTMLAttr(fragment, "href")
		nameStart := strings.Index(fragment, ">")
		if nameStart < 0 {
			break
		}
		nameEnd := strings.Index(strings.ToLower(fragment[nameStart:]), "</a>")
		if nameEnd < 0 {
			break
		}
		nameEnd += nameStart
		name := strings.TrimSpace(stripFootnotes(html.UnescapeString(stripTags(fragment[nameStart+1 : nameEnd]))))
		title := wikiTitleFromHref(href)
		if title != "" && name != "" && !seen[title] {
			seen[title] = true
			out = append(out, wikiPersonLink{Title: title, Name: name})
		}
		rest = fragment[nameEnd+len("</a>"):]
	}
	return out
}

func wikiTitleFromHref(href string) string {
	href = html.UnescapeString(strings.TrimSpace(href))
	for _, prefix := range []string{"https://en.wikipedia.org/wiki/", "http://en.wikipedia.org/wiki/", "/wiki/"} {
		if strings.HasPrefix(href, prefix) {
			title, err := url.PathUnescape(strings.TrimPrefix(href, prefix))
			if err != nil {
				return ""
			}
			title = strings.ReplaceAll(title, "_", " ")
			if strings.Contains(title, ":") {
				return ""
			}
			return title
		}
	}
	return ""
}

func (e *Enricher) applyWikidataMovie(ctx context.Context, movieID string, result *MovieMetadata) {
	entities, err := e.wikidataEntities(ctx, []string{movieID})
	if err != nil {
		return
	}
	movie, ok := entities[movieID]
	if !ok {
		return
	}
	castStatements := movie.Claims["P161"]
	directorStatements := movie.Claims["P57"]
	actorIDs := make([]string, 0, len(castStatements))
	characterIDs := make([]string, 0, len(castStatements))
	directorIDs := make([]string, 0, len(directorStatements))
	ratingSourceIDs := make([]string, 0, len(movie.Claims["P444"]))
	ratingMethodIDs := make([]string, 0, len(movie.Claims["P444"]))
	for _, statement := range castStatements {
		if id := entityID(statement.MainSnak); id != "" {
			actorIDs = appendUnique(actorIDs, id)
			if roles := statement.Qualifiers["P453"]; len(roles) > 0 {
				characterIDs = appendUnique(characterIDs, entityID(roles[0]))
			}
		}
	}
	for _, statement := range directorStatements {
		if id := entityID(statement.MainSnak); id != "" {
			directorIDs = appendUnique(directorIDs, id)
		}
	}
	for _, statement := range movie.Claims["P444"] {
		if sources := statement.Qualifiers["P447"]; len(sources) > 0 {
			ratingSourceIDs = appendUnique(ratingSourceIDs, entityID(sources[0]))
		}
		if methods := statement.Qualifiers["P459"]; len(methods) > 0 {
			ratingMethodIDs = appendUnique(ratingMethodIDs, entityID(methods[0]))
		}
	}
	allIDs := appendUnique(nil, actorIDs...)
	allIDs = appendUnique(allIDs, characterIDs...)
	allIDs = appendUnique(allIDs, directorIDs...)
	allIDs = appendUnique(allIDs, ratingSourceIDs...)
	allIDs = appendUnique(allIDs, ratingMethodIDs...)
	if len(allIDs) > 0 {
		if more, fetchErr := e.wikidataEntities(ctx, allIDs); fetchErr == nil {
			entities = more
		}
	}

	if len(result.Cast) == 0 {
		for _, actorID := range actorIDs {
			actor := entities[actorID]
			member := MovieCastMember{Name: entityLabel(actor, actorID)}
			for _, statement := range castStatements {
				if entityID(statement.MainSnak) != actorID {
					continue
				}
				if roles := statement.Qualifiers["P453"]; len(roles) > 0 {
					roleID := entityID(roles[0])
					member.Character = entityLabel(entities[roleID], roleID)
				}
				break
			}
			member.ImageURL = commonsImageURL(firstStringClaim(actor.Claims["P18"]))
			result.Cast = append(result.Cast, member)
		}
	} else {
		// Preserve the infobox's editorial order and fill its names with the
		// richer Wikidata roles and portraits when the links line up.
		for i := range result.Cast {
			for _, actorID := range actorIDs {
				actor := entities[actorID]
				if strings.EqualFold(result.Cast[i].Name, entityLabel(actor, actorID)) {
					result.Cast[i].Character = characterForActor(castStatements, actorID, entities)
					result.Cast[i].ImageURL = commonsImageURL(firstStringClaim(actor.Claims["P18"]))
					break
				}
			}
		}
	}

	if len(result.Directors) == 0 {
		for _, id := range directorIDs {
			result.Directors = append(result.Directors, entityLabel(entities[id], id))
		}
	}
	for _, statement := range movie.Claims["P444"] {
		value := stringValue(statement.MainSnak)
		if value == "" {
			continue
		}
		source := "Wikidata"
		if sources := statement.Qualifiers["P447"]; len(sources) > 0 {
			id := entityID(sources[0])
			source = entityLabel(entities[id], id)
		}
		method := ""
		if methods := statement.Qualifiers["P459"]; len(methods) > 0 {
			id := entityID(methods[0])
			method = entityLabel(entities[id], id)
		}
		result.Ratings = append(result.Ratings, MovieRating{Source: source, Value: value, Method: method})
		if len(result.Ratings) >= 4 {
			break
		}
	}
	if len(result.Ratings) > 1 {
		sort.SliceStable(result.Ratings, func(i, j int) bool { return result.Ratings[i].Source < result.Ratings[j].Source })
	}
}

func (e *Enricher) wikidataEntities(ctx context.Context, ids []string) (map[string]wikidataEntity, error) {
	if len(ids) == 0 {
		return map[string]wikidataEntity{}, nil
	}
	q := url.Values{}
	q.Set("action", "wbgetentities")
	q.Set("ids", strings.Join(ids, "|"))
	q.Set("props", "claims|labels")
	q.Set("languages", "en")
	q.Set("format", "json")
	data, err := e.fetch(ctx, wikidataBaseURL+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var response wikidataResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	return response.Entities, nil
}

func entityID(snak wikidataSnak) string {
	if snak.SnakType != "" && snak.SnakType != "value" || snak.DataValue == nil {
		return ""
	}
	var dataValue wikidataDataValue
	if json.Unmarshal(*snak.DataValue, &dataValue) != nil || len(dataValue.Value) == 0 {
		return ""
	}
	var value wikidataEntityValue
	if json.Unmarshal(dataValue.Value, &value) != nil {
		return ""
	}
	return value.ID
}

func stringValue(snak wikidataSnak) string {
	if snak.SnakType != "" && snak.SnakType != "value" || snak.DataValue == nil {
		return ""
	}
	var dataValue wikidataDataValue
	if json.Unmarshal(*snak.DataValue, &dataValue) != nil || len(dataValue.Value) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(dataValue.Value, &value) != nil {
		return ""
	}
	return value
}

func firstStringClaim(statements []wikidataStatement) string {
	if len(statements) == 0 {
		return ""
	}
	return stringValue(statements[0].MainSnak)
}

func entityLabel(entity wikidataEntity, fallback string) string {
	if label, ok := entity.Labels["en"]; ok && strings.TrimSpace(label.Value) != "" {
		return label.Value
	}
	return fallback
}

func characterForActor(statements []wikidataStatement, actorID string, entities map[string]wikidataEntity) string {
	for _, statement := range statements {
		if entityID(statement.MainSnak) != actorID {
			continue
		}
		if roles := statement.Qualifiers["P453"]; len(roles) > 0 {
			id := entityID(roles[0])
			return entityLabel(entities[id], id)
		}
	}
	return ""
}

func appendUnique(values []string, value ...string) []string {
	for _, candidate := range value {
		if candidate == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == candidate {
				found = true
				break
			}
		}
		if !found {
			values = append(values, candidate)
		}
	}
	return values
}

func commonsImageURL(filename string) string {
	if filename == "" {
		return ""
	}
	return strings.TrimRight(wikimediaFileBaseURL, "/") + "/" + url.PathEscape(filename) + "?width=160"
}
