package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

// themeFor returns the wire theme snapshot for a preset name. The earthy
// palette mirrors SonderThemePalette.earthy (8-digit RRGGBBAA hex).
func themeFor(preset string) api.ThemeSnapshot {
	if preset == "" {
		preset = "earthy"
	}
	switch preset {
	case "earthy":
		return api.ThemeSnapshot{
			Preset:     preset,
			Background: "#B0C4B138",
			Sidebar:    "#B0C4B157",
			Surface:    "#F7E1D7EB",
			Border:     "#B0C4B1E0",
			Accent:     "#4A5759",
			Text:       "#4A5759",
		}
	case "techmore":
		return api.ThemeSnapshot{
			Preset:     preset,
			Background: "#B5C8A3",
			Sidebar:    "#E4E8D9",
			Surface:    "#DDE5CF",
			Border:     "#A7B891",
			Accent:     "#526C3F",
			Text:       "#403D36",
		}
	case "bunny":
		return api.ThemeSnapshot{
			Preset:     preset,
			Background: "#080910CC",
			Sidebar:    "#191C31E6",
			Surface:    "#101223F2",
			Border:     "#2A2F4D",
			Accent:     "#6FE3C3",
			Text:       "#EEF0FF",
		}
	default:
		return api.ThemeSnapshot{
			Preset:     preset,
			Background: "#1C1C1E",
			Sidebar:    "#2C2C2E",
			Surface:    "#3A3A3C",
			Border:     "#48484A",
			Accent:     "#0A84FF",
			Text:       "#FFFFFF",
		}
	}
}

func (s *Server) serverSettings() api.ServerSettings {
	webPort, apiPort := s.activePorts()
	return api.ServerSettings{
		IsEnabled:          true,
		AllowLAN:           s.cfg().AllowLAN,
		Port:               webPort,
		WebPort:            webPort,
		APIPort:            apiPort,
		Version:            Version,
		ThemePreset:        s.cfg().ThemePreset,
		LibraryLayout:      config.NormalizeLibraryLayout(s.cfg().LibraryLayout),
		HideEmptyLibraries: s.cfg().HideEmptyLibraries,
		RequiresPairing:    s.requiresPairing(),
	}
}

// handleHealth implements GET /api/health per the API.md shape.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{
		Status:          "ok",
		Name:            AppName,
		App:             AppName,
		ID:              ServerID,
		Service:         ServiceDNS,
		Library:         "/api/library",
		AllowLAN:        s.cfg().AllowLAN,
		RequiresPairing: s.requiresPairing(),
	})
}

