package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/transcode"
)

func (s *Server) streamHLS(w http.ResponseWriter, r *http.Request, item *library.Item, start float64, sub, audio int, release func()) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	session, err := s.tm.StartHLS(ctx, filepath.Join(s.cfg().DataDir, "hls-runtime"), item.ID, r.URL.Query().Get("playback"), transcode.Request{Path: item.FilePath, StartSeconds: start, BurnSubtitleN: sub, AudioTrackN: audio}, release)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Video encoder busy; retry playback")
		return
	}
	location := "/stream/" + url.PathEscape(item.ID) + "/hls/" + session.ID + "/index.m3u8"
	if token := r.URL.Query().Get("token"); token != "" {
		location += "?token=" + url.QueryEscape(token)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, location, http.StatusTemporaryRedirect)
}

func (s *Server) handleHLSAsset(w http.ResponseWriter, r *http.Request) {
	session, ok := s.tm.HLS(r.PathValue("session"), r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusGone, "Playback session expired; resume to reconnect")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 22*time.Second)
	defer cancel()
	name := r.PathValue("asset")
	data, err := session.ReadAsset(ctx, name)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Video segment unavailable; resume to reconnect")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if name == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		lines := strings.Split(string(data), "\n")
		token := r.URL.Query().Get("token")
		for i, line := range lines {
			if line != "" && !strings.HasPrefix(line, "#") && token != "" {
				lines[i] = line + "?token=" + url.QueryEscape(token)
			}
		}
		lines = append(lines[:1], append([]string{"#EXT-X-START:TIME-OFFSET=0,PRECISE=YES"}, lines[1:]...)...)
		data = []byte(strings.Join(lines, "\n"))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
