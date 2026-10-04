package httpapi

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

type insightCounts struct {
	Titles             int   `json:"titles"`
	Files              int   `json:"files"`
	Bytes              int64 `json:"bytes"`
	UnknownSizeFiles   int   `json:"unknownSizeFiles"`
	MissingGenreTitles int   `json:"missingGenreTitles"`
}
type insightLibrary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	insightCounts
}
type insightKind struct {
	Kind api.MediaKind `json:"kind"`
	insightCounts
}
type insightTitle struct {
	Name      string          `json:"name,omitempty"`
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Kind      api.MediaKind   `json:"kind"`
	LibraryID string          `json:"libraryID"`
	Files     int             `json:"files"`
	Bytes     int64           `json:"bytes"`
	Format    api.MediaFormat `json:"format"`
}
type insightGenre struct {
	Name   string `json:"name"`
	Titles int    `json:"titles"`
}
type insightAccess struct {
	insightTitle
	Sessions      int       `json:"sessions"`
	ActiveSeconds float64   `json:"activeSeconds"`
	MediaSeconds  float64   `json:"mediaSeconds"`
	LastActivity  time.Time `json:"lastActivity"`
}
type insightActivity struct {
	Label         string          `json:"label"`
	Note          string          `json:"note"`
	Sessions      int             `json:"sessions"`
	ActiveSeconds float64         `json:"activeSeconds"`
	MediaSeconds  float64         `json:"mediaSeconds"`
	MostAccessed  []insightAccess `json:"mostAccessed"`
}
type libraryInsights struct {
	TotalGenreCount int                       `json:"totalGenreCount"`
	GeneratedAt     time.Time                 `json:"generatedAt"`
	LibraryID       string                    `json:"libraryID"`
	Kind            string                    `json:"kind"`
	Totals          insightCounts             `json:"totals"`
	Libraries       []insightLibrary          `json:"libraries"`
	Kinds           []insightKind             `json:"kinds"`
	LargestTitles   []insightTitle            `json:"largestTitles"`
	LargestFiles    []insightTitle            `json:"largestFiles"`
	Genres          []insightGenre            `json:"genres"`
	Activity        insightActivity           `json:"activity"`
	CatalogAccess   library.ReadMetricsReport `json:"catalogAccess"`
}

func (s *Server) handleLibraryInsights(w http.ResponseWriter, r *http.Request) {
	libraryID, kind := strings.TrimSpace(r.URL.Query().Get("libraryID")), strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind == "all" {
		kind = ""
	}
	if kind != "" && (!config.ValidKind(kind) || kind == "plex") {
		writeError(w, http.StatusBadRequest, "Invalid media kind")
		return
	}
	libraries := s.cfg().Libraries
	found := libraryID == ""
	for _, configured := range libraries {
		if configured.ID == libraryID {
			found = true
		}
	}
	if !found {
		writeError(w, http.StatusBadRequest, "Unknown library")
		return
	}
	_, activity, scoped, err := s.accountActivityForRequest(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Account activity is unavailable")
		return
	}
	reading := api.ReadingState{}
	if scoped {
		reading = activity.Reading
	} else if s.accountActivityUsername(r) == "" && (s.accounts == nil || !s.accounts.HasAccount()) {
		reading = s.store.ReadingState()
	}
	report := buildLibraryInsights(s.store.InternalItems(), libraries, libraryID, kind, reading)
	report.CatalogAccess = s.store.ReadMetrics()
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, report)
}