// handleDiscovery implements GET /api/discovery.
func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	webPort, _ := s.activePorts()
	local := "http://127.0.0.1:" + strconv.Itoa(webPort)
	trackRefresh := "/api/playback/{id}/refresh-tracks"
	resp := api.DiscoveryResponse{
		App:              AppName,
		Name:             AppName,
		ServerID:         ServerID,
		Version:          Version,
		Build:            Build,
		IsEnabled:        true,
		AllowLAN:         s.cfg().AllowLAN,
		RequiresPairing:  s.requiresPairing(),
		Port:             webPort,
		LocalURL:         local,
		LanURL:           s.lanURL(),
		DiscoveryMethods: []string{"bonjour", "manual", "tailscale"},
		TailscaleHint:    "If both devices are on your tailnet, use the tailscale URL.",
		Capabilities: api.DiscoveryCapabilities{
			Books: true, Ebooks: true, Audiobooks: true,
			Themes: true, ThemeSync: true, ProgressSync: true,
			MediaStreaming: true, VideoStreaming: true, Artwork: true,
			LibrarySync: true, RemoteCatalog: true,
		},
		Endpoints: api.DiscoveryEndpoints{
			Health:               "/api/health",
			Library:              "/api/library",
			Audiobooks:           strPtr("/api/audiobooks"),
			AudiobookBrowser:     strPtr("/audiobooks"),
			Discovery:            "/api/discovery",
			Progress:             "/api/progress/{id}",
			Playback:             "/api/playback/{id}",
			PlaybackTrackRefresh: &trackRefresh,
			RefreshTracks:        &trackRefresh,
			Stream:               "/stream/{id}",
			Subtitles:            strPtr("/subtitles/{id}/{index}"),
			Poster:               strPtr("/artwork/poster/{id}"),
			Backdrop:             strPtr("/artwork/backdrop/{id}"),
			MovieMetadata:        strPtr("/api/movies/{id}/metadata"),
			NetworkInterfaces:    strPtr("/api/network/interfaces"),
			NetworkStatus:        strPtr("/api/network/status"),
			NetworkRebind:        strPtr("/api/network/rebind"),
			NetworkExposure:      strPtr("/api/network/exposure"),
			CacheStatus:          strPtr("/api/cache/status"),
		},
		Theme: themeFor(s.cfg().ThemePreset),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) activePorts() (webPort, apiPort int) {
	webPort, apiPort = s.cfg().WebPort, s.cfg().APIPort
	if s.runtimeControl != nil {
		state := s.runtimeControl.CurrentState()
		if state.WebPort > 0 {
			webPort = state.WebPort
		}
		if state.APIPort > 0 {
			apiPort = state.APIPort
		}
	}
	if webPort <= 0 {
		webPort = s.cfg().Port
	}
	if apiPort <= 0 {
		apiPort = webPort + 1
	}
	return webPort, apiPort
}

func strPtr(s string) *string { return &s }

// handleLibrary implements GET /api/library and /library.json with ETag/304.
// The marshaled JSON (plain and gzipped) is memoized per store generation.
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	wantGzip := acceptsGzip(r)
	body, gzipped, etag := s.libraryPayload(wantGzip)
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if gzipped {
		w.Header().Set("Content-Encoding", "gzip")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// libraryPayload returns the cached JSON body for the current generation in
// the requested encoding, plus whether the body is gzipped. body is shared
// state: never mutate it.
func (s *Server) libraryPayload(acceptsGzip bool) ([]byte, bool, string) {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	gen := s.store.Generation()
	scanning := s.scanner != nil && s.scanner.State().Scanning
	// During a scan, the loaded snapshot is more useful than rebuilding a
	// 30MB response for every probe/update. Serve the warm snapshot while the
	// scanner reconciles the NAS, then rebuild once the scan is complete.
	if s.libJSON == nil || (!scanning && s.libGen != gen) {
		s.rebuildLibraryPayload(gen)
	}
	if acceptsGzip && s.libJSONGzip != nil {
		return s.libJSONGzip, true, s.libETag
	}
	return s.libJSON, false, s.libETag
}

// WarmLibraryCache builds the initial client payload from the loaded snapshot.
// main calls this alongside the initial scan so a restart can serve saved
// catalog data without making the first browser request pay the full rebuild.
func (s *Server) WarmLibraryCache() {
	s.libMu.Lock()
	defer s.libMu.Unlock()
	if s.libJSON == nil {
		s.rebuildLibraryPayload(s.store.Generation())
	}
}

func (s *Server) rebuildLibraryPayload(gen int64) {
	etag := s.store.ETag()
	resp := api.LibraryResponse{
		Items:            s.wireItems(),
		Progress:         s.store.Progress(),
		MediaDirectories: s.store.Directories(),
		Activity:         s.store.Activity(),
		ServerSettings:   ptrSettings(s.serverSettings()),
		Theme:            ptrTheme(themeFor(s.cfg().ThemePreset)),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(resp)
	s.libJSON = buf.Bytes()
	s.libJSONGzip = gzipBytes(s.libJSON)
	s.libGen = gen
	s.libETag = etag
}

func gzipBytes(b []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

func ptrSettings(v api.ServerSettings) *api.ServerSettings { return &v }
func ptrTheme(v api.ThemeSnapshot) *api.ThemeSnapshot      { return &v }

// wireItems returns client-safe catalog copies; filesystem paths are
// excluded by the DTO's json:"-" tags.
func (s *Server) wireItems() []api.MediaItem {
	items := s.store.GroupedItems(s.cfg().Libraries)
	posters := s.curatedPosters()
	for index := range items {
		key := items[index].ID
		if items[index].ShowGroupID != nil {
			key = *items[index].ShowGroupID
		}
		if poster, ok := posters[key]; ok {
			url := "/artwork/curated/" + key + "?v=" + poster.Filename
			items[index].PosterURL = &url
			source := "wikimedia-curated"
			items[index].CoverSource = &source
			items[index].CoverAvailable = true
		}
	}
	return items
}

// maxJSONBody bounds request bodies. Larger bodies are rejected rather than
// silently truncated.
const maxJSONBody = 1 << 20

// jsonDecode decodes a bounded JSON request body and rejects trailing data.
func jsonDecode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing data after JSON body")
	}
	return nil
}

// handleStatus implements GET /api/status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := s.scanner.State()
	writeJSON(w, http.StatusOK, map[string]any{
		"scanning":   state.Scanning,
		"enriching":  s.enriching.Load(),
		"lastScanAt": state.LastScanAt,
		"itemsSeen":  state.ItemsSeen,
		"lastResult": map[string]int{
			"added":   state.Added,
			"updated": state.Updated,
			"removed": state.Removed,
			"skipped": state.Skipped,
		},
		"itemCount": s.store.Count(),
		"version":   Version,
	})
}

