package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
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
	return api.ServerSettings{
		IsEnabled:       true,
		AllowLAN:        s.cfg.AllowLAN,
		Port:            s.cfg.Port,
		ThemePreset:     s.cfg.ThemePreset,
		RequiresPairing: s.requiresPairing(),
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
		AllowLAN:        s.cfg.AllowLAN,
		RequiresPairing: s.requiresPairing(),
	})
}

// handleDiscovery implements GET /api/discovery.
func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	local := "http://127.0.0.1:" + itoa(s.cfg.Port)
	trackRefresh := "/api/playback/{id}/refresh-tracks"
	resp := api.DiscoveryResponse{
		App:              AppName,
		Name:             AppName,
		ServerID:         ServerID,
		Version:          Version,
		Build:            Build,
		IsEnabled:        true,
		AllowLAN:         s.cfg.AllowLAN,
		RequiresPairing:  s.requiresPairing(),
		Port:             s.cfg.Port,
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
		},
		Theme: themeFor(s.cfg.ThemePreset),
	}
	writeJSON(w, http.StatusOK, resp)
}

func strPtr(s string) *string { return &s }

// handleLibrary implements GET /api/library and /library.json with ETag/304.
// The marshaled JSON (plain and gzipped) is memoized per store generation.
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	acceptsGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	body, gzipped, etag := s.libraryPayload(acceptsGzip)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
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
	if s.libJSON == nil || s.libGen != gen {
		s.rebuildLibraryPayload(gen)
	}
	if acceptsGzip && s.libJSONGzip != nil {
		return s.libJSONGzip, true, s.libETag
	}
	return s.libJSON, false, s.libETag
}

func (s *Server) rebuildLibraryPayload(gen int64) {
	etag := s.store.ETag()
	resp := api.LibraryResponse{
		Items:            s.wireItems(),
		Progress:         s.store.Progress(),
		MediaDirectories: s.store.Directories(),
		Activity:         s.store.Activity(),
		ServerSettings:   ptrSettings(s.serverSettings()),
		Theme:            ptrTheme(themeFor(s.cfg.ThemePreset)),
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

func gunzipOnce(b []byte) []byte {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		return b
	}
	return out
}

func ptrSettings(v api.ServerSettings) *api.ServerSettings { return &v }
func ptrTheme(v api.ThemeSnapshot) *api.ThemeSnapshot      { return &v }

// wireItems returns client-safe catalog copies; filesystem paths are
// excluded by the DTO's json:"-" tags.
func (s *Server) wireItems() []api.MediaItem { return s.store.Items() }

// jsonDecode decodes a bounded JSON request body.
func jsonDecode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

// handleStatus implements GET /api/status.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	state := s.scanner.State()
	writeJSON(w, http.StatusOK, map[string]any{
		"scanning":   state.Scanning,
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
	if err := jsonDecode(r, &upd); err != nil {
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
	if s.onMutation != nil {
		s.onMutation()
	}
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
	fresh, _ := s.store.Get(item.ID)
	writeJSON(w, http.StatusOK, s.sessionFor(fresh))
}

func (s *Server) handleProgressUpdate(w http.ResponseWriter, r *http.Request) {
	item, ok := s.applyUpdate(w, r)
	if !ok {
		return
	}
	fresh, _ := s.store.Get(item.ID)
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
		if err := s.refresher.RefreshTracks(id); err != nil {
			writeError(w, http.StatusInternalServerError, "Track refresh failed")
			return
		}
	}
	item, _ := s.store.Get(id)
	writeJSON(w, http.StatusOK, s.sessionFor(item))
}

// --- audiobooks ---

type audiobookChapter struct {
	Index        int      `json:"index"`
	Title        string   `json:"title"`
	StartSeconds float64  `json:"startSeconds"`
	EndSeconds   *float64 `json:"endSeconds"`
}

type audiobookItem struct {
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
}

type audiobookResponse struct {
	Items       []audiobookItem   `json:"items"`
	Count       int               `json:"count"`
	Theme       api.ThemeSnapshot `json:"theme"`
	GeneratedAt time.Time         `json:"generatedAt"`
}

type audiobookDetail struct {
	Item     audiobookItem      `json:"item"`
	Chapters []audiobookChapter `json:"chapters"`
}

func (s *Server) toAudiobookItem(it *library.Item) audiobookItem {
	var author, narrator *string
	if it.Studio != "" {
		n := it.Studio
		narrator = &n
	}
	if len(it.Tags) > 0 {
		a := it.Tags[0]
		author = &a
	}
	return audiobookItem{
		ID:              it.ID,
		Title:           it.Title,
		Subtitle:        it.Subtitle,
		Author:          author,
		Narrator:        narrator,
		Summary:         it.Summary,
		Studio:          it.Studio,
		Year:            it.Year,
		DurationSeconds: it.DurationSeconds,
		PosterURL:       it.PosterURL,
		BackdropURL:     it.BackdropURL,
		Tags:            it.Tags,
	}
}

func (s *Server) handleAudiobooks(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	items := []audiobookItem{}
	for _, it := range s.store.InternalItems() {
		if it.Kind != api.KindAudiobook {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Title+" "+it.Summary+" "+it.Studio), q) {
			continue
		}
		items = append(items, s.toAudiobookItem(it))
	}
	writeJSON(w, http.StatusOK, audiobookResponse{
		Items: items, Count: len(items),
		Theme: themeFor(s.cfg.ThemePreset), GeneratedAt: time.Now().UTC(),
	})
}

func (s *Server) handleAudiobookDetail(w http.ResponseWriter, r *http.Request) {
	it, ok := s.store.Get(r.PathValue("id"))
	if !ok || it.Kind != api.KindAudiobook {
		writeError(w, http.StatusNotFound, "Audiobook not found")
		return
	}
	chapters := []audiobookChapter{}
	if s.chapters != nil {
		if got, err := s.chapters.ChaptersFor(it.ID); err == nil && len(got) > 0 {
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
	writeJSON(w, http.StatusOK, audiobookDetail{
		Item:     s.toAudiobookItem(it),
		Chapters: chapters,
	})
}

// handleAudiobookBrowser serves the audiobook player page (port of
// SonderWebInterface.audiobooksHTML).
func (s *Server) handleAudiobookBrowser(w http.ResponseWriter, r *http.Request) {
	serveGzippableHTML(w, r, audiobooksPage)
}
