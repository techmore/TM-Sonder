package httpapi

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/transcode"
)

func (s *Server) movieVODItem(w http.ResponseWriter, r *http.Request) (*library.Item, string, bool) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok || item.Kind != api.KindMovie {
		writeError(w, http.StatusNotFound, "Movie not found")
		return nil, "", false
	}
	st, err := mediaInfo(item)
	if err != nil {
		writeError(w, http.StatusNotFound, "Movie source is unavailable")
		return nil, "", false
	}
	version := fmt.Sprintf("%s|%d|%d", item.FilePath, st.Size(), st.ModTime().UnixNano())
	return item, version, true
}
func (s *Server) vodRoot() string { return filepath.Join(s.cfg().DataDir, "movie-vod-cache") }
func (s *Server) writeVODStatus(w http.ResponseWriter, r *http.Request, item *library.Item, version string, statusCode int) {
	key := transcode.VODKey(item.ID, version)
	state := transcode.VODStatus{Status: "missing", CacheKey: key, ItemID: item.ID}
	if asset := s.tm.VOD(s.vodRoot(), item.ID, key); asset != nil {
		state = asset.Snapshot()
	}
	result := map[string]any{"status": state.Status, "phase": state.Phase, "progress": state.Progress, "error": state.Error, "cacheKey": key, "itemID": item.ID, "bytes": state.Bytes, "cacheLimitBytes": transcode.VODLimit()}
	if state.Status == "ready" {
		base := "/stream/" + url.PathEscape(item.ID) + "/vod/" + key + "/"
		result["playlistURL"] = base + "index.m3u8"
		result["downloadURL"] = base + "download.mp4"
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, statusCode, result)
}
func (s *Server) handleMoviePreparation(w http.ResponseWriter, r *http.Request) {
	item, version, ok := s.movieVODItem(w, r)
	if ok {
		s.writeVODStatus(w, r, item, version, http.StatusOK)
	}
}
func (s *Server) handleMoviePrepare(w http.ResponseWriter, r *http.Request) {
	item, version, ok := s.movieVODItem(w, r)
	if !ok {
		return
	}
	st, err := mediaInfo(item)
	if err != nil {
		writeError(w, http.StatusNotFound, "Movie source unavailable")
		return
	}
	cached, release := s.cacheItem(item, st)
	_, err = s.tm.PrepareVOD(s.vodRoot(), item.ID, version, transcode.Request{Path: cached.FilePath, BurnSubtitleN: -1, AudioTrackN: 0}, item.DurationSeconds, release)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.writeVODStatus(w, r, item, version, http.StatusAccepted)
}
func (s *Server) handleMovieVODAsset(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok || item.Kind != api.KindMovie {
		writeError(w, http.StatusNotFound, "Movie not found")
		return
	}
	asset := s.tm.VOD(s.vodRoot(), item.ID, r.PathValue("cache"))
	if asset == nil {
		writeError(w, http.StatusGone, "Prepared movie is no longer cached; prepare it again")
		return
	}
	name := r.PathValue("asset")
	file, err := asset.Open(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "Prepared movie file unavailable")
		return
	}
	defer file.Close()
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	w.Header().Set("Vary", "Cookie, Authorization")
	if strings.HasSuffix(name, ".m3u8") {
		data, err := io.ReadAll(io.LimitReader(file, 2<<20))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Cannot read movie playlist")
			return
		}
		lines := strings.Split(string(data), "\n")
		if token := r.URL.Query().Get("token"); token != "" {
			for i, line := range lines {
				if line != "" && !strings.HasPrefix(line, "#") {
					lines[i] = line + "?token=" + url.QueryEscape(token)
				}
			}
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write([]byte(strings.Join(lines, "\n")))
		return
	}
	if name == "download.mp4" {
		w.Header().Set("Content-Type", "video/mp4")
		if r.URL.Query().Get("inline") != "1" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": item.Title + ".mp4"}))
		}
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	st, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot read prepared movie")
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), file)
}
