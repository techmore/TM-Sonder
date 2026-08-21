package httpapi

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

type fixture struct {
	s     *Server
	store *library.Store
	ts    *httptest.Server
}

func newFixture(t *testing.T, mutate func(*config.Config)) *fixture {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	if mutate != nil {
		mutate(&cfg)
	}
	store := library.New()
	sc := library.NewScanner(store)
	f := &fixture{s: New(&cfg, store, sc, nil), store: store}
	f.ts = httptest.NewServer(f.s.Handler())
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fixture) addItem(t *testing.T, id, title string) *library.Item {
	t.Helper()
	p := filepath.Join(t.TempDir(), title+".mp4")
	if err := os.WriteFile(p, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	it := &library.Item{
		MediaItem: api.MediaItem{
			ID: id, Title: title, Kind: api.KindMovie,
			Format: api.FormatMP4, Tags: []string{},
			DurationSeconds: 100,
		},
		FilePath: p,
	}
	f.store.Upsert(it)
	return it
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	return resp, body
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer gz.Close()
		r = gz
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestHealthShape(t *testing.T) {
	f := newFixture(t, nil)
	resp, body := get(t, f.ts.URL+"/api/health")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var h api.HealthResponse
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	if h.Status != "ok" || h.ID != "tm-sonder" || h.Service != "_tmsonder._tcp" ||
		h.Library != "/api/library" || h.AllowLAN || h.RequiresPairing {
		t.Errorf("health shape wrong: %+v", h)
	}
}

func TestDiscoveryShape(t *testing.T) {
	f := newFixture(t, nil)
	resp, body := get(t, f.ts.URL+"/api/discovery")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var d map[string]any
	json.Unmarshal([]byte(body), &d)
	caps := d["capabilities"].(map[string]any)
	for _, k := range []string{"books", "audiobooks", "themes", "progressSync", "mediaStreaming", "artwork", "librarySync"} {
		if caps[k] != true {
			t.Errorf("capability %s not true: %v", k, caps[k])
		}
	}
	eps := d["endpoints"].(map[string]any)
	if eps["stream"] != "/stream/{id}" || eps["poster"] != "/artwork/poster/{id}" {
		t.Errorf("endpoints wrong: %v", eps)
	}
	if d["localURL"] == "" || d["lanURL"] != nil {
		t.Errorf("urls wrong: local=%v lan=%v", d["localURL"], d["lanURL"])
	}
}

func TestLibraryETagAnd304(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "i1", "Film")

	resp1, _ := get(t, f.ts.URL+"/api/library")
	etag := resp1.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	if cc := resp1.Header.Get("Cache-Control"); !strings.Contains(cc, "must-revalidate") {
		t.Errorf("cache-control = %q", cc)
	}

	req, _ := http.NewRequest("GET", f.ts.URL+"/api/library", nil)
	req.Header.Set("If-None-Match", etag)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, resp2)
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("want 304, got %d", resp2.StatusCode)
	}

	// Mutation bumps generation -> 200 again.
	f.store.RecordActivity("x", "", "")
	req3, _ := http.NewRequest("GET", f.ts.URL+"/api/library", nil)
	req3.Header.Set("If-None-Match", etag)
	resp3, _ := http.DefaultClient.Do(req3)
	readAll(t, resp3)
	if resp3.StatusCode != 200 {
		t.Errorf("after mutation want 200, got %d", resp3.StatusCode)
	}
}

func TestLibraryHidesFilesystemPaths(t *testing.T) {
	f := newFixture(t, nil)
	it := f.addItem(t, "i1", "Secret Film")
	body := getBody(t, f.ts.URL+"/api/library")
	if strings.Contains(body, it.FilePath) || strings.Contains(body, "/var/folders") ||
		strings.Contains(body, "pairingToken") {
		t.Error("library leaks filesystem paths or tokens")
	}
}

