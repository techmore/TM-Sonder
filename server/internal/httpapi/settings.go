package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/enrich"
	"tm-sonder/server/internal/library"
)

// SettingsPayload is the GET/PUT shape for /api/settings. The pairing token
// is only included when the request comes from loopback.
type SettingsPayload struct {
	Version         int            `json:"version"`
	Port            int            `json:"port"`
	DataDir         string         `json:"dataDir"`
	AllowLAN        bool           `json:"allowLAN"`
	TokenConfigured bool           `json:"tokenConfigured"`
	PairingToken    *string        `json:"pairingToken,omitempty"`
	RequiresPairing bool           `json:"requiresPairing"`
	ThemePreset     string         `json:"themePreset"`
	HWAccel         string         `json:"hwaccel"`
	MaxConcurrent   int            `json:"maxConcurrent"`
	Libraries       []LibraryEntry `json:"libraries"`
	SuggestedMounts []string       `json:"suggestedMounts"`
}

type LibraryEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	ItemCount int    `json:"itemCount"`
}

func (s *Server) settingsPayload(includeToken bool) SettingsPayload {
	counts := map[string]int{}
	for _, it := range s.store.InternalItems() {
		if it.LibraryID != nil {
			counts[*it.LibraryID]++
		}
	}
	libs := make([]LibraryEntry, 0, len(s.cfg.Libraries))
	for _, l := range s.cfg.Libraries {
		libs = append(libs, LibraryEntry{
			ID: l.ID, Name: l.Name, Path: l.Path, Kind: l.Kind,
			ItemCount: counts[l.ID],
		})
	}
	p := SettingsPayload{
		Version:         int(atomic.LoadInt64(&s.settingsVersion)),
		Port:            s.cfg.Port,
		DataDir:         s.cfg.DataDir,
		AllowLAN:        s.cfg.AllowLAN,
		TokenConfigured: s.cfg.PairingToken != "",
		RequiresPairing: s.requiresPairing(),
		ThemePreset:     s.cfg.ThemePreset,
		HWAccel:         s.cfg.Transcode.HWAccel,
		MaxConcurrent:   s.cfg.Transcode.MaxConcurrent,
		Libraries:       libs,
		SuggestedMounts: suggestedMounts(),
	}
	if includeToken && s.cfg.PairingToken != "" {
		t := s.cfg.PairingToken
		p.PairingToken = &t
	}
	return p
}

// suggestedMounts lists mounted external volumes — NAS shares mount under
// /Volumes on macOS; /mnt covers common Linux container layouts.
func suggestedMounts() []string {
	var out []string
	for _, root := range []string{"/Volumes", "/mnt"} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "Macintosh HD" || name == "home" {
				continue
			}
			p := filepath.Join(root, name)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				out = append(out, p)
			}
		}
	}
	return out
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsPayload(isLoopback(peerHost(r))))
}

type settingsUpdate struct {
	AllowLAN    *bool           `json:"allowLAN"`
	ThemePreset *string         `json:"themePreset"`
	Libraries   *[]LibraryEntry `json:"libraries"`
}

// handleSettingsPut implements PUT/PATCH /api/settings: applies validated
// changes to the running config, persists the config file, and rescans when
// the library table changed.
func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var upd settingsUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&upd); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid settings payload")
		return
	}

	rescanNeeded := false
	if upd.Libraries != nil {
		// Preserve stable library identities: a re-submitted library pointing
		// at the same path keeps its previous ID so already-scanned items
		// stay attached.
		prevByID := map[string]config.Library{}
		for _, l := range s.cfg.Libraries {
			prevByID[l.Path] = l
		}
		for i := range *upd.Libraries {
			if (*upd.Libraries)[i].ID == "" {
				if prev, ok := prevByID[filepath.Clean(strings.TrimSpace((*upd.Libraries)[i].Path))]; ok {
					(*upd.Libraries)[i].ID = prev.ID
				}
			}
		}
		libs, err := sanitizeLibraries(*upd.Libraries)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.cfg.Libraries = libs
		s.store.SetDirectories(directoriesFromLibraries(libs))
		rescanNeeded = true
	}
	if upd.AllowLAN != nil {
		s.cfg.AllowLAN = *upd.AllowLAN
		// Mirror startup policy: enabling LAN auto-provisions a token.
		if s.cfg.AllowLAN && s.cfg.PairingToken == "" {
			token := api.NewID() + api.NewID()
			s.cfg.PairingToken = token
			s.persistPairingToken(token)
		}
	}
	if upd.ThemePreset != nil && strings.TrimSpace(*upd.ThemePreset) != "" {
		s.cfg.ThemePreset = strings.TrimSpace(*upd.ThemePreset)
	}

	if err := s.persistConfig(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save config: "+err.Error())
		return
	}
	atomic.AddInt64(&s.settingsVersion, 1)

	if rescanNeeded {
		go func() {
			if _, err := s.scanner.ScanAll(s.cfg.Libraries); err == nil && s.snapshotPath != "" {
				_ = s.store.Flush(s.snapshotPath)
			}
		}()
	}
	writeJSON(w, http.StatusOK, s.settingsPayload(isLoopback(peerHost(r))))
}

type browseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type browsePayload struct {
	Path    string        `json:"path"`
	Parent  *string       `json:"parent"`
	Entries []browseEntry `json:"entries"`
}

// handleSettingsBrowse implements GET /api/settings/browse?path=... — a
// directory navigator for the add-library flow. Without a path it offers
// starting points (home, /Volumes for NAS mounts, filesystem root).
func (s *Server) handleSettingsBrowse(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("path"))
	if q == "" {
		out := browsePayload{Path: "", Entries: []browseEntry{}}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			out.Entries = append(out.Entries, browseEntry{Name: "Home", Path: home})
		}
		if st, err := os.Stat("/Volumes"); err == nil && st.IsDir() {
			out.Entries = append(out.Entries, browseEntry{Name: "Volumes (NAS mounts)", Path: "/Volumes"})
		}
		out.Entries = append(out.Entries, browseEntry{Name: "Filesystem root", Path: "/"})
		writeJSON(w, http.StatusOK, out)
		return
	}

	path := filepath.Clean(q)
	st, err := os.Stat(path)
	if err != nil || !st.IsDir() {
		writeError(w, http.StatusBadRequest, "Not a readable directory: "+path)
		return
	}

	parent := filepath.Dir(path)
	entries := []browseEntry{}
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, http.StatusForbidden, "Cannot read directory: "+err.Error())
		return
	}
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() {
			continue
		}
		entries = append(entries, browseEntry{
			Name: name,
			Path: filepath.Join(path, name),
		})
	}

	out := browsePayload{Path: path, Entries: entries}
	if parent != path {
		p := parent
		out.Parent = &p
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSettingsRescan(w http.ResponseWriter, r *http.Request) {
	state := s.scanner.State()
	if state.Scanning {
		writeError(w, http.StatusConflict, "Scan already in progress")
		return
	}
	go func() {
		if _, err := s.scanner.ScanAll(s.cfg.Libraries); err == nil && s.snapshotPath != "" {
			_ = s.store.Flush(s.snapshotPath)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

// handleSettingsEnrich implements POST /api/settings/enrich: runs the
// metadata pass against the live store (no second process needed).
func (s *Server) handleSettingsEnrich(w http.ResponseWriter, r *http.Request) {
	if !s.enriching.CompareAndSwap(false, true) {
		writeError(w, http.StatusConflict, "Enrichment already running")
		return
	}
	go func() {
		defer s.enriching.Store(false)
		logger := log.New(log.Writer(), "sonder-enrich ", log.LstdFlags)
		RunEnrichmentPass(logger, s.store, filepath.Join(s.cfg.DataDir, "metadata-cache"))
		if s.snapshotPath != "" {
			_ = s.store.Flush(s.snapshotPath)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

// sanitizeLibraries validates and normalizes a submitted library table.
func sanitizeLibraries(in []LibraryEntry) ([]config.Library, error) {
	out := make([]config.Library, 0, len(in))
	seenPath := map[string]bool{}
	validKinds := map[string]bool{"movie": true, "tvShow": true, "documentary": true,
		"audiobook": true, "ebook": true, "all": true, "plex": true}
	for i, l := range in {
		path := filepath.Clean(strings.TrimSpace(l.Path))
		name := strings.TrimSpace(l.Name)
		kind := strings.TrimSpace(strings.ToLower(l.Kind))
		if path == "" || path == "/" {
			return nil, fmt.Errorf("libraries[%d]: path required", i)
		}
		if st, err := os.Stat(path); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("libraries[%d] (%q): path is not a readable directory", i, name)
		}
		if !validKinds[kind] {
			return nil, fmt.Errorf("libraries[%d] (%q): kind must be movie|tvShow|documentary|audiobook|ebook|all", i, name)
		}
		if seenPath[path] {
			continue // duplicate paths collapse silently
		}
		seenPath[path] = true
		id := strings.TrimSpace(l.ID)
		if id == "" {
			id = api.NewID()
		}
		if name == "" {
			name = filepath.Base(path)
		}
		out = append(out, config.Library{ID: id, Name: name, Path: path, Kind: kind})
	}
	// Expand "plex" meta-libraries into per-kind child libraries. Duplicate
	// paths (e.g. an existing explicit library plus a plex root containing
	// the same folder) collapse via the same seenPath set.
	return config.ExpandPlexLibraries(out)
}

// directoriesFromLibraries maps the library table to MediaDirectory entries
// shown in /api/library.
func directoriesFromLibraries(libs []config.Library) []api.MediaDirectory {
	out := make([]api.MediaDirectory, 0, len(libs))
	for _, l := range libs {
		out = append(out, api.MediaDirectory{
			ID: l.ID, Name: l.Name, Kind: l.Kind, LibraryID: l.ID,
		})
	}
	return out
}

// RunEnrichmentPass fetches official metadata for items missing a summary or
// still carrying only generated thumbnails. Official posters override frame
// grabs; locally discovered artwork is never replaced. Returns updates count.
// runEnrichmentPass fetches official metadata for items that are missing a
// summary or still carry only a generated thumbnail. Official provider
// posters override generated thumbnails; locally discovered artwork is never
// replaced. Returns how many items were updated.
func RunEnrichmentPass(logger *log.Logger, store *library.Store, cacheRoot string) int {
	enricher := enrich.New(cacheRoot)
	const workers = 3

	type job struct{ item *library.Item }
	var candidates []*library.Item
	for _, it := range store.InternalItems() {
		needsSummary := it.Summary == ""
		needsOfficialPoster := it.PosterSource == "" || it.PosterSource == "thumbnail"
		if needsSummary || needsOfficialPoster {
			candidates = append(candidates, it)
		}
	}
	logger.Printf("enrichment pass: %d candidate item(s)", len(candidates))

	in := make(chan job)
	var wg sync.WaitGroup
	var updated atomic.Int64
	done := 0
	var progressMu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				it := j.item
				in2 := enrich.Input{
					Title:            it.Title,
					Kind:             string(it.Kind),
					Year:             it.Year,
					Studio:           it.Studio,
					Edition:          derefStr(it.Edition),
					MetadataIDSource: derefStr(it.MetadataIDSource),
					MetadataID:       derefStr(it.MetadataID),
				}
				if it.ShowTitle != nil {
					in2.ShowTitle = *it.ShowTitle
				}
				if it.SeasonNumber != nil && it.EpisodeNumber != nil {
					in2.Season, in2.Episode = *it.SeasonNumber, *it.EpisodeNumber
				}

				result, err := enricher.Enrich(context.Background(), in2)
				if os.Getenv("SONDER_ENRICH_DEBUG") == "1" &&
					(done < 3 || strings.Contains(strings.ToLower(in2.Title), "matrix") ||
						strings.Contains(strings.ToLower(in2.Title), "inception")) {
					why := ""
					switch {
					case err != nil:
						why = err.Error()
					case result == nil:
						why = "result nil"
					default:
						why = fmt.Sprintf("sum=%d tags=%d poster=%v provider=%s",
							len(result.Summary), len(result.Tags), result.PosterPath != "", result.Provider)
					}
					fmt.Fprintf(os.Stderr, "[pass] %q (%s): %s\n", in2.Title, in2.Kind, why)
				}
				if err != nil || result == nil ||
					(result.Summary == "" && len(result.Tags) == 0 && result.PosterPath == "") {
					progressMu.Lock()
					done++
					if done%25 == 0 {
						logger.Printf("enrichment: %d/%d processed", done, len(candidates))
					}
					progressMu.Unlock()
					continue
				}

				fresh, ok := store.Get(it.ID)
				if ok {
					changed := false
					if result.Summary != "" && fresh.Summary == "" {
						fresh.Summary = result.Summary
						changed = true
					}
					if result.Author != "" && fresh.Author == nil {
						a := result.Author
						fresh.Author = &a
						changed = true
					}
					if result.Narrator != "" && fresh.Narrator == nil {
						n := result.Narrator
						fresh.Narrator = &n
						changed = true
					}
					for _, tag := range result.Tags {
						dup := false
						for _, existing := range fresh.Tags {
							if strings.EqualFold(existing, tag) {
								dup = true
								break
							}
						}
						if !dup {
							fresh.Tags = append(fresh.Tags, tag)
							changed = true
						}
					}
					// Official poster beats generated thumbnails; local
					// discovered artwork is never replaced. Art is installed
					// Plex-style into the media folder when possible so other
					// tools see it; the enricher cache copy is the fallback.
					if result.PosterPath != "" && fresh.PosterSource != "local" {
						installed := result.PosterPath
						posterDest, _ := library.PlexArtPaths(fresh)
						if dest, err := library.CopyArtTo(result.PosterPath, posterDest); err == nil {
							installed = dest
						}
						if fresh.PosterPath != installed || fresh.PosterSource != result.Provider {
							fresh.PosterPath = installed
							u := "/artwork/poster/" + fresh.ID
							fresh.PosterURL = &u
							fresh.PosterSource = result.Provider
							changed = true
						}
					}
					if result.BackdropPath != "" && fresh.BackdropPath == "" {
						_, fanartDest := library.PlexArtPaths(fresh)
						installed := result.BackdropPath
						if dest, err := library.CopyArtTo(result.BackdropPath, fanartDest); err == nil {
							installed = dest
						}
						fresh.BackdropPath = installed
						u := "/artwork/backdrop/" + fresh.ID
						fresh.BackdropURL = &u
						changed = true
					}
					if changed {
						store.Upsert(fresh)
						updated.Add(1)
					}
				}

				progressMu.Lock()
				done++
				if done%25 == 0 {
					logger.Printf("enrichment: %d/%d processed (%d updated)",
						done, len(candidates), updated.Load())
				}
				progressMu.Unlock()
			}
		}()
	}
	for _, it := range candidates {
		in <- job{item: it}
	}
	close(in)
	wg.Wait()
	return int(updated.Load())
}
