package library

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"tm-sonder/server/internal/api"
)

// sonderNamespace is the fixed UUIDv5 namespace for stable item IDs derived
// from canonical file paths.
var sonderNamespace = [16]byte{
	0x74, 0x6d, 0x73, 0x6f, 0x6e, 0x64, 0x65, 0x72,
	0x73, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x69, 0x64,
}

// StableID derives the deterministic item ID for a canonical path.
func StableID(canonicalPath string) string {
	return uuidV5(sonderNamespace, canonicalPath)
}

// uuidV5 computes an RFC 4122 version-5 (SHA-1) UUID.
func uuidV5(ns [16]byte, name string) string {
	h := sha1.New()
	h.Write(ns[:])
	h.Write([]byte(name))
	var u [16]byte
	copy(u[:], h.Sum(nil)[:16])
	u[6] = (u[6] & 0x0f) | 0x50
	u[8] = (u[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// ParserVersion bumps whenever parser output semantics change. The scanner
// treats items stamped with an older version as changed on the next scan
// (IDs are path-derived, so progress and item identity survive the rebuild),
// which propagates parsing fixes to already-cataloged libraries without a
// full wipe.
const ParserVersion = 5

// Parsed is the ported result of SonderMediaParser.parseTitle. Kind is chosen
// by the scanner from library config + extension, not by the parser.
type Parsed struct {
	Title            string
	Subtitle         string
	Year             int
	ShowTitle        string
	Season           *int
	Episode          *int
	MetadataIDSource string
	MetadataID       string
	Edition          string
	SplitPart        string
	Series           string // ebook/audiobook series or author grouping
}

var (
	reTVEpisodeShow = regexp.MustCompile(`(?i)^(.+?)[\s._-]+S(\d{1,2})[\s._-]*E(\d{1,2})[\s._-]*(.*)$`)
	reTVBare        = regexp.MustCompile(`(?i)^S(\d{1,2})[\s._-]*E(\d{1,2})[\s._-]*(.*)$`)
	reTVCross       = regexp.MustCompile(`(?i)^(.+?)[\s._-]+(\d{1,2})x(\d{1,2})[\s._-]*(.*)$`)
	// Parenthesised code right after the show name: "Cheers (S08E20) Title".
	reTVParen = regexp.MustCompile(`(?i)^(.+?)\s*\(\s*S(\d{1,2})[\s._-]*E(\d{1,2})\s*\)\s*(.*)$`)

	reAbsEpisodeShow = regexp.MustCompile(`(?i)^(.+?)[\s._-]+(?:episode|ep)?[\s._-]*(\d{1,3})(?:v\d+)?(?:[\s._-]+(.+))?$`)
	reAbsEpisodeBare = regexp.MustCompile(`(?i)^(?:episode|ep)?[\s._-]*(\d{1,3})(?:v\d+)?(?:[\s._-]+(.+))?$`)

	reLeadingTag    = regexp.MustCompile(`^\[[^\]]+\][\s._-]*`)
	reQualityBrack  = regexp.MustCompile(`(?i)\[[^\]]*(?:720|1080|2160|x264|x265|h\.264|h\.265|hevc|aac|flac|bd|bluray|web|webrip|dvd|dual audio)[^\]]*\]`)
	reQualityParen  = regexp.MustCompile(`(?i)\([^)]*(?:720|1080|2160|x264|x265|h\.264|h\.265|hevc|aac|flac|bd|bluray|web|webrip|dvd|dual audio)[^)]*\]`)
	reHash8         = regexp.MustCompile(`(?i)\[[A-F0-9]{8}\]`)
	reYearParen     = regexp.MustCompile(`\(\d{4}\)`)
	reYearBare      = regexp.MustCompile(`\b\d{4}\b`)
	reYearAny       = regexp.MustCompile(`(19|20)\d{2}`)
	reMetadataTag   = regexp.MustCompile(`(?i)\{(imdb|tmdb|audible|audnexus)-([^}]+)\}`)
	reEditionTag    = regexp.MustCompile(`(?i)\{edition-([^}]{1,32})\}`)
	reSplitSuffix   = regexp.MustCompile(`(?i)(?:^|[\s._-])(cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$`)
	reTrailingSplit = regexp.MustCompile(`(?i)[\s._-]+(?:cd\d+|disc\d+|disk\d+|dvd\d+|part\d+|pt\d+)$`)

	// Quality tail: from the first resolution token (1080p/720p/480p...) to
	// the end — e.g. " Show S04 E18 Extended 1080p Bluray AAC" -> " Show S04 E18 Extended".
	reTrailingQuality = regexp.MustCompile(`(?i)[\s._-]*\b(?:480p|576p|720p|1080p|2160p|4k)\b[\s\S]*$`)
	// Release chain: resolution token followed by MORE junk tokens before the
	// string ends ("1080p Bluray AAC 5.1 x265-GRP") — a release chain, not a
	// lone quality suffix. Matched where used in cleanEpisodeTitle.
	reQualityChain = regexp.MustCompile(`(?i)[\s._-]*\b(?:480p|576p|720p|1080p|2160p|4k)\b(?:[\s._-]+\S+){1,}[\s\S]*$`)
	// Episode code + everything after it inside a show-title candidate:
	// "Battlestar Galactica (2003) S02 E13 1080p Bluray AAC" -> "Battlestar Galactica (2003)".
	reEpisodeCodeTail = regexp.MustCompile(`(?i)[\s._-]+S\d{1,2}[\s._]*E\d{1,2}[\s\S]*$`)
	reSeasonFolder    = regexp.MustCompile(`(?i)(?:season[\s._-]*(\d{1,2}))|(?:^s(\d{1,2})$)|(?:^series[\s._-]*(\d{1,2})$)`)
	reSpecialsOnly    = regexp.MustCompile(`(?i)^(specials|season[\s._-]*0|s0{1,2})$`)
)

func cleanMediaTitle(s string) string {
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

func removePlexTags(s string) string {
	s = reMetadataTag.ReplaceAllString(s, "")
	return reEditionTag.ReplaceAllString(s, "")
}

func removeSplitSuffix(s string) string {
	return reTrailingSplit.ReplaceAllString(s, "")
}

func cleanShowTitle(s string) string {
	s = reLeadingTag.ReplaceAllString(s, "")
	s = removePlexTags(s)
	// Per-episode folder names leak into show titles ("Show S04 E18
	// Extended 1080p Bluray AAC"): drop the episode code and any quality
	// tail after it.
	s = reEpisodeCodeTail.ReplaceAllString(s, "")
	s = reTrailingQuality.ReplaceAllString(s, "")
	return cleanMediaTitle(s)
}

func cleanEpisodeTitle(s string) string {
	s = reQualityBrack.ReplaceAllString(s, "")
	s = reQualityParen.ReplaceAllString(s, "")
	s = reHash8.ReplaceAllString(s, "")
	s = removePlexTags(s)
	// Bare trailing release chains ("1080p Bluray AAC 5.1 x265-GRP") are not
	// a real episode name; drop from the first resolution token on. A lone
	// quality suffix ("One Minute 1080p") is kept for Swift parity.
	s = reQualityChain.ReplaceAllString(s, "")
	s = cleanMediaTitle(s)
	return strings.Trim(s, "- ")
}

func extractYear(s string) int {
	m := reYearAny.FindString(s)
	if m == "" {
		return 0
	}
	y, _ := strconv.Atoi(m)
	return y
}

// movieNameAndYear splits "Title (2019)" or "Title 2019" into title + year.
func movieNameAndYear(s string) (string, int, bool) {
	year := extractYear(s)
	if year == 0 {
		return "", 0, false
	}
	title := reYearParen.ReplaceAllString(s, "")
	title = reYearBare.ReplaceAllString(title, "")
	title = removePlexTags(title)
	title = removeSplitSuffix(title)
	title = cleanMediaTitle(title)
	if title == "" {
		title = cleanMediaTitle(s)
	}
	return title, year, true
}

func extractMetadataTag(s string) (source, id string) {
	if m := reMetadataTag.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1]), m[2]
	}
	return "", ""
}

func extractEdition(s string) string {
	if m := reEditionTag.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func extractSplitPart(s string) string {
	if m := reSplitSuffix.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func extractSeasonFolder(s string) (int, bool) {
	norm := strings.TrimSpace(s)
	if reSpecialsOnly.MatchString(norm) {
		return 0, true
	}
	if m := reSeasonFolder.FindStringSubmatch(norm); m != nil {
		for _, g := range m[1:] {
			if g != "" {
				n, _ := strconv.Atoi(g)
				return n, true
			}
		}
	}
	return 0, false
}

func tvShowName(parent, grandparent string) string {
	if strings.Contains(strings.ToLower(parent), "season") {
		return grandparent
	}
	return parent
}

func intPtr(n int) *int { return &n }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func episodeCode(season, episode int) string {
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

// tvMatch describes one SxxEyy filename pattern and how to read its groups.
type tvMatch struct {
	re      *regexp.Regexp
	extract func(m []string) (show, season, episode, title string)
}

var tvPatterns = []tvMatch{
	{reTVEpisodeShow, func(m []string) (string, string, string, string) {
		return m[1], m[2], m[3], m[4]
	}},
	{reTVBare, func(m []string) (string, string, string, string) {
		return "", m[1], m[2], m[3]
	}},
	{reTVParen, func(m []string) (string, string, string, string) {
		return m[1], m[2], m[3], m[4]
	}},
	{reTVCross, func(m []string) (string, string, string, string) {
		return m[1], m[2], m[3], m[4]
	}},
}

// parseAbsoluteEpisode ports the tvShows-library absolute numbering fallback.
func parseAbsoluteEpisode(raw, folderShow string, season int) (*Parsed, bool) {
	normalized := cleanMediaTitle(reLeadingTag.ReplaceAllString(raw, ""))
	type cand struct {
		showGroup bool
		re        *regexp.Regexp
	}
	for _, c := range []cand{{true, reAbsEpisodeShow}, {false, reAbsEpisodeBare}} {
		m := c.re.FindStringSubmatch(normalized)
		if m == nil {
			continue
		}
		var show, epStr, title string
		if c.showGroup {
			show, epStr, title = m[1], m[2], m[3]
		} else {
			epStr, title = m[1], m[2]
		}
		ep, err := strconv.Atoi(epStr)
		if err != nil || ep < 1 || ep > 200 {
			continue
		}
		p := &Parsed{
			Title:     cleanEpisodeTitle(title),
			ShowTitle: firstNonEmpty(cleanShowTitle(show), folderShow),
			Season:    intPtr(season),
			Episode:   &ep,
		}
		return p, true
	}
	return nil, false
}

// ParseFilename ports SonderMediaParser for one media file path.
// libraryKind: movie|tvShow|documentary|audiobook|ebook|all.
func ParseFilename(path, libraryKind string) Parsed {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	raw := cleanMediaTitle(base)
	parent := cleanMediaTitle(filepath.Base(filepath.Dir(path)))
	grandparent := cleanMediaTitle(filepath.Base(filepath.Dir(filepath.Dir(path))))

	metadataSource, metadataID := extractMetadataTag(raw)
	if metadataSource == "" {
		metadataSource, metadataID = extractMetadataTag(parent)
	}
	edition := extractEdition(raw)
	if edition == "" {
		edition = extractEdition(parent)
	}
	splitPart := extractSplitPart(raw)

	tvKind := libraryKind == "tvShow"
	docKind := libraryKind == "documentary"

	// 1. SxxEyy patterns apply regardless of library kind.
	folderShow := tvShowName(parent, grandparent)
	for _, pat := range tvPatterns {
		m := pat.re.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		showStr, seasonStr, episodeStr, titleStr := pat.extract(m)
		season, err1 := strconv.Atoi(seasonStr)
		episode, err2 := strconv.Atoi(episodeStr)
		if err1 != nil || err2 != nil {
			continue
		}
		parsedShow := ""
		if showStr != "" {
			parsedShow = cleanShowTitle(showStr)
		}
		show := firstNonEmpty(parsedShow, folderShow)
		if docKind {
			show = folderShow
		}
		epTitle := cleanEpisodeTitle(titleStr)
		if epTitle == "" {
			epTitle = "Episode " + strconv.Itoa(episode)
		}
		return Parsed{
			Title:            epTitle,
			Subtitle:         show + " - " + episodeCode(season, episode),
			Year:             firstNonZero(extractYear(raw), extractYear(show)),
			ShowTitle:        show,
			Season:           intPtr(season),
			Episode:          intPtr(episode),
			MetadataIDSource: metadataSource,
			MetadataID:       metadataID,
			Edition:          edition,
			SplitPart:        splitPart,
		}
	}

	// 2-3. tvShows-only absolute episodes and season folders.
	if tvKind {
		season, hasSeason := extractSeasonFolder(parent)
		if abs, ok := parseAbsoluteEpisode(raw, folderShow, defaultSeason(season, hasSeason)); ok {
			abs.MetadataIDSource, abs.MetadataID = metadataSource, metadataID
			abs.Edition, abs.SplitPart = edition, splitPart
			abs.Year = firstNonZero(extractYear(raw), extractYear(abs.ShowTitle))
			if abs.Title == "" {
				abs.Title = "Episode " + strconv.Itoa(*abs.Episode)
			}
			abs.Subtitle = abs.ShowTitle + " - " + episodeCode(*abs.Season, *abs.Episode)
			return *abs
		}
		p := Parsed{
			Title:            raw,
			Subtitle:         folderShow,
			Year:             firstNonZero(extractYear(raw), extractYear(folderShow)),
			ShowTitle:        folderShow,
			MetadataIDSource: metadataSource,
			MetadataID:       metadataID,
			Edition:          edition,
			SplitPart:        splitPart,
		}
		if hasSeason {
			p.Season = intPtr(season)
			p.Subtitle = fmt.Sprintf("%s - Season %d", folderShow, season)
		}
		return p
	}

	// 4+. Kind-specific simple resolution.
	fileTitle, fileYear, fileHasYear := movieNameAndYear(raw)
	switch libraryKind {
	case "movie":
		if title, year, ok := movieNameAndYear(parent); ok {
			return simpleParsed(title, "Movie - "+strconv.Itoa(year), year, metadataSource, metadataID, edition, splitPart)
		}
		if fileHasYear {
			return simpleParsed(fileTitle, "Movie - "+strconv.Itoa(fileYear), fileYear, metadataSource, metadataID, edition, splitPart)
		}
	case "documentary":
		sub := "Documentary"
		year := fileYear
		if _, y, ok := movieNameAndYear(parent); ok {
			year = y
		}
		if year > 0 {
			sub = "Documentary - " + strconv.Itoa(year)
		}
		t := fileTitle
		if t == "" {
			t = raw
		}
		return simpleParsed(t, sub, year, metadataSource, metadataID, edition, splitPart)
	case "audiobook":
		t := fileTitle
		if t == "" {
			t = removePlexTags(raw)
		}
		sub := "Audiobook"
		if y := extractYear(raw); y > 0 {
			sub = "Audiobook - " + strconv.Itoa(y)
		}
		return simpleParsed(t, sub, extractYear(raw), metadataSource, metadataID, edition, splitPart)
	case "ebook":
		t := fileTitle
		if t == "" {
			t = removePlexTags(raw)
		}
		// Ebook filenames commonly embed the author ("Title (Author)",
		// "Author - Title", "Title - Author"). Split it off into Studio
		// so clients can group by author; parent dir is the fallback.
		author := ""
		t, author = splitEbookAuthor(t)
		if author == "" {
			pa := removePlexTags(parent)
			if pa != "" && !looksLikeJunkDir(pa) {
				author = pa
			}
		}
		sub := "Book"
		if y := extractYear(raw); y > 0 {
			sub = "Book - " + strconv.Itoa(y)
		}
		p := simpleParsed(t, sub, extractYear(raw), metadataSource, metadataID, edition, splitPart)
		if author != "" {
			p.Series = author
		}
		return p
	}

	// Fallback: unknown/"all" libraries infer intent from the name itself.
	lower := strings.ToLower(base)
	switch {
	case strings.Contains(lower, "audiobook"):
		t := fileTitle
		if t == "" {
			t = removePlexTags(raw)
		}
		return simpleParsed(t, "Audiobook", extractYear(raw), metadataSource, metadataID, edition, splitPart)
	case api.FormatForExtension(strings.TrimPrefix(filepath.Ext(path), ".")) == api.FormatEPUB ||
		api.FormatForExtension(strings.TrimPrefix(filepath.Ext(path), ".")) == api.FormatPDF ||
		strings.Contains(lower, "book"):
		t := fileTitle
		if t == "" {
			t = removePlexTags(raw)
		}
		return simpleParsed(t, "Book", extractYear(raw), metadataSource, metadataID, edition, splitPart)
	default:
		t := fileTitle
		if t == "" {
			t = raw
		}
		sub := "Imported local media"
		if fileHasYear {
			sub = "Movie - " + strconv.Itoa(fileYear)
		}
		return simpleParsed(t, sub, fileYear, metadataSource, metadataID, edition, splitPart)
	}
}

func defaultSeason(season int, has bool) int {
	if has {
		return season
	}
	return 1
}

func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// splitEbookAuthor splits common ebook "Author - Title" / "Title (Author)"
// filename shapes. Returns (title, author); author is "" when no confident
// split exists.
func splitEbookAuthor(s string) (string, string) {
	s = strings.TrimSpace(s)
	// "Title (Author Name)" trailing parenthetical with name-ish content.
	if m := reEbookParenAuthor.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return s, ""
}

var reEbookParenAuthor = regexp.MustCompile(`^(.{2,}?)\s*\(([^)(]{2,60})\)$`)

// looksLikeJunkDir reports whether a directory name is unusable as an author
// fallback (collection piles, format tags, etc.).
func looksLikeJunkDir(name string) bool {
	n := strings.ToLower(name)
	for _, junk := range []string{"ebook", "ebooks", "books", "book", "calibre",
		"library", "mybooks", "download", "downloads", "converted", "unknown"} {
		if n == junk {
			return true
		}
	}
	if strings.Contains(n, "collection") || strings.Contains(n, "novels") {
		return true
	}
	return false
}

func simpleParsed(title, subtitle string, year int, src, id, edition, splitPart string) Parsed {
	return Parsed{
		Title:            title,
		Subtitle:         subtitle,
		Year:             year,
		MetadataIDSource: src,
		MetadataID:       id,
		Edition:          edition,
		SplitPart:        splitPart,
	}
}
