package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/mediacache"
	"tm-sonder/server/internal/transcode"
)

// handleStream serves /stream/{id}: kernel-fast ServeContent for direct play,
// or an fMP4 transcode session when ?transcode=1.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	st, err := mediaInfo(item)
	if err != nil {
		writeError(w, http.StatusNotFound, "Media file missing")
		return
	}

	if r.URL.Query().Get("transcode") == "1" {
		s.streamTranscode(w, r, item, st)
		return
	}

	f, sourceInfo, release, err := s.openMedia(item, st)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "Media file missing")
		} else {
			writeError(w, http.StatusInternalServerError, "Cannot open media")
		}
		return
	}
	defer release()
	defer f.Close()
	w.Header().Set("Content-Type", item.Format.ContentType())
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(item.FilePath), sourceInfo.ModTime(), f)
}

// streamTranscode pipes a live fMP4 session to the client. Range semantics
// do not apply; the fragmented container supports player-side seeking.
func (s *Server) streamTranscode(w http.ResponseWriter, r *http.Request, item *library.Item, st os.FileInfo) {
	q := r.URL.Query()
	mode := transcode.Mode(q.Get("mode"))
	if mode == "" {
		mode = transcode.ModeAuto
	}
	start := 0.0
	if ss := q.Get("ss"); ss != "" {
		if v, err := strconv.ParseFloat(ss, 64); err == nil && v > 0 {
			start = v
		}
	}
	burnSub := -1
	if sub := q.Get("sub"); sub != "" {
		if n, err := strconv.Atoi(sub); err == nil && n >= 0 {
			burnSub = n
		}
	}
	audioTrack := -1
	if a := q.Get("audio"); a != "" {
		if n, err := strconv.Atoi(a); err == nil && n >= 0 {
			audioTrack = n
		}
	}

	streamItem, release := s.cacheItem(item, st)
	defer release()
	reader, cleanup, err := s.tm.Attach(r.Context(), streamItem, mode, start, burnSub, audioTrack)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Transcode failed to start")
		return
	}
	defer cleanup()

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (s *Server) cacheMedia(item *library.Item, st os.FileInfo) (string, func(), bool) {
	if s.mediaCache == nil {
		return item.FilePath, func() {}, false
	}
	return s.mediaCache.Acquire(mediacache.Media{
		ID:         item.ID,
		SourcePath: item.FilePath,
		Kind:       item.Kind,
		Year:       item.Year,
		Size:       st.Size(),
		ModTime:    st.ModTime(),
	})
}

func (s *Server) cacheItem(item *library.Item, st os.FileInfo) (*library.Item, func()) {
	path, release, hit := s.cacheMedia(item, st)
	if !hit {
		return item, release
	}
	copy := *item
	copy.FilePath = path
	return &copy, release
}

func (s *Server) openMedia(item *library.Item, st os.FileInfo) (*os.File, os.FileInfo, func(), error) {
	path, release, hit := s.cacheMedia(item, st)
	f, err := os.Open(path)
	if err == nil {
		return f, st, release, nil
	}
	if hit {
		// A cache file can disappear between Acquire and Open (manual cleanup,
		// disk pressure, or a partial filesystem failure). Fall back to the NAS
		// source for this request rather than turning a valid item into a 500.
		release()
		f, err = os.Open(item.FilePath)
		if err == nil {
			return f, st, func() {}, nil
		}
	}
	return nil, st, func() {}, err
}

// mediaInfo prefers the current source stat, but falls back to the catalog's
// last known size and modification time. That lets a completed local cache
// continue serving while a NAS mount is temporarily unavailable.
func mediaInfo(item *library.Item) (os.FileInfo, error) {
	if st, err := os.Stat(item.FilePath); err == nil {
		return st, nil
	}
	if item.SizeBytes <= 0 || item.ModTime.IsZero() {
		return nil, os.ErrNotExist
	}
	return catalogFileInfo{name: filepath.Base(item.FilePath), size: item.SizeBytes, modTime: item.ModTime}, nil
}

type catalogFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i catalogFileInfo) Name() string       { return i.name }
func (i catalogFileInfo) Size() int64        { return i.size }
func (i catalogFileInfo) Mode() os.FileMode  { return 0o600 }
func (i catalogFileInfo) ModTime() time.Time { return i.modTime }
func (i catalogFileInfo) IsDir() bool        { return false }
func (i catalogFileInfo) Sys() any           { return nil }

// handleSubtitle serves GET /subtitles/{id}/{index} from sidecar paths,
// matching the Swift server's content types.
func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Subtitle not found")
		return
	}
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || idx < 0 || idx >= len(item.SidecarPaths) {
		writeError(w, http.StatusNotFound, "Subtitle not found")
		return
	}
	data, err := os.ReadFile(item.SidecarPaths[idx])
	if err != nil {
		writeError(w, http.StatusNotFound, "Subtitle not found")
		return
	}
	ct := "application/x-subrip; charset=utf-8"
	if strings.EqualFold(filepath.Ext(item.SidecarPaths[idx]), ".vtt") {
		ct = "text/vtt; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveArtwork resolves and streams one artwork file with image content type.
func (s *Server) serveArtwork(w http.ResponseWriter, r *http.Request, path string) {
	if path == "" {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "png":
		w.Header().Set("Content-Type", "image/png")
	case "webp":
		w.Header().Set("Content-Type", "image/webp")
	case "gif":
		w.Header().Set("Content-Type", "image/gif")
	case "avif":
		w.Header().Set("Content-Type", "image/avif")
	case "heic", "heif":
		w.Header().Set("Content-Type", "image/heic")
	default:
		w.Header().Set("Content-Type", "image/jpeg")
	}
	// Posters are content-stable per item version; let clients cache a day.
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Server) handlePoster(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}

	// Catalogs imported from another host can retain the old absolute
	// artwork path while the image itself was copied into this data dir.
	// Prefer the recorded path, then resolve the standard generated-artwork
	// location by stable item ID.
	path := item.PosterPath
	if path == "" || !fileExists(path) {
		path = filepath.Join(s.cfg().DataDir, "artwork", item.ID+".jpg")
	}
	if fileExists(path) {
		s.serveArtwork(w, r, path)
		return
	}
	if item.Kind == "movie" {
		s.serveMoviePlaceholder(w, r, item)
		return
	}
	writeError(w, http.StatusNotFound, "Artwork not found")
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (s *Server) serveMoviePlaceholder(w http.ResponseWriter, r *http.Request, item *library.Item) {
	initial := "?"
	for _, ch := range strings.TrimSpace(item.Title) {
		initial = strings.ToUpper(string(ch))
		break
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 450"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#1b2520"/><stop offset="1" stop-color="#536a59"/></linearGradient></defs><rect width="300" height="450" fill="url(#g)"/><circle cx="150" cy="165" r="64" fill="#d7e3d7" opacity=".18"/><text x="150" y="190" text-anchor="middle" font-family="Arial,sans-serif" font-size="72" font-weight="700" fill="#f2f5ed">` + initial + `</text></svg>`))
}

func (s *Server) handleBackdrop(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}
	s.serveArtwork(w, r, item.BackdropPath)
}

// handleIndex serves the library browser (port of SonderWebInterface.html).
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	serveGzippableHTML(w, r, libraryPageForThemeAndLayout(s.cfg().ThemePreset, s.cfg().LibraryLayout))
}
