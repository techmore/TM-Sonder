package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/mediacache"
)

func newEmptyStore() *library.Store { return library.New() }

func TestSettingsGetLoopbackIncludesToken(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.AllowLAN = true
	cfg.PairingToken = "tok-123"

	f := &fixture{s: New(&cfg, newEmptyStore(), nil, nil)}
	f.s.SetConfigPath(filepath.Join(t.TempDir(), "server.json"))

	req := httptest.NewRequest("GET", "/api/settings", nil)
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var p map[string]any
	json.Unmarshal(rec.Body.Bytes(), &p)
	if p["pairingToken"] != "tok-123" {
		t.Errorf("loopback should see token, got %v", p["pairingToken"])
	}
	if _, ok := p["suggestedMounts"]; !ok {
		t.Error("suggestedMounts missing")
	}
	if p["libraryLayout"] != "rails" {
		t.Errorf("default library layout = %v, want rails", p["libraryLayout"])
	}
	if p["hideEmptyLibraries"] != true {
		t.Errorf("default hide empty libraries = %v, want true", p["hideEmptyLibraries"])
	}
}

func TestSettingsPutAddsLibraryAndRescans(t *testing.T) {
	mediaDir := t.TempDir()
	os.WriteFile(filepath.Join(mediaDir, "Film.mp4"), []byte("0123456789"), 0o600)

	cfg := config.Default()
	f := newFixture(t, func(c *config.Config) { c.DataDir = cfg.DataDir })
	realPath := filepath.Join(t.TempDir(), "server.json")
	f.s.SetConfigPath(realPath)

	body := `{"libraries":[{"name":"NAS","path":"` + mediaDir + `","kind":"movie"}]}`
	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("put status %d: %s", rec.Code, rec.Body.String())
	}

	// Config file persisted.
	data, err := os.ReadFile(realPath)
	if err != nil || !strings.Contains(string(data), `"NAS"`) {
		t.Fatalf("config not persisted: %v %s", err, data)
	}

	// Rescan was kicked off asynchronously; poll for the item.
	var found bool
	for i := 0; i < 40; i++ {
		time.Sleep(100 * time.Millisecond)
		if f.store.Count() == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("library was not rescanned after add")
	}
}

func TestSettingsPutPersistsLibraryLayout(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "server.json")
	f.s.SetConfigPath(path)

	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"libraryLayout":"classic"}`))
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.s.cfg().LibraryLayout; got != "classic" {
		t.Fatalf("in-memory library layout = %q, want classic", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"libraryLayout": "classic"`) {
		t.Fatalf("library layout not persisted: %v %s", err, data)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["libraryLayout"] != "classic" {
		t.Errorf("response library layout = %v, want classic", payload["libraryLayout"])
	}
}

func TestSettingsPutPersistsAudiobookLayout(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "server.json")
	f.s.SetConfigPath(path)

	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"audiobookLayout":"classic"}`))
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.s.cfg().AudiobookLayout; got != "classic" {
		t.Fatalf("in-memory audiobook layout = %q, want classic", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"audiobookLayout": "classic"`) {
		t.Fatalf("audiobook layout not persisted: %v %s", err, data)
	}
}

func TestSettingsPutPersistsHideEmptyLibraries(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "server.json")
	f.s.SetConfigPath(path)

	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"hideEmptyLibraries":false}`))
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if f.s.cfg().HideEmptyLibraries {
		t.Fatal("in-memory hide empty libraries remained enabled")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"hideEmptyLibraries": false`) {
		t.Fatalf("hide empty libraries not persisted: %v %s", err, data)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["hideEmptyLibraries"] != false {
		t.Errorf("response hide empty libraries = %v, want false", payload["hideEmptyLibraries"])
	}
}

func TestSettingsPutReconfiguresMediaCache(t *testing.T) {
	f := newFixture(t, nil)
	path := filepath.Join(t.TempDir(), "server.json")
	f.s.SetConfigPath(path)
	cache, err := mediacache.New(mediacache.Config{
		Enabled: true, Dir: filepath.Join(t.TempDir(), "media-cache"), MaxBytes: 1024, MinFreeBytes: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	f.s.SetMediaCache(cache)

	req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(`{"mediaCache":{"enabled":true,"maxBytes":64,"minFreeBytes":7}}`))
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.s.cfg().MediaCache; got.MaxBytes != 64 || got.MinFreeBytes != 7 || !got.Enabled {
		t.Fatalf("in-memory media cache settings = %+v", got)
	}
	status := cache.Status()
	if status.MaxBytes != 64 || status.MinFreeBytes != 7 || !status.Enabled {
		t.Fatalf("live media cache settings = %+v", status)
	}

	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"maxBytes": 64`) {
		t.Fatalf("media cache settings not persisted: %v %s", err, data)
	}
}

func TestSettingsPutRejectsBadKindAndMissingPath(t *testing.T) {
	f := newFixture(t, nil)
	f.s.SetConfigPath(filepath.Join(t.TempDir(), "server.json"))

	cases := []string{
		`{"libraries":[{"name":"X","path":"/no/such/dir","kind":"movie"}]}`,
		`{"libraries":[{"name":"X","path":"` + t.TempDir() + `","kind":"films"}]}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest("PUT", "/api/settings", strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:1111"
		req.Host = "127.0.0.1:8797"
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s -> status %d, want 400", body, rec.Code)
		}
	}
}

