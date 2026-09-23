// Package httpapi serves the TM Sonder wire contract documented in API.md.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/audiobookopt"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/transcode"
)

const (
	AppName    = "TM Sonder"
	ServerID   = "tm-sonder"
	ServiceDNS = "_tmsonder._tcp"
)

// Version and Build are link-time stamped for release binaries. Keeping the
// defaults useful makes `go run` and local tests self-describing too.
var (
	Version = "0.2.0-dev"
	Build   = "local"
)

// TrackRefresher re-probes one item's tracks (refresh-tracks endpoint).
// The context is the request context so a disconnect cancels the probe.
type TrackRefresher interface {
	RefreshTracks(ctx context.Context, itemID string) error
}

// ChapterProvider supplies audiobook chapter markers for the detail route.
type ChapterProvider interface {
	ChaptersFor(ctx context.Context, itemID string) ([]api.AudiobookChapter, error)
}

// Server wires the catalog, scanner, and transcoder into the HTTP contract.
type Server struct {
	cfgPtr             atomic.Pointer[config.Config]
	store              *library.Store
	scanner            *library.Scanner
	tm                 *transcode.Manager
	refresher          TrackRefresher
	chapters           ChapterProvider
	audiobookOptimizer *audiobookopt.Manager
	onMutation         func()
	onProgress         func()

	configPath   string
	snapshotPath string
	enriching    atomic.Bool

	settingsVersion int64

	// libraryCache memoizes the /api/library JSON (plain and gzipped) per
	// store generation, so repeat page loads skip marshal + gzip work.
	libMu       sync.Mutex
	libGen      int64
	libJSON     []byte
	libJSONGzip []byte
	libETag     string

	mux    *http.ServeMux
	logger *log.Logger
}

