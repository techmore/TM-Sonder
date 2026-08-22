package httpapi

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tm-sonder/server/internal/library"
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
	st, err := os.Stat(item.FilePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "Media file missing")
		return
	}

	if r.URL.Query().Get("transcode") == "1" {
		s.streamTranscode(w, r, item)
		return
	}

	f, err := os.Open(item.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot open media")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", item.Format.ContentType())
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(item.FilePath), st.ModTime(), f)
}

// streamTranscode pipes a live fMP4 session to the client. Range semantics
// do not apply; the fragmented container supports player-side seeking.
func (s *Server) streamTranscode(w http.ResponseWriter, r *http.Request, item *library.Item) {
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

	reader, cleanup, err := s.tm.Attach(r.Context(), item, mode, start, burnSub)
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
	s.serveArtwork(w, r, item.PosterPath)
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
	serveGzippableHTML(w, r, libraryPage)
}