func TestSettingsRescanEndpoint(t *testing.T) {
	f := newFixture(t, nil)
	req := httptest.NewRequest("POST", "/api/settings/rescan", nil)
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("status %d, want 202", rec.Code)
	}
}

func TestSettingsBrowse(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "Movies")
	os.MkdirAll(filepath.Join(sub, "Nested"), 0o755)
	os.WriteFile(filepath.Join(sub, "file.mp4"), []byte("x"), 0o600)
	f := newFixture(t, nil)

	getJSON := func(q string) map[string]any {
		req := httptest.NewRequest("GET", "/api/settings/browse"+q, nil)
		req.RemoteAddr = "127.0.0.1:1111"
		req.Host = "127.0.0.1:8797"
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("browse %q status %d: %s", q, rec.Code, rec.Body.String())
		}
		var p map[string]any
		json.Unmarshal(rec.Body.Bytes(), &p)
		return p
	}

	res := getJSON("?path=" + sub)
	entries := res["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("want 1 dir entry (files+hidden excluded), got %v", res["entries"])
	}
	if entries[0].(map[string]any)["name"] != "Nested" {
		t.Errorf("entry = %v", entries[0])
	}
	if res["parent"] == nil {
		t.Error("parent missing")
	}

	// Nonexistent path -> 400.
	req := httptest.NewRequest("GET", "/api/settings/browse?path=/no/such/dir", nil)
	req.RemoteAddr = "127.0.0.1:1111"
	req.Host = "127.0.0.1:8797"
	rec2 := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec2, req)
	if rec2.Code != 400 {
		t.Errorf("bad path status = %d", rec2.Code)
	}

	// No path -> starting points include Home, plus the NAS mount point when
	// the platform has one.
	//
	// The volumes entry is guarded by an os.Stat on /Volumes, because on Linux
	// (including the container machine, where the NAS is deliberately not
	// mounted) that directory does not exist and offering it would send a user
	// to a browse error. Asserting it unconditionally made this test pass only
	// on macOS and fail everywhere else.
	res3 := getJSON("")
	names := []string{}
	for _, e := range res3["entries"].([]any) {
		names = append(names, e.(map[string]any)["name"].(string))
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "Home") {
		t.Errorf("starting points missing Home: %v", names)
	}
	_, volumesErr := os.Stat("/Volumes")
	hasVolumesEntry := strings.Contains(joined, "Volumes")
	if volumesErr == nil && !hasVolumesEntry {
		t.Errorf("/Volumes exists but is not offered as a starting point: %v", names)
	}
	if volumesErr != nil && hasVolumesEntry {
		t.Errorf("/Volumes does not exist but is offered anyway: %v", names)
	}
}

// TestConcurrentSettingsWritesAndReads guards the copy-on-write config fix:
// settings writes must not race config reads from other handlers. Run with
// -race to be meaningful.
func TestConcurrentSettingsWritesAndReads(t *testing.T) {
	mediaDir := t.TempDir()
	cfg := config.Default()
	f := newFixture(t, func(c *config.Config) { c.DataDir = cfg.DataDir })
	f.s.SetConfigPath(filepath.Join(t.TempDir(), "server.json"))

	do := func(method, path, body string) {
		var rdr *strings.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		} else {
			rdr = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, rdr)
		req.RemoteAddr = "127.0.0.1:1111"
		req.Host = "127.0.0.1:8797"
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
	}

	body := `{"libraries":[{"name":"NAS","path":"` + mediaDir + `","kind":"movie"}],"themePreset":"earthy"}`
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			do("PUT", "/api/settings", body)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			do("GET", "/api/health", "")
			do("GET", "/api/discovery", "")
			do("GET", "/api/settings", "")
		}()
	}
	wg.Wait()

	// The last write is visible and the config file parses.
	if got := f.s.cfg().ThemePreset; got != "earthy" {
		t.Errorf("theme = %q", got)
	}
}

// TestSettingsClearingLibrariesRequiresConfirmation verifies the guard against
// accidentally wiping the catalog with an empty library table.
func TestSettingsClearingLibrariesRequiresConfirmation(t *testing.T) {
	cfg := config.Default()
	f := newFixture(t, func(c *config.Config) {
		c.DataDir = cfg.DataDir
		c.Libraries = []config.Library{{ID: "movies", Name: "Movies", Path: t.TempDir(), Kind: "movie"}}
	})
	f.s.SetConfigPath(filepath.Join(t.TempDir(), "server.json"))

	put := func(path string) int {
		req := httptest.NewRequest("PUT", path, strings.NewReader(`{"libraries":[]}`))
		req.RemoteAddr = "127.0.0.1:1111"
		req.Host = "127.0.0.1:8797"
		rec := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := put("/api/settings"); got != http.StatusConflict {
		t.Errorf("unconfirmed clear status = %d, want 409", got)
	}
	if got := put("/api/settings?confirm=empty-libraries"); got != http.StatusOK {
		t.Errorf("confirmed clear status = %d, want 200", got)
	}
	if n := len(f.s.cfg().Libraries); n != 0 {
		t.Errorf("libraries = %d, want 0 after confirmed clear", n)
	}
}
