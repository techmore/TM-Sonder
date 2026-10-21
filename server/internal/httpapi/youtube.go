package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	case "channelPause", "channelResume", "channelCheck", "channelInterval", "channelRemove":
		actions := map[string]string{"channelPause": "pause", "channelResume": "resume", "channelCheck": "check", "channelInterval": "interval", "channelRemove": "remove"}
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
	if s.scanner == nil {
		return
	}
	s.youtubeScanDirty.Store(true)
	if !s.youtubeScanPending.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer func() {
			s.youtubeScanPending.Store(false)
			if s.youtubeScanDirty.Load() {
				s.QueueYouTubeCatalogScan()
			}
		}()
		for attempt := 0; attempt < 10; attempt++ {
			time.Sleep(time.Minute)
			if s.scanner.State().Scanning {
				continue
			}
			s.youtubeScanDirty.Store(false)
			if _, err := s.scanner.ScanAll(s.cfg().Libraries); err == nil {
				if s.snapshotPath != "" {
					_ = s.store.Flush(s.snapshotPath)
				}
				atomic.AddInt64(&s.settingsVersion, 1)
				if !s.youtubeScanDirty.Load() {
					return
				}
			}
			s.youtubeScanDirty.Store(true)
		}
	}()
}

var youtubeJS = newGzippedPage(func() []byte { return mustReadWeb("web/youtube.js") })
var youtubePage = newGzippedPage(func() []byte { return mustReadWeb("web/youtube.html") })

func (s *Server) handleYouTubeJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, youtubeJS, "text/javascript; charset=utf-8")
}

// handleYouTubePage is a browser page, so a visitor who is not the owner gets
// a browser answer: anonymous sessions go to sign-in (like the other
// protected pages), and signed-in non-owners get a short HTML explanation.
// Both used to receive the API's raw JSON error body.
func (s *Server) handleYouTubePage(w http.ResponseWriter, r *http.Request) {
	if !s.isRuntimeOwner(r) {
		if _, signedIn := s.sessionUsername(r); !signedIn {
			setIdentityHeaders(w)
			http.Redirect(w, r, "/account/login?next="+url.QueryEscape("/youtube"), http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(ownerOnlyPageHTML))
		return
	}
	serveGzippableHTML(w, r, youtubePage)
}

// ownerOnlyPageHTML is the HTML reply for signed-in accounts that reach an
// owner-only page. It carries no user data, so it is a plain constant.
const ownerOnlyPageHTML = `<!doctype html>
<html lang="en" data-theme="earthy"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Owner only · TM Sonder</title><link rel="icon" href="/favicon.png"><link rel="stylesheet" href="/library.css">
<style>body{margin:0;background:var(--bg,#111);color:var(--text,#eee);font:15px system-ui}main{max-width:560px;margin:12vh auto;padding:24px}h1{font-size:22px;margin:0 0 10px}p{color:var(--muted,#aaa);line-height:1.5}a{color:inherit}</style></head>
<body><main><h1>This page is for the library owner</h1><p>YouTube subscriptions are managed by the account that set up this server. Ask the owner if you need a channel added.</p><p><a href="/">Back to the library</a></p></main></body></html>
`

var youtubeLinkJS = newGzippedPage(func() []byte { return mustReadWeb("web/youtube-link.js") })

func (s *Server) handleYouTubeLinkJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, youtubeLinkJS, "text/javascript; charset=utf-8")
}
func (s *Server) handleYouTubePermission(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.requireSubtitleOwner(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"owner": true})
}
