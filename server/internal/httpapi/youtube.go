package httpapi

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
	"tm-sonder/server/internal/youtube"
)

func (s *Server) SetYouTubeManager(m *youtube.Manager) { s.youtubeManager = m }
func (s *Server) handleYouTube(w http.ResponseWriter, r *http.Request) {
	if !s.requireSubtitleOwner(w, r) {
		return
	}
	if s.youtubeManager == nil {
		writeError(w, http.StatusServiceUnavailable, "YouTube subscriptions are unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.youtubeManager.Status())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var p struct {
		Action        string           `json:"action"`
		URL           string           `json:"url"`
		ID            string           `json:"id"`
		IntervalHours int              `json:"intervalHours"`
		Backfill      int              `json:"backfill"`
		Settings      youtube.Settings `json:"settings"`
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid subscription action")
		return
	}
	var err error
	switch p.Action {
	case "settings":
		err = s.youtubeManager.Configure(p.Settings)
	case "add":
		_, err = s.youtubeManager.Add(p.URL, p.IntervalHours, p.Backfill)
	case "checkAll":
		err = s.youtubeManager.CheckAll()
	case "channelPause", "channelResume", "channelCheck", "channelInterval":
		actions := map[string]string{"channelPause": "pause", "channelResume": "resume", "channelCheck": "check", "channelInterval": "interval"}
		err = s.youtubeManager.ChannelAction(p.ID, actions[p.Action], p.IntervalHours)
	case "jobRetry", "jobCancel":
		action := "retry"
		if p.Action == "jobCancel" {
			action = "cancel"
		}
		err = s.youtubeManager.JobAction(p.ID, action)
	default:
		writeError(w, http.StatusBadRequest, "Unknown subscription action")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"saved": true})
}

// Coalesce download completions into a scan; never overlap an existing scan.
func (s *Server) QueueYouTubeCatalogScan() {
	if s.scanner == nil || !s.youtubeScanPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.youtubeScanPending.Store(false)
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(time.Minute)
			if s.scanner.State().Scanning {
				continue
			}
			if _, err := s.scanner.ScanAll(s.cfg().Libraries); err == nil {
				if s.snapshotPath != "" {
					_ = s.store.Flush(s.snapshotPath)
				}
				atomic.AddInt64(&s.settingsVersion, 1)
				return
			}
		}
	}()
}

var youtubeJS = newGzippedPage(func() []byte { return mustReadWeb("web/youtube.js") })
var youtubePage = newGzippedPage(func() []byte { return mustReadWeb("web/youtube.html") })

func (s *Server) handleYouTubeJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, youtubeJS, "text/javascript; charset=utf-8")
}
func (s *Server) handleYouTubePage(w http.ResponseWriter, r *http.Request) {
	if !s.requireSubtitleOwner(w, r) {
		return
	}
	serveGzippableHTML(w, r, youtubePage)
}