func buildLibraryInsights(items []*library.Item, configured []config.Library, libraryID, kind string, reading api.ReadingState) libraryInsights {
	report := libraryInsights{GeneratedAt: time.Now().UTC(), LibraryID: libraryID, Kind: kind, Libraries: []insightLibrary{}, Kinds: []insightKind{}, LargestTitles: []insightTitle{}, LargestFiles: []insightTitle{}, Genres: []insightGenre{}, Activity: insightActivity{Label: "Your recorded listening & reading", Note: "Saved playback sessions only; stream and range requests are not counted. Activity predating session recording is unavailable.", MostAccessed: []insightAccess{}}}
	libIndexes := map[string]int{}
	for _, lib := range configured {
		libIndexes[lib.ID] = len(report.Libraries)
		report.Libraries = append(report.Libraries, insightLibrary{ID: lib.ID, Name: lib.Name, Kind: lib.Kind})
	}
	selected := []*library.Item{}
	for _, item := range items {
		if item.LibraryID == nil || item.FilePath == "" {
			continue
		}
		if _, ok := libIndexes[*item.LibraryID]; !ok {
			continue
		}
		if libraryID != "" && *item.LibraryID != libraryID || kind != "" && string(item.Kind) != kind {
			continue
		}
		selected = append(selected, item)
	}
	// Group libraries separately: identical relative book folders in distinct
	// libraries must never merge their files or activity.
	groups := map[string]library.BookGrouping{}
	byLibrary := map[string][]*library.Item{}
	for _, item := range selected {
		byLibrary[*item.LibraryID] = append(byLibrary[*item.LibraryID], item)
	}
	for _, libItems := range byLibrary {
		derived, _ := library.BookGroups(libItems, configured)
		for id, group := range derived {
			groups[id] = group
		}
	}
	titles := map[string]*insightTitle{}
	genreSets := map[string]map[string]string{}
	itemKeys := map[string]string{}
	kinds := map[api.MediaKind]*insightCounts{}
	for _, item := range selected {
		key := item.ID
		title := item.Title
		if group, ok := groups[item.ID]; ok {
			key = group.ID
			title = item.Title
		}
		itemKeys[item.ID] = key
		entry, exists := titles[key]
		if !exists {
			entry = &insightTitle{ID: item.ID, Title: title, Kind: item.Kind, LibraryID: *item.LibraryID, Format: item.Format}
			titles[key] = entry
			genreSets[key] = map[string]string{}
		}
		if group, ok := groups[item.ID]; ok && group.Index == 1 {
			entry.ID = item.ID
			entry.Title = item.Title
		}
		entry.Files++
		size := item.SizeBytes
		if size < 0 {
			size = 0
		}
		entry.Bytes += size
		if entry.Format != item.Format {
			entry.Format = api.FormatUnknown
		}
		file := insightTitle{Name: filepath.Base(item.FilePath), ID: item.ID, Title: item.Title, Kind: item.Kind, LibraryID: *item.LibraryID, Files: 1, Bytes: size, Format: item.Format}
		report.LargestFiles = append(report.LargestFiles, file)
		if kinds[item.Kind] == nil {
			kinds[item.Kind] = &insightCounts{}
		}
		for _, counts := range []*insightCounts{&report.Totals, &report.Libraries[libIndexes[*item.LibraryID]].insightCounts, kinds[item.Kind]} {
			counts.Files++
			counts.Bytes += size
			if size == 0 {
				counts.UnknownSizeFiles++
			}
		}
		for _, genre := range item.Genres {
			genre = strings.TrimSpace(genre)
			if genre != "" {
				normalized := strings.ToLower(genre)
				if previous, ok := genreSets[key][normalized]; !ok || genre < previous {
					genreSets[key][normalized] = genre
				}
			}
		}
	}
	genres := map[string]insightGenre{}
	for key, title := range titles {
		report.LargestTitles = append(report.LargestTitles, *title)
		for _, counts := range []*insightCounts{&report.Totals, &report.Libraries[libIndexes[title.LibraryID]].insightCounts, kinds[title.Kind]} {
			counts.Titles++
			if len(genreSets[key]) == 0 {
				counts.MissingGenreTitles++
			}
		}
		for normalized, name := range genreSets[key] {
			value := genres[normalized]
			if value.Name == "" || name < value.Name {
				value.Name = name
			}
			value.Titles++
			genres[normalized] = value
		}
	}
	for k, counts := range kinds {
		report.Kinds = append(report.Kinds, insightKind{Kind: k, insightCounts: *counts})
	}
	for _, genre := range genres {
		report.Genres = append(report.Genres, genre)
	}
	sort.Slice(report.Kinds, func(i, j int) bool { return report.Kinds[i].Kind < report.Kinds[j].Kind })
	sort.Slice(report.Genres, func(i, j int) bool {
		if report.Genres[i].Titles != report.Genres[j].Titles {
			return report.Genres[i].Titles > report.Genres[j].Titles
		}
		return report.Genres[i].Name < report.Genres[j].Name
	})
	report.TotalGenreCount = len(report.Genres)
	if len(report.Genres) > 20 {
		report.Genres = report.Genres[:20]
	}
	for _, list := range []*[]insightTitle{&report.LargestTitles, &report.LargestFiles} {
		sort.Slice(*list, func(i, j int) bool {
			a, b := (*list)[i], (*list)[j]
			if a.Bytes != b.Bytes {
				return a.Bytes > b.Bytes
			}
			return a.ID < b.ID
		})
		if len(*list) > 10 {
			*list = (*list)[:10]
		}
	}
	// A repeated checkpoint is cumulative, and a session may span book parts.
	// Count each session once per logical title and keep its greatest counters.
	sessions := map[string]map[string]api.ReadingSession{}
	for _, record := range reading.Records {
		key, ok := itemKeys[record.ItemID]
		if !ok {
			continue
		}
		if sessions[key] == nil {
			sessions[key] = map[string]api.ReadingSession{}
		}
		for _, run := range record.Reads {
			for _, session := range run.Sessions {
				if session.ID == "" {
					continue
				}
				prior := sessions[key][session.ID]
				if session.ActiveSeconds > prior.ActiveSeconds {
					prior.ActiveSeconds = session.ActiveSeconds
				}
				if session.MediaSeconds > prior.MediaSeconds {
					prior.MediaSeconds = session.MediaSeconds
				}
				if session.UpdatedAt.After(prior.UpdatedAt) {
					prior.UpdatedAt = session.UpdatedAt
				}
				sessions[key][session.ID] = prior
			}
		}
	}
	for key, recorded := range sessions {
		entry := insightAccess{insightTitle: *titles[key]}
		for _, session := range recorded {
			entry.Sessions++
			entry.ActiveSeconds += session.ActiveSeconds
			entry.MediaSeconds += session.MediaSeconds
			if session.UpdatedAt.After(entry.LastActivity) {
				entry.LastActivity = session.UpdatedAt
			}
		}
		if entry.Sessions == 0 {
			continue
		}
		report.Activity.Sessions += entry.Sessions
		report.Activity.ActiveSeconds += entry.ActiveSeconds
		report.Activity.MediaSeconds += entry.MediaSeconds
		report.Activity.MostAccessed = append(report.Activity.MostAccessed, entry)
	}
	sort.Slice(report.Activity.MostAccessed, func(i, j int) bool {
		a, b := report.Activity.MostAccessed[i], report.Activity.MostAccessed[j]
		if a.Sessions != b.Sessions {
			return a.Sessions > b.Sessions
		}
		if !a.LastActivity.Equal(b.LastActivity) {
			return a.LastActivity.After(b.LastActivity)
		}
		return a.ID < b.ID
	})
	if len(report.Activity.MostAccessed) > 10 {
		report.Activity.MostAccessed = report.Activity.MostAccessed[:10]
	}
	return report
}