func New(cfg *config.Config, store *library.Store, scanner *library.Scanner, tm *transcode.Manager) *Server {
	s := &Server{
		store:   store,
		scanner: scanner,
		tm:      tm,
		logger:  log.New(log.Writer(), "sonder-http ", log.LstdFlags),
	}
	s.cfgPtr.Store(cfg)
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// cfg returns the current config snapshot. Settings updates replace the whole
// value (copy-on-write) so request handlers never race a settings writer.
func (s *Server) cfg() *config.Config { return s.cfgPtr.Load() }

// updateConfig applies mutate to a private copy of the current config and
// atomically installs it, returning the new snapshot.
func (s *Server) updateConfig(mutate func(next *config.Config)) *config.Config {
	for {
		cur := s.cfgPtr.Load()
		next := *cur
		mutate(&next)
		if s.cfgPtr.CompareAndSwap(cur, &next) {
			return &next
		}
	}
}

// SetTrackRefresher wires optional probe-backed track refresh.
func (s *Server) SetTrackRefresher(r TrackRefresher) { s.refresher = r }

// SetChapterProvider wires optional audiobook chapter extraction.
func (s *Server) SetChapterProvider(p ChapterProvider) { s.chapters = p }

// SetAudiobookOptimizer wires the persistent staged-conversion worker.
func (s *Server) SetAudiobookOptimizer(m *audiobookopt.Manager) { s.audiobookOptimizer = m }

// SetAutoSave registers a callback fired after catalog mutations so main can
// debounce snapshot writes.
func (s *Server) SetAutoSave(fn func()) { s.onMutation = fn }

// SetProgressSave registers a callback fired after playback-progress writes.
// main wires this to a small progress sidecar so heartbeats do not rewrite the
// full catalog snapshot. Falls back to the catalog callback when unset.
func (s *Server) SetProgressSave(fn func()) { s.onProgress = fn }

// progressChanged notifies the persistence hook for a progress-only mutation.
func (s *Server) progressChanged() {
	if s.onProgress != nil {
		s.onProgress()
		return
	}
	if s.onMutation != nil {
		s.onMutation()
	}
}

// SetConfigPath records where the running config was loaded from; settings
// saves write back to this file.
func (s *Server) SetConfigPath(path string) { s.configPath = path }

// SetSnapshotPath records where the catalog snapshot lives so rescans and
// enrichment triggered from the API can persist their results.
func (s *Server) SetSnapshotPath(path string) { s.snapshotPath = path }

// persistConfig atomically writes the current in-memory config to the config
// file. Note: // comments from a hand-edited file are lost on save.
func (s *Server) persistConfig() error {
	if s.configPath == "" {
		return fmt.Errorf("no config path recorded")
	}
	data, err := json.MarshalIndent(s.cfg(), "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sonder-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.configPath)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// persistPairingToken stores the token beside the snapshot so restarts keep it.
func (s *Server) persistPairingToken(token string) {
	if s.cfg().DataDir == "" {
		return
	}
	p := filepath.Join(s.cfg().DataDir, "pairing-token")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(token+"\n"), 0o600)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /ping", s.handleAudiobookshelfPing)
	m.HandleFunc("GET /status", s.handleAudiobookshelfStatus)
	m.HandleFunc("POST /login", s.handleAudiobookshelfLogin)
	m.HandleFunc("GET /api/libraries", s.handleAudiobookshelfLibraries)
	m.HandleFunc("GET /api/libraries/{libraryID}/items", s.handleAudiobookshelfItems)
	m.HandleFunc("GET /api/items/{id}", s.handleAudiobookshelfItem)
	m.HandleFunc("GET /api/items/{id}/cover", s.handleAudiobookshelfCover)
	m.HandleFunc("GET /api/items/{id}/file/{ino}/download", s.handleAudiobookshelfDownload)
	m.HandleFunc("GET /System/Info/Public", s.handleJellyfinPublicInfo)
	m.HandleFunc("GET /QuickConnect/Enabled", s.handleJellyfinQuickConnectEnabled)
	m.HandleFunc("POST /Users/AuthenticateByName", s.handleJellyfinAuthenticate)
	m.HandleFunc("GET /UserViews", s.handleJellyfinViews)
	m.HandleFunc("GET /Items", s.handleJellyfinItems)
	m.HandleFunc("GET /Users/{userID}/Views", s.handleJellyfinViews)
	m.HandleFunc("GET /Users/{userID}/Items", s.handleJellyfinItems)
	m.HandleFunc("GET /Users/{userID}/Items/{itemID}", s.handleJellyfinItem)
	m.HandleFunc("GET /Items/{itemID}", s.handleJellyfinItem)
	m.HandleFunc("GET /Items/{itemID}/Images/Primary", s.handleJellyfinImage)
	m.HandleFunc("GET /Items/{itemID}/Download", s.handleJellyfinDownload)
	m.HandleFunc("GET /Artists/AlbumArtists", s.handleJellyfinAlbumArtists)
	m.HandleFunc("GET /Persons", s.handleJellyfinPersons)
	m.HandleFunc("GET /api/health", s.handleHealth)
	m.HandleFunc("GET /api/discovery", s.handleDiscovery)
	m.HandleFunc("GET /api/library", s.handleLibrary)
	m.HandleFunc("GET /library.json", s.handleLibrary)
	m.HandleFunc("GET /api/status", s.handleStatus)
	m.HandleFunc("GET /api/library/health", s.handleLibraryHealth)
	m.HandleFunc("GET /api/optimization/queue", s.handleOptimizationQueue)
	m.HandleFunc("GET /api/optimization/audiobooks/jobs", s.handleAudiobookOptimizationJobs)
	m.HandleFunc("POST /api/optimization/audiobooks/jobs", s.handleAudiobookOptimizationEnqueue)
	m.HandleFunc("POST /api/optimization/audiobooks/queue/pause", s.handleAudiobookOptimizationPause)
	m.HandleFunc("POST /api/optimization/audiobooks/queue/resume", s.handleAudiobookOptimizationResume)
	m.HandleFunc("POST /api/optimization/audiobooks/jobs/{id}/retry", s.handleAudiobookOptimizationRetry)
	m.HandleFunc("POST /api/optimization/audiobooks/jobs/{id}/prioritize", s.handleAudiobookOptimizationPrioritize)
	m.HandleFunc("POST /api/optimization/audiobooks/jobs/{id}/review", s.handleAudiobookOptimizationReview)
	m.HandleFunc("POST /api/optimization/audiobooks/jobs/{id}/promote", s.handleAudiobookOptimizationPromote)
	m.HandleFunc("GET /api/optimization/audiobooks/jobs/{id}/stream", s.handleAudiobookOptimizationStream)
	m.HandleFunc("DELETE /api/optimization/audiobooks/jobs/{id}", s.handleAudiobookOptimizationCancel)
	m.HandleFunc("GET /api/library/storage", s.handleLibraryStorage)
	m.HandleFunc("GET /api/data/export", s.handleDataExport)
	m.HandleFunc("POST /api/data/import", s.handleDataImport)
	m.HandleFunc("GET /api/lists", s.handleLists)
	m.HandleFunc("POST /api/lists", s.handleLists)
	m.HandleFunc("PATCH /api/lists/{listID}", s.handleList)
	m.HandleFunc("DELETE /api/lists/{listID}", s.handleList)
	m.HandleFunc("POST /api/lists/{listID}/items", s.handleListItems)
	m.HandleFunc("DELETE /api/lists/{listID}/items/{itemID}", s.handleListItems)
	m.HandleFunc("POST /api/lists/{listID}/reorder", s.handleListReorder)
	m.HandleFunc("GET /api/playback/{id}", s.handlePlaybackGet)
	m.HandleFunc("POST /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("PATCH /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("PUT /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("POST /api/playback/{id}/refresh-tracks", s.handleRefreshTracks)
	m.HandleFunc("POST /api/progress/{id}", s.handleProgressUpdate)
	m.HandleFunc("GET /stream/{id}", s.handleStream)
	m.HandleFunc("GET /subtitles/{id}/{index}", s.handleSubtitle)
	m.HandleFunc("GET /artwork/poster/{id}", s.handlePoster)
	m.HandleFunc("GET /artwork/curated/{id}", s.handleCuratedPoster)
	m.HandleFunc("GET /artwork/backdrop/{id}", s.handleBackdrop)
	m.HandleFunc("GET /api/audiobooks", s.handleAudiobooks)
	m.HandleFunc("GET /api/audiobooks/{id}", s.handleAudiobookDetail)
	m.HandleFunc("GET /audiobooks", s.handleAudiobookBrowser)
	m.HandleFunc("GET /api/ebooks", s.handleEbooks)
	m.HandleFunc("GET /ebooks", s.handleEbookBrowser)
	m.HandleFunc("GET /api/settings", s.handleSettingsGet)
	m.HandleFunc("GET /api/settings/browse", s.handleSettingsBrowse)
	m.HandleFunc("PUT /api/settings", s.handleSettingsPut)
	m.HandleFunc("PATCH /api/settings", s.handleSettingsPut)
	m.HandleFunc("POST /api/settings/rescan", s.handleSettingsRescan)
	m.HandleFunc("POST /api/settings/enrich", s.handleSettingsEnrich)
	m.HandleFunc("GET /{$}", s.handleIndex)
	m.HandleFunc("GET /shared.js", s.handleSharedJS)
	m.HandleFunc("GET /library.css", s.handleLibraryCSS)
	m.HandleFunc("GET /library.js", s.handleLibraryJS)
	m.HandleFunc("GET /favicon.svg", s.handleFavicon)
	m.HandleFunc("GET /favicon.png", s.handleFaviconPNG)
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	return withLocalPprof(s.withAccessLog(s.withGzip(s.withAuth(s.withRecovery(s.mux)))))
}

// withRecovery converts a handler panic into a 500 instead of dropping the
// connection, and logs the stack so the failure is diagnosable.
func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			s.logger.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, rec, debug.Stack())
			writeError(w, http.StatusInternalServerError, "Internal server error")
		}()
		next.ServeHTTP(w, r)
	})
}