func (s *Server) handleCacheStatus(w http.ResponseWriter, r *http.Request) {
	if s.mediaCache == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"policy": map[string]string{
				"ebook":       "highest priority",
				"audiobook":   "high priority",
				"movie":       "medium priority; newer release years retained first",
				"documentary": "medium-low priority; newer release years retained first",
				"tvShow":      "low priority",
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, s.mediaCache.Status())
}

func (s *Server) handleLibraryHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Health(s.cfg().Libraries))
}

func (s *Server) handleOptimizationQueue(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.OptimizationQueue())
}

func (s *Server) handleAudiobookOptimizationJobs(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationEnqueue(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var body struct {
		ItemIDs                 []string       `json:"itemIDs"`
		ApprovedCatalogCoverIDs []string       `json:"approvedCatalogCoverIDs"`
		BitrateKbps             map[string]int `json:"bitrateKbps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid optimization job request")
		return
	}
	approved := make(map[string]bool, len(body.ApprovedCatalogCoverIDs))
	for _, id := range body.ApprovedCatalogCoverIDs {
		approved[id] = true
	}
	jobs, err := s.audiobookOptimizer.Enqueue(body.ItemIDs, approved, body.BitrateKbps)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"jobs": jobs})
}

func (s *Server) handleAudiobookOptimizationPause(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Pause(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationResume(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Resume(); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationRetry(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Retry(r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationPrioritize(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Prioritize(r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationReview(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid playback review")
		return
	}
	if err := s.audiobookOptimizer.MarkPlaybackReview(r.PathValue("id"), body.Decision, body.Note); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationPromote(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Promote(r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

func (s *Server) handleAudiobookOptimizationStream(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	f, name, modified, err := s.audiobookOptimizer.OpenStagedOutput(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "Verified staged output is unavailable")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "audio/mp4")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, name, modified, f)
}

func (s *Server) handleAudiobookOptimizationCancel(w http.ResponseWriter, r *http.Request) {
	if s.audiobookOptimizer == nil {
		writeError(w, http.StatusServiceUnavailable, "Audiobook optimization worker is unavailable")
		return
	}
	if err := s.audiobookOptimizer.Cancel(r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookOptimizer.Snapshot())
}

// --- playback sessions ---

// defaultSelections ports the Swift server's anime-friendly defaults:
// prefer Japanese audio; enable first English subtitle track.
func defaultSelections(audio, subtitle []api.PlaybackTrack) (audioID, subID *string, subsEnabled *bool) {
	for i := range audio {
		if audio[i].LanguageCode != nil && *audio[i].LanguageCode == "ja" {
			id := audio[i].ID
			audioID = &id
			break
		}
	}
	for i := range subtitle {
		if subtitle[i].LanguageCode != nil && *subtitle[i].LanguageCode == "en" {
			id := subtitle[i].ID
			subID = &id
			yes := true
			subsEnabled = &yes
			break
		}
	}
	return audioID, subID, subsEnabled
}

// sessionFor builds the PlaybackSession response for an item.
func (s *Server) sessionFor(item *library.Item) api.PlaybackSession {
	sess := api.PlaybackSession{
		ItemID:         &item.ID,
		StreamURL:      "/stream/" + item.ID,
		Duration:       item.DurationSeconds,
		AudioTracks:    item.EmbeddedAudioTracks,
		SubtitleTracks: library.MergedSubtitleTracks(item),
	}
	if item.DurationSeconds > 0 && item.ProgressSeconds > 0 {
		sess.Percent = item.ProgressSeconds / item.DurationSeconds * 100
	}
	if prev, ok := s.store.ProgressFor(item.ID); ok {
		sess.Seconds = prev.Seconds
		sess.Duration = prev.Duration
		if prev.Duration > 0 {
			sess.Percent = prev.Seconds / prev.Duration * 100
		}
		u := prev.UpdatedAt
		sess.UpdatedAt = &u
		sess.AudioTrackID = prev.AudioTrackID
		sess.SubtitleTrackID = prev.SubtitleTrackID
		sess.SubtitlesEnabled = prev.SubtitlesEnabled
	}
	if sess.AudioTrackID == nil && sess.SubtitleTrackID == nil && sess.SubtitlesEnabled == nil {
		a, sub, en := defaultSelections(sess.AudioTracks, sess.SubtitleTracks)
		sess.AudioTrackID, sess.SubtitleTrackID, sess.SubtitlesEnabled = a, sub, en
	}
	return sess
}

func (s *Server) handlePlaybackGet(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionFor(item))
}

var allowedMethods = map[string]bool{http.MethodPost: true, http.MethodPatch: true, http.MethodPut: true}

// applyUpdate decodes a PlaybackStateUpdate and merges it last-write-wins.
// Omitted optional track fields keep their previously saved values.
func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) (*library.Item, bool) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return nil, false
	}
	var upd api.PlaybackStateUpdate
	if err := jsonDecode(w, r, &upd); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid playback payload")
		return nil, false
	}

	rec, _ := s.store.ProgressFor(item.ID)
	rec.ItemID = item.ID
	rec.Seconds = upd.Seconds
	if upd.Duration > 0 {
		rec.Duration = upd.Duration
	}
	if upd.AudioTrackID != nil {
		rec.AudioTrackID = upd.AudioTrackID
	}
	if upd.SubtitleTrackID != nil {
		rec.SubtitleTrackID = upd.SubtitleTrackID
	}
	if upd.SubtitlesEnabled != nil {
		rec.SubtitlesEnabled = upd.SubtitlesEnabled
	}
	rec.UpdatedAt = time.Now().UTC()
	if rec.ID == "" {
		rec.ID = api.NewID()
	}
	s.store.SetProgress(rec)
	s.progressChanged()
	return item, true
}

func (s *Server) handlePlaybackUpdate(w http.ResponseWriter, r *http.Request) {
	if !allowedMethods[r.Method] {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	item, ok := s.applyUpdate(w, r)
	if !ok {
		return
	}
	fresh, ok := s.store.Get(item.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionFor(fresh))
}

func (s *Server) handleProgressUpdate(w http.ResponseWriter, r *http.Request) {
	item, ok := s.applyUpdate(w, r)
	if !ok {
		return
	}
	fresh, ok := s.store.Get(item.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionFor(fresh))
}

// handleRefreshTracks re-probes one item and returns the fresh session.
func (s *Server) handleRefreshTracks(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.store.Get(id); !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	if s.refresher != nil {
		if err := s.refresher.RefreshTracks(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, "Track refresh failed")
			return
		}
	}
	item, ok := s.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionFor(item))
}

// --- audiobooks ---

type audiobookChapter struct {
	Index        int      `json:"index"`
	Title        string   `json:"title"`
	StartSeconds float64  `json:"startSeconds"`
	EndSeconds   *float64 `json:"endSeconds"`
}

// catalogPart is one file of a book that was delivered as many files. A book
// is listed once; its parts are listed here so the client can play them in
// order. IDs are real item IDs, so streaming and progress still address the
// individual file.
type catalogPart struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Index           int     `json:"index"`
	DurationSeconds float64 `json:"durationSeconds"`
	PosterURL       *string `json:"posterURL"`
}

// catalogConflict reports a book folder whose files could not be merged into
// one entry. It is surfaced rather than swallowed so a wrong total never
// reaches the UI without an explanation.
type catalogConflict struct {
	// BookID identifies the book folder; for a folder that produced no book
	// entry, the affected items are listed in ItemIDs.
	Kind    string   `json:"kind"`
	ItemIDs []string `json:"itemIDs"`
	Detail  string   `json:"detail"`
}

type catalogItem struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle"`
	Author          *string  `json:"author"`
	Series          *string  `json:"series"`
	Narrator        *string  `json:"narrator"`
	Summary         string   `json:"summary"`
	Studio          string   `json:"studio"`
	Year            int      `json:"year"`
	DurationSeconds float64  `json:"durationSeconds"`
	ChapterCount    int      `json:"chapterCount"`
	PosterURL       *string  `json:"posterURL"`
	BackdropURL     *string  `json:"backdropURL"`
	Tags            []string `json:"tags"`
	// PartCount and Parts describe a book delivered as many files. A single
	// file book omits them, so clients can treat the common case as unchanged.
	PartCount int           `json:"partCount,omitempty"`
	Parts     []catalogPart `json:"parts,omitempty"`
	// BookConflict explains why a folder's files were not merged. Without it a
	// whole-book file sitting beside its own segments would silently double
	// the listed runtime, and a pair of duplicate copies would look like one
	// book.
	BookConflict *catalogConflict `json:"bookConflict,omitempty"`
	// Playback resume state (beta rails). Zero/omitted when never played.
	ProgressSeconds   float64    `json:"progressSeconds"`
	ProgressUpdatedAt *time.Time `json:"progressUpdatedAt,omitempty"`
}

type catalogResponse struct {
	Items       []catalogItem     `json:"items"`
	Count       int               `json:"count"`
	Theme       api.ThemeSnapshot `json:"theme"`
	GeneratedAt time.Time         `json:"generatedAt"`
}

type catalogDetail struct {
	Item     catalogItem        `json:"item"`
	Chapters []audiobookChapter `json:"chapters"`
}

func (s *Server) toCatalogItem(it *library.Item) catalogItem {
	// Author/Narrator come from real item fields when known (enrichment or
	// filename parsing); Studio/Tags are the legacy fallbacks for items
	// enriched before those fields existed.
	var author, narrator, series *string
	if it.Kind == api.KindAudiobook || it.Kind == api.KindEbook {
		// For book kinds the parser stores the series (or filename-derived
		// author) in Studio; surface it on the dedicated Series field too.
		if sv := strings.TrimSpace(it.Studio); sv != "" {
			series = &sv
		}
	}
	if it.Author != nil {
		author = it.Author
	} else if len(it.Tags) > 0 {
		a := it.Tags[0]
		author = &a
	}
	if it.Narrator != nil {
		narrator = it.Narrator
	} else if it.Studio != "" && it.Author != nil && it.Studio != *it.Author {
		n := it.Studio
		narrator = &n
	}
	out := catalogItem{
		ID:              it.ID,
		Title:           it.Title,
		Subtitle:        it.Subtitle,
		Author:          author,
		Narrator:        narrator,
		Series:          series,
		Summary:         it.Summary,
		Studio:          it.Studio,
		Year:            it.Year,
		DurationSeconds: it.DurationSeconds,
		PosterURL:       it.PosterURL,
		BackdropURL:     it.BackdropURL,
		Tags:            it.Tags,
		ProgressSeconds: it.ProgressSeconds,
	}
	if rec, ok := s.store.ProgressFor(it.ID); ok && !rec.UpdatedAt.IsZero() {
		u := rec.UpdatedAt
		out.ProgressUpdatedAt = &u
	}
	return out
}

// mediaCatalog serves the audiobook and ebook catalog routes: kind-filtered,
// optionally searched, with the theme snapshot. Author/Narrator participate in
// the search so books can be found by name as well as title.
//
// An audiobook library is collapsed to one row per book. A book delivered as
// many files used to appear once per file, which put a 148-file recording into
// the listing as 148 rows and made alphabetical browsing useless. The rows for
// one book are merged, with the parts preserved for playback.
func (s *Server) mediaCatalog(w http.ResponseWriter, r *http.Request, kind api.MediaKind) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	all := s.store.InternalItemsOfKind(kind)
	items := []catalogItem{}
	if kind == api.KindAudiobook {
		items = s.collapseIntoBooks(all, q)
	} else {
		for _, it := range all {
			if q != "" && !catalogMatches(it, q) {
				continue
			}
			items = append(items, s.toCatalogItem(it))
		}
	}
	writeJSON(w, http.StatusOK, catalogResponse{
		Items: items, Count: len(items),
		Theme: themeFor(s.cfg().ThemePreset), GeneratedAt: time.Now().UTC(),
	})
}

// collapseIntoBooks merges the items of one book into a single catalog row.
// The first part supplies the book-level metadata (title, author, narrator,
// cover), durations and progress are summed, and the parts are returned in
// playback order. Books whose folder cannot be resolved are listed per file so
// they never disappear from the catalog.
func (s *Server) collapseIntoBooks(all []*library.Item, q string) []catalogItem {
	groups, conflicts := library.BookGroups(all, s.cfg().Libraries)
	byGroup := map[string][]*library.Item{}
	var ungrouped []*library.Item
	// Files left out of a book because of a conflict still need to be listed,
	// flagged, so nothing silently disappears from the catalog.
	conflicted := map[string]string{}

	for _, it := range all {
		g, ok := groups[it.ID]
		if !ok {
			ungrouped = append(ungrouped, it)
			continue
		}
		byGroup[g.ID] = append(byGroup[g.ID], it)
	}
	for id, c := range conflicts {
		for _, itemID := range c.ExcludedIDs {
			conflicted[itemID] = id
		}
	}

	items := make([]catalogItem, 0, len(byGroup)+len(ungrouped))

	// Deterministic book order: by title, then by the book id so two books
	// sharing a title keep a stable relative order. Titles are resolved once
	// rather than per comparison.
	titleOf := make(map[string]string, len(byGroup))
	for id, parts := range byGroup {
		titleOf[id] = groups[parts[0].ID].Title
	}
	ids := make([]string, 0, len(byGroup))
	for id := range byGroup {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool {
		if titleOf[ids[a]] != titleOf[ids[b]] {
			return titleOf[ids[a]] < titleOf[ids[b]]
		}
		return ids[a] < ids[b]
	})

	for _, id := range ids {
		parts := byGroup[id]
		// Playback order, so a client that queues Parts gets the book's own
		// track order rather than map iteration order.
		order := map[string]int{}
		for _, p := range parts {
			order[p.ID] = groups[p.ID].Index
		}
		sort.Slice(parts, func(a, b int) bool { return order[parts[a].ID] < order[parts[b].ID] })

		head := parts[0]
		entry := s.toCatalogItem(head)
		if q != "" && !catalogMatches(head, q) {
			continue
		}
		if len(parts) == 1 {
			// A single-file book is the common case and must stay exactly as it
			// was: no parts array, no changed duration. Clients that do not know
			// about parts keep working unchanged.
			items = append(items, entry)
			continue
		}
		entry.PartCount = len(parts)
		entry.DurationSeconds = 0
		entry.ProgressSeconds = 0
		entry.ProgressUpdatedAt = nil
		entry.Parts = make([]catalogPart, 0, len(parts))
		if c, ok := conflicts[id]; ok {
			entry.BookConflict = &catalogConflict{
				Kind: c.Kind, ItemIDs: c.ExcludedIDs, Detail: c.Detail,
			}
		}
		for i, p := range parts {
			entry.DurationSeconds += p.DurationSeconds
			entry.ProgressSeconds += p.ProgressSeconds
			entry.Parts = append(entry.Parts, catalogPart{
				ID:              p.ID,
				Title:           p.Title,
				Index:           i + 1,
				DurationSeconds: p.DurationSeconds,
				PosterURL:       p.PosterURL,
			})
			if entry.PosterURL == nil && p.PosterURL != nil {
				entry.PosterURL = p.PosterURL
			}
			if entry.ProgressUpdatedAt == nil {
				if rec, ok := s.store.ProgressFor(p.ID); ok && !rec.UpdatedAt.IsZero() {
					u := rec.UpdatedAt
					entry.ProgressUpdatedAt = &u
				}
			}
		}
		items = append(items, entry)
	}

	for _, it := range ungrouped {
		if q != "" && !catalogMatches(it, q) {
			continue
		}
		entry := s.toCatalogItem(it)
		if bookID, ok := conflicted[it.ID]; ok {
			if c, found := conflicts[bookID]; found {
				entry.BookConflict = &catalogConflict{
					Kind: c.Kind, ItemIDs: c.ExcludedIDs, Detail: c.Detail,
				}
			}
		}
		items = append(items, entry)
	}
	return items
}

// catalogMatches reports whether an item matches a lowercased query across
// title, summary, studio, author, and narrator.
func catalogMatches(it *library.Item, q string) bool {
	if strings.Contains(strings.ToLower(it.Title+" "+it.Summary+" "+it.Studio), q) {
		return true
	}
	if it.Author != nil && strings.Contains(strings.ToLower(*it.Author), q) {
		return true
	}
	return it.Narrator != nil && strings.Contains(strings.ToLower(*it.Narrator), q)
}

func (s *Server) handleAudiobooks(w http.ResponseWriter, r *http.Request) {
	s.mediaCatalog(w, r, api.KindAudiobook)
}

// bookPartsFor returns the ordered files of the book that item belongs to. The
// third return is false for a single-file book, so a caller can leave the
// response byte-identical to the pre-grouping shape.
func (s *Server) bookPartsFor(item *library.Item) ([]catalogPart, *catalogConflict, bool) {
	all := s.store.InternalItemsOfKind(api.KindAudiobook)
	groups, conflicts := library.BookGroups(all, s.cfg().Libraries)
	g, ok := groups[item.ID]
	if !ok || g.Count < 2 {
		return nil, nil, false
	}
	parts := make([]catalogPart, 0, g.Count)
	for _, p := range all {
		if pg, ok := groups[p.ID]; ok && pg.ID == g.ID {
			parts = append(parts, catalogPart{
				ID:              p.ID,
				Title:           p.Title,
				Index:           pg.Index,
				DurationSeconds: p.DurationSeconds,
				PosterURL:       p.PosterURL,
			})
		}
	}
	sort.Slice(parts, func(a, b int) bool { return parts[a].Index < parts[b].Index })
	var conflict *catalogConflict
	if c, found := conflicts[g.ID]; found {
		conflict = &catalogConflict{Kind: c.Kind, ItemIDs: c.ExcludedIDs, Detail: c.Detail}
	}
	return parts, conflict, true
}

func (s *Server) handleAudiobookDetail(w http.ResponseWriter, r *http.Request) {
	it, ok := s.store.Get(r.PathValue("id"))
	if !ok || it.Kind != api.KindAudiobook {
		writeError(w, http.StatusNotFound, "Audiobook not found")
		return
	}
	// A request may address any single file of a multi-file book, so the parts
	// list is resolved from the book grouping rather than assumed to be the
	// requested file.
	if parts, conflict, multi := s.bookPartsFor(it); multi {
		detail := s.toCatalogItem(it)
		detail.PartCount = len(parts)
		detail.Parts = parts
		detail.BookConflict = conflict
		chapters := []audiobookChapter{}
		if s.chapters != nil {
			if got, err := s.chapters.ChaptersFor(r.Context(), it.ID); err == nil && len(got) > 0 {
				for _, c := range got {
					chapters = append(chapters, audiobookChapter{
						Index:        c.Index,
						Title:        c.Title,
						StartSeconds: c.StartSeconds,
						EndSeconds:   c.EndSeconds,
					})
				}
			}
		}
		writeJSON(w, http.StatusOK, catalogDetail{Item: detail, Chapters: chapters})
		return
	}
	chapters := []audiobookChapter{}
	if s.chapters != nil {
		if got, err := s.chapters.ChaptersFor(r.Context(), it.ID); err == nil && len(got) > 0 {
			for _, c := range got {
				chapters = append(chapters, audiobookChapter{
					Index:        c.Index,
					Title:        c.Title,
					StartSeconds: c.StartSeconds,
					EndSeconds:   c.EndSeconds,
				})
			}
		}
	}
	detail := s.toCatalogItem(it)
	detail.ChapterCount = len(chapters)
	writeJSON(w, http.StatusOK, catalogDetail{
		Item:     detail,
		Chapters: chapters,
	})
}

// handleEbooks implements GET /api/ebooks?q= — ebook catalog.
func (s *Server) handleEbooks(w http.ResponseWriter, r *http.Request) {
	s.mediaCatalog(w, r, api.KindEbook)
}

// handleAudiobookBrowser serves the audiobook player page. The layout follows
// the audiobookLayout setting ("rails" default, "classic" for the legacy
// list); ?layout= overrides per visit.
func (s *Server) handleAudiobookBrowser(w http.ResponseWriter, r *http.Request) {
	layout := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("layout")))
	if layout == "" {
		layout = config.NormalizeAudiobookLayout(s.cfg().AudiobookLayout)
	}
	if layout == "classic" {
		serveGzippableHTML(w, r, audiobooksPage)
		return
	}
	serveGzippableHTML(w, r, audiobooksBetaPage)
}

// handleAudiobookClassic always serves the legacy list layout ("Classic").
func (s *Server) handleAudiobookClassic(w http.ResponseWriter, r *http.Request) {
	serveGzippableHTML(w, r, audiobooksPage)
}

// handleAudiobookBeta always serves the rails layout (kept as a stable alias).
func (s *Server) handleAudiobookBeta(w http.ResponseWriter, r *http.Request) {
	serveGzippableHTML(w, r, audiobooksBetaPage)
}

// handleEbookBrowser serves the ebook browser page.
func (s *Server) handleEbookBrowser(w http.ResponseWriter, r *http.Request) {
	serveGzippableHTML(w, r, ebooksPage)
}