func TestPlaybackDefaultsAndMerge(t *testing.T) {
	f := newFixture(t, nil)
	it := f.addItem(t, "ep1", "Show")
	it.EmbeddedAudioTracks = []api.PlaybackTrack{
		{ID: "embedded-audio:0", Label: "English", Kind: api.TrackEmbedded, LanguageCode: strPtr("en")},
		{ID: "embedded-audio:1", Label: "Japanese", Kind: api.TrackEmbedded, LanguageCode: strPtr("ja")},
	}
	it.EmbeddedSubtitleTracks = []api.PlaybackTrack{
		{ID: "embedded-subtitle:0", Label: "English", Kind: api.TrackEmbedded, LanguageCode: strPtr("en")},
	}
	f.store.Upsert(it)

	// GET returns defaults: ja audio + en subs enabled.
	var sess api.PlaybackSession
	json.Unmarshal([]byte(getBody(t, f.ts.URL+"/api/playback/ep1")), &sess)
	if sess.AudioTrackID == nil || *sess.AudioTrackID != "embedded-audio:1" {
		t.Errorf("default audio = %v", sess.AudioTrackID)
	}
	if sess.SubtitleTrackID == nil || *sess.SubtitleTrackID != "embedded-subtitle:0" || sess.SubtitlesEnabled == nil || !*sess.SubtitlesEnabled {
		t.Errorf("default subs wrong: %v %v", sess.SubtitleTrackID, sess.SubtitlesEnabled)
	}
	if len(sess.SubtitleTracks) != 1 || sess.SubtitleTracks[0].Kind != api.TrackEmbedded {
		t.Errorf("subtitle tracks wrong: %+v", sess.SubtitleTracks)
	}

	// POST progress; omitted track fields keep previous values.
	body := `{"seconds":42.5,"duration":100}`
	resp, err := http.Post(f.ts.URL+"/api/playback/ep1", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(readAll(t, resp)), &sess)
	if sess.Seconds != 42.5 || *sess.AudioTrackID != "embedded-audio:1" {
		t.Errorf("merge failed: %+v", sess)
	}
	p, _ := f.store.ProgressFor("ep1")
	if p.Seconds != 42.5 {
		t.Errorf("store seconds = %v", p.Seconds)
	}
}

func TestProgressRouteAcceptsUpdate(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "p1", "Movie")
	sub := false
	body, _ := json.Marshal(api.PlaybackStateUpdate{Seconds: 5, Duration: 10, SubtitlesEnabled: &sub})
	resp, err := http.Post(f.ts.URL+"/api/progress/p1", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	rec, ok := f.store.ProgressFor("p1")
	if !ok || rec.Seconds != 5 || rec.SubtitlesEnabled == nil || *rec.SubtitlesEnabled {
		t.Errorf("progress record wrong: %+v ok=%v", rec, ok)
	}
}

func TestStatusRoute(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "s1", "One")
	var m map[string]any
	json.Unmarshal([]byte(getBody(t, f.ts.URL+"/api/status")), &m)
	if m["itemCount"].(float64) != 1 {
		t.Errorf("itemCount = %v", m["itemCount"])
	}
}

func TestStreamDirectPlayWithRange(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "v1", "Video")
	req, _ := http.NewRequest("GET", f.ts.URL+"/stream/v1", nil)
	req.Header.Set("Range", "bytes=0-7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusPartialContent || len(body) != 8 {
		t.Errorf("range status=%d len=%d", resp.StatusCode, len(body))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("content-type = %q", ct)
	}
}

