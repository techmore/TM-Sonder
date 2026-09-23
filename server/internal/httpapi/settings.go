package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/enrich"
)

// SettingsPayload is the GET/PUT shape for /api/settings. The pairing token
// is only included when the request comes from loopback.
type SettingsPayload struct {
	Version         int            `json:"version"`
	Port            int            `json:"port"`
	WebPort         int            `json:"webPort,omitempty"`
	APIPort         int            `json:"apiPort,omitempty"`
	DataDir         string         `json:"dataDir"`
	AllowLAN        bool           `json:"allowLAN"`
	TokenConfigured bool           `json:"tokenConfigured"`
	PairingToken    *string        `json:"pairingToken,omitempty"`
	RequiresPairing bool           `json:"requiresPairing"`
	ThemePreset     string         `json:"themePreset"`
	AudiobookLayout string         `json:"audiobookLayout"`
	MoviesLayout    string         `json:"moviesLayout"`
	TVLayout        string         `json:"tvLayout"`
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
	webPort, apiPort := s.activePorts()
	counts := s.store.CountByLibrary()
	libs := make([]LibraryEntry, 0, len(s.cfg().Libraries))
	for _, l := range s.cfg().Libraries {
		libs = append(libs, LibraryEntry{
			ID: l.ID, Name: l.Name, Path: l.Path, Kind: l.Kind,
			ItemCount: counts[l.ID],
		})
	}
	p := SettingsPayload{
		Version:         int(atomic.LoadInt64(&s.settingsVersion)),
		Port:            webPort,
		WebPort:         webPort,
		APIPort:         apiPort,
		DataDir:         s.cfg().DataDir,
		AllowLAN:        s.cfg().AllowLAN,
		TokenConfigured: s.cfg().PairingToken != "",
		RequiresPairing: s.requiresPairing(),
		ThemePreset:     s.cfg().ThemePreset,
		AudiobookLayout: config.NormalizeAudiobookLayout(s.cfg().AudiobookLayout),
		MoviesLayout:    config.NormalizeMediaLayout(s.cfg().MoviesLayout),
		TVLayout:        config.NormalizeMediaLayout(s.cfg().TVLayout),
		HWAccel:         s.cfg().Transcode.HWAccel,
		MaxConcurrent:   s.cfg().Transcode.MaxConcurrent,
		Libraries:       libs,
		SuggestedMounts: suggestedMounts(),
	}
	if includeToken && s.cfg().PairingToken != "" {
		t := s.cfg().PairingToken
		p.PairingToken = &t
	}
	return p
}

// mountsCacheTTL bounds how stale the mount list may be. Enumerating /Volumes
// and /mnt can block for seconds on a stale network mount, so it must not run
// on every settings request.
const mountsCacheTTL = 30 * time.Second

var (
	mountsMu     sync.Mutex
	mountsCache  []string
	mountsCached time.Time
)

// suggestedMounts lists mounted external volumes — NAS shares mount under
// /Volumes on macOS; /mnt covers common Linux container layouts. The listing is
// cached briefly so a hung mount cannot stall the settings endpoint.
func suggestedMounts() []string {
	mountsMu.Lock()
	defer mountsMu.Unlock()
	if mountsCache != nil && time.Since(mountsCached) < mountsCacheTTL {
		return append([]string(nil), mountsCache...)
	}
	out := listMounts()
	mountsCache, mountsCached = out, time.Now()
	return append([]string(nil), out...)
}

func listMounts() []string {
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
	AllowLAN        *bool           `json:"allowLAN"`
	ThemePreset     *string         `json:"themePreset"`
	AudiobookLayout *string         `json:"audiobookLayout"`
	MoviesLayout    *string         `json:"moviesLayout"`
	TVLayout        *string         `json:"tvLayout"`
	Libraries       *[]LibraryEntry `json:"libraries"`
}

// handleSettingsPut implements PUT/PATCH /api/settings: applies validated
// changes to the running config, persists the config file, and rescans when
// the library table changed.
func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var upd settingsUpdate
	if err := jsonDecode(w, r, &upd); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid settings payload")
		return
	}

	current := s.cfg()
	rescanNeeded := false
	nextLibs := current.Libraries
	if upd.Libraries != nil {
		// Guard against an accidental empty library table wiping the catalog:
		// require an explicit acknowledgement when libraries already exist.
		if len(*upd.Libraries) == 0 && len(current.Libraries) > 0 &&
			r.URL.Query().Get("confirm") != "empty-libraries" {
			writeError(w, http.StatusConflict,
				"Refusing to clear all libraries; resend with ?confirm=empty-libraries")
			return
		}
		// Preserve stable library identities: a re-submitted library pointing
		// at the same path keeps its previous ID so already-scanned items
		// stay attached.
		prevByID := map[string]config.Library{}
		for _, l := range current.Libraries {
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
		nextLibs = libs
		rescanNeeded = true
	}

	newToken := ""
	updated := s.updateConfig(func(next *config.Config) {
		if upd.Libraries != nil {
			next.Libraries = nextLibs
		}
		if upd.AllowLAN != nil {
			next.AllowLAN = *upd.AllowLAN
			// Mirror startup policy: enabling LAN auto-provisions a token.
			if next.AllowLAN && next.PairingToken == "" {
				next.PairingToken = api.NewID() + api.NewID()
				newToken = next.PairingToken
			}
		}
		if upd.ThemePreset != nil && strings.TrimSpace(*upd.ThemePreset) != "" {
			next.ThemePreset = strings.TrimSpace(*upd.ThemePreset)
		}
		if upd.AudiobookLayout != nil {
			next.AudiobookLayout = config.NormalizeAudiobookLayout(*upd.AudiobookLayout)
		}
		if upd.MoviesLayout != nil {
			next.MoviesLayout = config.NormalizeMediaLayout(*upd.MoviesLayout)
		}
		if upd.TVLayout != nil {
			next.TVLayout = config.NormalizeMediaLayout(*upd.TVLayout)
		}
	})
	if upd.Libraries != nil {
		s.store.SetDirectories(directoriesFromLibraries(updated.Libraries))
		if s.audiobookOptimizer != nil {
			s.audiobookOptimizer.SetLibraries(updated.Libraries)
		}
	}
	if newToken != "" {
		s.persistPairingToken(newToken)
	}

	if err := s.persistConfig(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save config: "+err.Error())
		return
	}
	atomic.AddInt64(&s.settingsVersion, 1)
	// Theme and server settings are embedded in /api/library. Invalidate the
	// memoized payload so clients do not reapply a stale theme on refresh.
	s.libMu.Lock()
	s.libJSON = nil
	s.libJSONGzip = nil
	s.libMu.Unlock()

	if rescanNeeded {
		libs := append([]config.Library(nil), updated.Libraries...)
		go func() {
			if _, err := s.scanner.ScanAll(libs); err == nil && s.snapshotPath != "" {
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
		if _, err := s.scanner.ScanAll(s.cfg().Libraries); err == nil && s.snapshotPath != "" {
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
		enrich.RunPass(context.Background(), logger, s.store,
			filepath.Join(s.cfg().DataDir, "metadata-cache"))
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
		if !config.ValidKind(kind) {
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