// withGzip compresses JSON/HTML/text responses when the client accepts it.
// Streams and artwork bypass compression (already-compressed or binary).
// The encoding header is applied lazily on first write so bodiless
// responses (304) are not advertised as gzipped.
func (s *Server) withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		path := r.URL.Path
		if strings.HasPrefix(path, "/stream/") ||
			strings.HasPrefix(path, "/artwork/") ||
			strings.HasPrefix(path, "/subtitles/") ||
			path == "/api/library" || path == "/library.json" ||
			path == "/" || path == "/audiobooks" || path == "/ebooks" ||
			path == "/shared.js" || path == "/library.css" || path == "/library.js" ||
			path == "/favicon.svg" || path == "/favicon.png" {
			// These routes manage their own cached gzip.
			next.ServeHTTP(w, r)
			return
		}
		gz := newGzipWriter(w)
		defer gz.Close()
		next.ServeHTTP(gz, r)
	})
}

// acceptsGzip reports whether the request's Accept-Encoding allows gzip,
// honouring an explicit q=0 ("gzip;q=0" means gzip is not acceptable).
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		q := 1.0
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if v, ok := strings.CutPrefix(p, "q="); ok {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = parsed
				}
			}
		}
		return q > 0
	}
	return false
}

// etagMatches reports whether an If-None-Match header matches etag, handling
// comma-separated lists, "*", and the weak "W/" prefix. etag is compared
// verbatim (including its quotes).
func etagMatches(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	for _, part := range strings.Split(ifNoneMatch, ",") {
		cand := strings.TrimSpace(part)
		cand = strings.TrimPrefix(cand, "W/")
		if cand == etag {
			return true
		}
	}
	return false
}

