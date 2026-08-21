package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != DefaultPort {
		t.Errorf("port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.ThemePreset != DefaultThemePreset {
		t.Errorf("themePreset = %q", cfg.ThemePreset)
	}
	if cfg.Transcode.MaxConcurrent != 2 || cfg.Transcode.HWAccel != "videotoolbox" {
		t.Errorf("transcode defaults wrong: %+v", cfg.Transcode)
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func setenv(t *testing.T, k, v string) {
	t.Helper()
	old, had := os.LookupEnv(k)
	os.Setenv(k, v)
	t.Cleanup(func() {
		if had {
			os.Setenv(k, old)
		} else {
			os.Unsetenv(k)
		}
	})
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	p := write(t, `{
		// commented line tolerated
		"port": 9000,
		"dataDir": "`+dir+`",
		"libraries": [{"id":"l1","name":"Films","path":"/media/films","kind":"movie"}],
		"allowLAN": true,
		"themePreset": "midnight",
		"transcode": {"maxConcurrent": 4, "hwaccel": "none", "preset": "fast"}
	}`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 || !cfg.AllowLAN || cfg.ThemePreset != "midnight" {
		t.Errorf("file values not applied: %+v", cfg)
	}
	if len(cfg.Libraries) != 1 || cfg.Libraries[0].Kind != "movie" {
		t.Errorf("libraries not applied: %+v", cfg.Libraries)
	}
	if cfg.Transcode.MaxConcurrent != 4 {
		t.Errorf("transcode not applied: %+v", cfg.Transcode)
	}
}

func TestRejectsUnknownKeys(t *testing.T) {
	p := write(t, `{"port": 8797, "porrt": 1234}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown key")
	}
	p2 := write(t, `{"port": 8797}`)
	if _, err := Load(p2); err != nil {
		t.Fatalf("valid minimal config rejected: %v", err)
	}
}

func TestEnvOverridesBeatFile(t *testing.T) {
	p := write(t, `{"port": 9000, "dataDir": "/tmp/fromfile"}`)
	setenv(t, "SONDER_PORT", "9100")
	setenv(t, "SONDER_DATA_DIR", "/tmp/fromenv")
	setenv(t, "SONDER_ALLOW_LAN", "true")
	setenv(t, "SONDER_TOKEN", "sekrit")
	setenv(t, "SONDER_HWACCEL", "none")

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9100 || cfg.DataDir != "/tmp/fromenv" || !cfg.AllowLAN ||
		cfg.PairingToken != "sekrit" || cfg.Transcode.HWAccel != "none" {
		t.Errorf("env overrides failed: %+v", cfg)
	}
}

func TestResolvePathSearchOrder(t *testing.T) {
	// Isolate $HOME so the search-order probe never touches the real
	// user config directory.
	t.Setenv("HOME", t.TempDir())
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if wd := os.Getenv("OLDWD"); wd != "" {
			os.Chdir(wd)
		}
	}()

	if p, ok := ResolvePath(); p == "" {
		t.Errorf("expected non-empty path, got %q (exists=%v)", p, ok)
	}

	if err := os.WriteFile("sonder-server.json", []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, ok := ResolvePath(); !ok || p != "sonder-server.json" {
		t.Errorf("local file not preferred: %q exists=%v", p, ok)
	}
	os.Remove("sonder-server.json")

	setenv(t, "SONDER_CONFIG", filepath.Join(t.TempDir(), "explicit.json"))
	if p, _ := ResolvePath(); p != os.Getenv("SONDER_CONFIG") {
		t.Errorf("SONDER_CONFIG ignored: %q", p)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	base := Default()
	base.DataDir = t.TempDir()
	bad := base
	bad.Port = 0
	if err := bad.validate(); err == nil {
		t.Error("port 0 accepted")
	}
	bad = base
	bad.Libraries = []Library{{ID: "x", Name: "X", Path: "/m", Kind: "films"}}
	if err := bad.validate(); err == nil {
		t.Error("invalid library kind accepted")
	}
	bad = base
	bad.Transcode.HWAccel = "cuda"
	if err := bad.validate(); err == nil {
		t.Error("invalid hwaccel accepted")
	}
}

func TestWriteTemplateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "server.json")
	if err := WriteTemplate(p); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err != nil {
		t.Fatalf("generated template does not load: %v", err)
	}
	before, _ := os.ReadFile(p)
	if err := WriteTemplate(p); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Error("WriteTemplate overwrote existing file")
	}
}
