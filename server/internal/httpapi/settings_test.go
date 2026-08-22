package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
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
	rec2 := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec2, req)
	if rec2.Code != 400 {
		t.Errorf("bad path status = %d", rec2.Code)
	}

	// No path -> starting points include Home and Volumes.
	res3 := getJSON("")
	names := []string{}
	for _, e := range res3["entries"].([]any) {
		names = append(names, e.(map[string]any)["name"].(string))
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "Home") || !strings.Contains(joined, "Volumes") {
		t.Errorf("starting points missing Home/Volumes: %v", names)
	}
}
