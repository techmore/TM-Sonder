package library

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"tm-sonder/server/internal/api"
)

func apiKindMovie() api.MediaKind  { return api.KindMovie }
func apiKindTVShow() api.MediaKind { return api.KindTVShow }

func formatForPath(path string) api.MediaFormat {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	return api.FormatForExtension(ext)
}

func apiMediaItem(id, title, subtitle string, kind api.MediaKind, year int,
	summary, studio string, format api.MediaFormat) api.MediaItem {
	return api.MediaItem{
		ID: id, Title: title, Subtitle: subtitle, Kind: kind, Year: year,
		Summary: summary, Studio: studio, Format: format, Tags: []string{},
	}
}

// Plex import ports SonderPlexImport.swift's database reader. Decision per
// roadmap: shell out to the bundled sqlite3 binary instead of taking on the
// project's first third-party dependency.

const plexSQLiteCandidates = "/usr/bin/sqlite3:/usr/local/bin/sqlite3:/opt/homebrew/bin/sqlite3"

type PlexImportResult struct {
	Movies   int
	Shows    int
	Episodes int
	Skipped  int // rows whose media file is missing on disk
}

func findSQLite3() (string, error) {
	for _, p := range strings.Split(plexSQLiteCandidates, ":") {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	if p, err := exec.LookPath("sqlite3"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("library: sqlite3 not found (needed for Plex import)")
}

// runSQLRows executes one query returning JSON rows (sqlite3 -json). JSON is
// immune to delimiter collisions in titles/paths that break -list mode.
func runSQLRows(sqlitePath, dbPath, sql string) ([]map[string]any, error) {
	cmd := exec.Command(sqlitePath, "-json", dbPath, sql)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("library: plex query failed: %w", err)
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("library: plex query parse: %w", err)
	}
	return rows, nil
}

func rowStr(row map[string]any, key string) string {
	if v, ok := row[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func rowNum(row map[string]any, key string, fallback int) int {
	if v, ok := row[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case string:
			if i, err := strconv.Atoi(n); err == nil {
				return i
			}
		}
	}
	return fallback
}

const plexMoviesSQL = `select
  m.id as id,
  coalesce(m.title, '') as title,
  coalesce(m.summary, '') as summary,
  coalesce(m.studio, '') as studio,
  coalesce(m.year, 0) as year,
  coalesce(m.tags_genre, '') as genres,
  coalesce(m.guid, '') as guid,
  coalesce(group_concat(distinct p.file), '') as files
from metadata_items m
left join media_items mi on mi.metadata_item_id = m.id
left join media_parts p on p.media_item_id = mi.id and p.deleted_at is null
where m.metadata_type = 1 and m.deleted_at is null
group by m.id
order by m.refreshed_at desc
limit %d;`

const plexEpisodesSQL = `select
  coalesce(show.title, '') as show_title,
  coalesce(s."index", 1) as season,
  coalesce(e."index", 1) as episode,
  coalesce(e.title, '') as title,
  coalesce(e.summary, '') as summary,
  coalesce(e.guid, '') as guid,
  coalesce(group_concat(distinct p.file), '') as files
from metadata_items e
join metadata_items s on s.id = e.parent_id and s.deleted_at is null
join metadata_items show on show.id = s.parent_id and show.deleted_at is null
left join media_items mi on mi.metadata_item_id = e.id
left join media_parts p on p.media_item_id = mi.id and p.deleted_at is null
where e.metadata_type = 4 and e.deleted_at is null
group by e.id
order by show.title, s."index", e."index"
limit %d;`

// plexGUID splits agent-style guids like
// "com.plexapp.agents.imdb://tt0111161?lang=en" into source + id.
func plexGUID(guid string) (source, id string) {
	if guid == "" {
		return "", ""
	}
	schemeIdx := strings.Index(guid, "://")
	if schemeIdx < 0 {
		return "", ""
	}
	source = strings.ToLower(strings.TrimPrefix(guid[:schemeIdx], "com.plexapp.agents."))
	id = guid[schemeIdx+3:]
	if q := strings.Index(id, "?"); q >= 0 {
		id = id[:q]
	}
	switch source {
	case "imdb", "tmdb", "themoviedb":
		if source == "themoviedb" {
			source = "tmdb"
		}
		return source, id
	default:
		return "", ""
	}
}

func plexFirstFile(joined string) string {
	for _, f := range strings.Split(joined, ",") {
		f = strings.TrimSpace(f)
		f = strings.TrimPrefix(f, "file://")
		if f == "" {
			continue
		}
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			return f
		}
	}
	return ""
}

// ImportPlexLibrary reads a Plex sqlite database and upserts movies and TV
// episodes into the store. Items get StableIDs derived from their file paths
// so subsequent filesystem scans reconcile instead of duplicating.
func ImportPlexLibrary(s *Store, dbPath string, limit int) (*PlexImportResult, error) {
	sqlitePath, err := findSQLite3()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10000
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("library: plex db: %w", err)
	}

	res := &PlexImportResult{}
	var items []*Item

	movieRows, err := runSQLRows(sqlitePath, dbPath, fmt.Sprintf(plexMoviesSQL, limit))
	if err != nil {
		return nil, err
	}
	for _, row := range movieRows {
		path := plexFirstFile(rowStr(row, "files"))
		if path == "" {
			res.Skipped++
			continue
		}
		src, id := plexGUID(rowStr(row, "guid"))
		title, _, _ := movieNameAndYear(cleanMediaTitle(rowStr(row, "title")))
		items = append(items, plexItem(path, apiKindMovie(), title, "",
			rowNum(row, "year", 0), rowStr(row, "summary"), rowStr(row, "studio"),
			rowStr(row, "genres"), src, id))
		res.Movies++
	}

	epRows, err := runSQLRows(sqlitePath, dbPath, fmt.Sprintf(plexEpisodesSQL, limit))
	if err != nil {
		return nil, err
	}
	shows := map[string]bool{}
	for _, row := range epRows {
		path := plexFirstFile(rowStr(row, "files"))
		if path == "" {
			res.Skipped++
			continue
		}
		showTitle := rowStr(row, "show_title")
		season := rowNum(row, "season", 1)
		episode := rowNum(row, "episode", 1)
		subtitle := fmt.Sprintf("%s - S%02dE%02d", showTitle, season, episode)
		it := plexItem(path, apiKindTVShow(), rowStr(row, "title"), subtitle, 0,
			rowStr(row, "summary"), "", "", "", "")
		it.ShowTitle = &showTitle
		it.SeasonNumber = intPtr(season)
		it.EpisodeNumber = intPtr(episode)
		items = append(items, it)
		if !shows[showTitle] {
			shows[showTitle] = true
			res.Shows++
		}
		res.Episodes++
	}

	s.Upsert(items...)
	s.RecordActivity("Plex import",
		fmt.Sprintf("imported %d movie(s), %d episode(s) (%d skipped)",
			res.Movies, res.Episodes, res.Skipped), "shippingbox")
	return res, nil
}

func plexItem(path string, kind api.MediaKind, title, subtitle string, year int,
	summary, studio, genres, metaSource, metaID string) *Item {
	format := formatForPath(path)
	it := &Item{
		MediaItem: apiMediaItem(
			StableID(path), title, subtitle, kind, year, summary, studio, format),
		FilePath: path,
	}
	if st, err := os.Stat(path); err == nil {
		it.SizeBytes = st.Size()
		it.ModTime = st.ModTime()
	}
	for _, g := range strings.Split(genres, "|") {
		if g = strings.TrimSpace(g); g != "" {
			it.Tags = append(it.Tags, g)
		}
	}
	if metaSource != "" && metaID != "" {
		it.MetadataIDSource = &metaSource
		it.MetadataID = &metaID
	}
	return it
}
