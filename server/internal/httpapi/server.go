// Package httpapi serves the TM Sonder wire contract documented in API.md.
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/transcode"
)

const (
	AppName    = "TM Sonder"
	ServerID   = "tm-sonder"
	ServiceDNS = "_tmsonder._tcp"
	Version    = "0.1.0"
	Build      = "go-port"
)

// TrackRefresher re-probes one item's tracks (refresh-tracks endpoint).
type TrackRefresher interface {
	RefreshTracks(itemID string) error
}

// ChapterProvider supplies audiobook chapter markers for the detail route.
type ChapterProvider interface {
	ChaptersFor(itemID string) ([]api.AudiobookChapter, error)
}

// Server wires the catalog, scanner, and transcoder into the HTTP contract.
type Server struct {
	cfg        *config.Config
	store      *library.Store
	scanner    *library.Scanner
	tm         *transcode.Manager
	refresher  TrackRefresher
	chapters   ChapterProvider
	onMutation func()

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
		cfg:     cfg,
		store:   store,
		scanner: scanner,
		tm:      tm,
		logger:  log.New(log.Writer(), "sonder-http ", log.LstdFlags),
	}
	s.mux = http.NewServeMux()
	s.routes()
	return s
}

// SetTrackRefresher wires optional probe-backed track refresh.
func (s *Server) SetTrackRefresher(r TrackRefresher) { s.refresher = r }

// SetChapterProvider wires optional audiobook chapter extraction.
func (s *Server) SetChapterProvider(p ChapterProvider) { s.chapters = p }

// SetAutoSave registers a callback fired after catalog mutations so main can
// debounce snapshot writes.
func (s *Server) SetAutoSave(fn func()) { s.onMutation = fn }

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
	data, err := json.MarshalIndent(s.cfg, "", "  ")
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
	if s.cfg.DataDir == "" {
		return
	}
	p := filepath.Join(s.cfg.DataDir, "pairing-token")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(token+"\n"), 0o600)
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /api/health", s.handleHealth)
	m.HandleFunc("GET /api/discovery", s.handleDiscovery)
	m.HandleFunc("GET /api/library", s.handleLibrary)
	m.HandleFunc("GET /library.json", s.handleLibrary)
	m.HandleFunc("GET /api/status", s.handleStatus)
	m.HandleFunc("GET /api/playback/{id}", s.handlePlaybackGet)
	m.HandleFunc("POST /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("PATCH /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("PUT /api/playback/{id}", s.handlePlaybackUpdate)
	m.HandleFunc("POST /api/playback/{id}/refresh-tracks", s.handleRefreshTracks)
	m.HandleFunc("POST /api/progress/{id}", s.handleProgressUpdate)
	m.HandleFunc("GET /stream/{id}", s.handleStream)
	m.HandleFunc("GET /subtitles/{id}/{index}", s.handleSubtitle)
	m.HandleFunc("GET /artwork/poster/{id}", s.handlePoster)
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
}

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	return withLocalPprof(s.withAccessLog(s.withGzip(s.withAuth(s.mux))))
}

// withGzip compresses JSON/HTML/text responses when the client accepts it.
// Streams and artwork bypass compression (already-compressed or binary).
// The encoding header is applied lazily on first write so bodiless
// responses (304) are not advertised as gzipped.
func (s *Server) withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if strings.HasPrefix(path, "/stream/") ||
			strings.HasPrefix(path, "/artwork/") ||
			strings.HasPrefix(path, "/subtitles/") ||
			path == "/api/library" || path == "/library.json" ||
			path == "/" || path == "/audiobooks" {
			// /api/library and the HTML pages manage their own cached gzip.
			next.ServeHTTP(w, r)
			return
		}
		gz := newGzipWriter(w)
		defer gz.Close()
		next.ServeHTTP(gz, r)
	})
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

// withAuth implements API.md auth: loopback bypass, LAN gate, Bearer/?token=.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer := peerHost(r)
		if isLoopback(peer) {
			next.ServeHTTP(w, r)
			return
		}
		if !s.cfg.AllowLAN {
			writeError(w, http.StatusForbidden, "LAN access disabled")
			return
		}
		if s.cfg.PairingToken != "" && !s.tokenMatches(r) {
			writeError(w, http.StatusUnauthorized, "Invalid or missing pairing token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenMatches(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") &&
		strings.TrimSpace(auth[7:]) == s.cfg.PairingToken {
		return true
	}
	return r.URL.Query().Get("token") == s.cfg.PairingToken
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
	return s.cfg.AllowLAN && s.cfg.PairingToken != ""
}

// lanURL discovers the first non-loopback IPv4 address when LAN is enabled.
func (s *Server) lanURL() *string {
	if !s.cfg.AllowLAN {
		return nil
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			u := "http://" + ipnet.IP.String() + ":" + itoa(s.cfg.Port)
			return &u
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
