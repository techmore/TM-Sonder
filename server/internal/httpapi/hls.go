package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/transcode"
)

// Older pages request only ?transcode=1. Apple players need HLS even when
// the page did not supply a delivery hint or its MIME capability check failed.
func useNativeHLS(r *http.Request) bool {
	if delivery := r.URL.Query().Get("delivery"); delivery != "" {
		return delivery == "hls"
	}
	ua := r.UserAgent()
	return strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") ||
		strings.Contains(ua, "iPod") || (strings.Contains(ua, "Macintosh") &&
		strings.Contains(ua, "Safari/") && !strings.Contains(ua, "Chrome/") &&
		!strings.Contains(ua, "Chromium/") && !strings.Contains(ua, "Edg/"))
}

func hlsPlaybackIdentity(r *http.Request, itemID string, start float64, sub, audio int) (string, string) {
	q := r.URL.Query()
	owner, generation := q.Get("playback"), q.Get("generation")
	if len(owner) >= 20 && len(owner) <= 100 && generation != "" {
		return owner, generation
	}
	// Safari can probe the same URL more than once. Legacy pages lack a client
	// ID, so scope probe reuse to the authenticated browser and movie. Hash
	// credentials rather than retaining them in the transcode registry.
	credential := q.Get("token")
	if credential == "" {
		credential = r.Header.Get("Authorization")
	}
	if credential == "" {
		credential = r.Header.Get("Cookie")
	}
	if credential == "" {
		credential, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	identity := sha256.Sum256([]byte(credential + "\x00" + r.UserAgent() + "\x00" + itemID))
	return fmt.Sprintf("legacy-%x", identity), fmt.Sprintf("%.3f|%d|%d", start, sub, audio)
}

func (s *Server) streamHLS(w http.ResponseWriter, r *http.Request, item *library.Item, start float64, sub, audio int, release func()) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	owner, generation := hlsPlaybackIdentity(r, item.ID, start, sub, audio)
	session, err := s.tm.StartHLS(ctx, filepath.Join(s.cfg().DataDir, "hls-runtime"), item.ID, owner, generation, transcode.Request{Path: item.FilePath, StartSeconds: start, BurnSubtitleN: sub, AudioTrackN: audio}, release)
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