// --- middleware ---

func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path != "/api/health" {
			s.logger.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// ReadFrom forwards to the underlying writer's ReadFrom so http.ServeContent
// can use sendfile for direct-play streams. Embedding the ResponseWriter
// interface alone would hide it and force a userspace copy.
func (w *statusWriter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}

// Flush forwards to the underlying Flusher when available.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// peerHost extracts the connection peer address, never trusting Host headers.
func peerHost(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostIsLoopback reports whether the request's Host header names a loopback
// address. The peer address alone cannot be trusted for the auth bypass: a
// browser on the machine can be tricked by DNS rebinding into sending requests
// to 127.0.0.1 with an attacker-controlled Host header. Requiring the Host to
// be loopback as well closes that path while leaving native local clients and
// direct http://127.0.0.1 / http://localhost use working.
func hostIsLoopback(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return isLoopback(host)
}

// withAuth implements API.md auth: loopback bypass, LAN gate, Bearer/?token=.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The browser requests these immutable UI assets separately after it
		// loads the token-bearing page URL. Browsers do not copy the page query
		// string onto stylesheet, script, or favicon requests. The assets contain
		// no catalog, settings, or media data; the page and all API routes remain
		// protected below.
		if isPublicWebAsset(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// Audiobookshelf/BookPlayer compatibility endpoints intentionally have no
		// authentication. They are a local-LAN media feed, not the Sonder admin API.
		if isAudiobookshelfPath(r.URL.Path) || isJellyfinPath(r.URL.Path) || hasJellyfinCredentials(r) {
			if isLoopback(peerHost(r)) && hostIsLoopback(r.Host) {
				next.ServeHTTP(w, r)
				return
			}
			if !s.cfg().AllowLAN {
				writeError(w, http.StatusForbidden, "LAN access disabled")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if isLoopback(peerHost(r)) && hostIsLoopback(r.Host) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.cfg().AllowLAN {
			writeError(w, http.StatusForbidden, "LAN access disabled")
			return
		}
		if s.cfg().PairingToken != "" && !s.tokenMatches(r) {
			writeError(w, http.StatusUnauthorized, "Invalid or missing pairing token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAudiobookshelfPath(path string) bool {
	return path == "/ping" || path == "/status" || path == "/login" ||
		path == "/api/libraries" || strings.HasPrefix(path, "/api/libraries/") ||
		strings.HasPrefix(path, "/api/items/")
}

func isJellyfinPath(path string) bool {
	return path == "/System/Info/Public" || path == "/QuickConnect/Enabled" ||
		path == "/Users/AuthenticateByName" || path == "/UserViews" || path == "/Items" ||
		strings.HasPrefix(path, "/Users/") || strings.HasPrefix(path, "/Items/") ||
		strings.HasPrefix(path, "/Artists/") || path == "/Persons"
}

func isPublicWebAsset(path string) bool {
	switch path {
	case "/shared.js", "/library.css", "/library.js", "/favicon.svg", "/favicon.png":
		return true
	default:
		return false
	}
}

func hasJellyfinCredentials(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(strings.ToLower(auth), "mediabrowser ") ||
		r.URL.Query().Get("api_key") == jellyfinToken
}

// tokenMatches reports whether the request carries the configured pairing
// token, via `Authorization: Bearer <token>` or `?token=`. Comparison is
// constant-time so the token cannot be recovered by timing responses.
func (s *Server) tokenMatches(r *http.Request) bool {
	want := s.cfg().PairingToken
	if want == "" {
		return false
	}
	var got string
	if auth := r.Header.Get("Authorization"); len(auth) >= 7 && strings.EqualFold(auth[:7], "bearer ") {
		got = strings.TrimSpace(auth[7:])
	} else {
		got = r.URL.Query().Get("token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- shared response pieces ---

func (s *Server) requiresPairing() bool {
	return s.cfg().AllowLAN && s.cfg().PairingToken != ""
}

// lanURL discovers the first non-loopback IPv4 address when LAN is enabled.
func (s *Server) lanURL() *string {
	if !s.cfg().AllowLAN {
		return nil
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			u := "http://" + ipnet.IP.String() + ":" + strconv.Itoa(s.cfg().Port)
			return &u
		}
	}
	return nil
}