func TestStreamMissingItem404(t *testing.T) {
	f := newFixture(t, nil)
	resp, _ := get(t, f.ts.URL+"/stream/nope")
	if resp.StatusCode != 404 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestSubtitleAndArtworkRoutes(t *testing.T) {
	f := newFixture(t, nil)
	dir := t.TempDir()
	srt := filepath.Join(dir, "Ep.srt")
	os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nhi\n"), 0o600)
	poster := filepath.Join(dir, "poster.jpg")
	os.WriteFile(poster, []byte("\xff\xd8\xffjpeg"), 0o600)

	it := &library.Item{MediaItem: api.MediaItem{ID: "sub1", Title: "Ep", Format: api.FormatMP4}, SidecarPaths: []string{srt}, PosterPath: poster}
	f.store.Upsert(it)

	body := getBody(t, f.ts.URL+"/subtitles/sub1/0")
	if !strings.Contains(body, "00:00:01,000") {
		t.Errorf("srt body wrong: %q", body)
	}
	resp, _ := get(t, f.ts.URL+"/subtitles/sub1/9")
	if resp.StatusCode != 404 {
		t.Errorf("bad index status = %d", resp.StatusCode)
	}
	resp2, body2 := get(t, f.ts.URL+"/artwork/poster/sub1")
	if resp2.StatusCode != 200 || resp2.Header.Get("Content-Type") != "image/jpeg" || len(body2) == 0 {
		t.Errorf("poster wrong: %d %q", resp2.StatusCode, resp2.Header.Get("Content-Type"))
	}
}

func TestAudiobookRoutes(t *testing.T) {
	f := newFixture(t, nil)
	ab := &library.Item{MediaItem: api.MediaItem{
		ID: "ab1", Title: "Dune", Kind: api.KindAudiobook,
		Format: api.FormatM4B, DurationSeconds: 36000, Studio: "Someone",
	}}
	f.store.Upsert(ab)

	var list struct {
		Items []map[string]any `json:"items"`
		Count int              `json:"count"`
	}
	json.Unmarshal([]byte(getBody(t, f.ts.URL+"/api/audiobooks")), &list)
	if list.Count != 1 || len(list.Items) != 1 || list.Items[0]["title"] != "Dune" {
		t.Errorf("audiobook list wrong: %+v", list)
	}

	q := getBody(t, f.ts.URL+`/api/audiobooks?q=dune`)
	if !strings.Contains(q, "Dune") {
		t.Errorf("?q filter failed")
	}
	q2 := getBody(t, f.ts.URL+`/api/audiobooks?q=nope`)
	if strings.Contains(q2, `"count":1`) {
		t.Errorf("?q nope should filter out")
	}

	var detail struct {
		Item     map[string]any `json:"item"`
		Chapters []any          `json:"chapters"`
	}
	json.Unmarshal([]byte(getBody(t, f.ts.URL+"/api/audiobooks/ab1")), &detail)
	if detail.Item["id"] != "ab1" || detail.Chapters == nil {
		t.Errorf("detail wrong: %+v", detail)
	}
	resp, _ := get(t, f.ts.URL+"/api/audiobooks/movie1")
	if resp.StatusCode != 404 {
		t.Errorf("movie as audiobook should 404, got %d", resp.StatusCode)
	}
}

func TestAuthMatrix(t *testing.T) {
	token := "pair-me"
	newSrv := func(allowLAN bool) *fixture {
		return newFixture(t, func(c *config.Config) {
			c.AllowLAN = allowLAN
			c.PairingToken = token
		})
	}
	do := func(f *fixture, remote, hdr, query string) int {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.RemoteAddr = remote
		if query != "" {
			req.URL.RawQuery = "token=" + query
		}
		if hdr != "" {
			req.Header.Set("Authorization", "Bearer "+hdr)
		}
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("lan off rejects remote", func(t *testing.T) {
		f := newSrv(false)
		if got := do(f, "192.168.1.50:1234", "", ""); got != http.StatusForbidden {
			t.Errorf("status = %d, want 403", got)
		}
	})

	t.Run("lan on requires token", func(t *testing.T) {
		f := newSrv(true)
		if got := do(f, "192.168.1.50:1234", "", ""); got != http.StatusUnauthorized {
			t.Errorf("no-token status = %d, want 401", got)
		}
		if got := do(f, "192.168.1.50:1234", "", token); got != 200 {
			t.Errorf("query token status = %d", got)
		}
		if got := do(f, "192.168.1.50:1234", token, ""); got != 200 {
			t.Errorf("bearer status = %d", got)
		}
		if got := do(f, "192.168.1.50:1234", "wrong", "wrong"); got != http.StatusUnauthorized {
			t.Errorf("bad token status = %d, want 401", got)
		}
	})

	t.Run("loopback bypasses everything", func(t *testing.T) {
		f := newSrv(false)
		if got := do(f, "127.0.0.1:5555", "", ""); got != 200 {
			t.Errorf("v4 loopback status = %d", got)
		}
		if got := do(f, "[::1]:5555", "", ""); got != 200 {
			t.Errorf("v6 loopback status = %d", got)
		}
	})

	t.Run("peer addr not host header", func(t *testing.T) {
		f := newSrv(false)
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.RemoteAddr = "127.0.0.1:9999"
		req.Host = "evil.example.com"
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Errorf("host header spoof changed nothing? status=%d", rec.Code)
		}
	})
}

func TestIndexPage(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "z1", "Zed")
	resp, body := get(t, f.ts.URL+"/")
	if resp.StatusCode != 200 || !strings.Contains(body, "TM Sonder") ||
		!strings.Contains(body, `id="grid"`) || !strings.Contains(body, "/api/library") {
		t.Errorf("library web UI not served: %d %.120s", resp.StatusCode, body)
	}
	// Audiobook browser page.
	resp2, body2 := get(t, f.ts.URL+"/audiobooks")
	if resp2.StatusCode != 200 || !strings.Contains(body2, "Audiobooks") ||
		!strings.Contains(body2, "/api/audiobooks") {
		t.Errorf("audiobooks web UI not served: %d %.120s", resp2.StatusCode, body2)
	}
}

func TestRefreshTracksWithoutProberReturnsSession(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "r1", "Refreshable")
	resp, _ := get(t, f.ts.URL+"/api/playback/r1/refresh-tracks-x") // wrong route shape
	_ = resp
	req, _ := http.NewRequest("POST", f.ts.URL+"/api/playback/r1/refresh-tracks", nil)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r2.StatusCode != 200 {
		t.Errorf("refresh status = %d", r2.StatusCode)
	}
	readAll(t, r2)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, body := get(t, url)
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s = %d", url, resp.StatusCode)
	}
	return body
}

var _ = time.Now
