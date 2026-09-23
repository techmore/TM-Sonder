// Package config resolves server configuration from three layers:
// built-in defaults <- JSON file <- environment variables.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DefaultPort matches Jellyfin's native HTTP listener so clients such as
	// BookPlayer can connect with the standard host:8096 expectation.
	DefaultPort        = 8096
	DefaultThemePreset = "earthy"

	// DefaultAudiobookLayout is the rails browser ("rails"); the legacy
	// list view remains available as "classic".
	DefaultAudiobookLayout = "rails"

	// DefaultMediaLayout is the rails browser for movies/TV ("rails");
	// "grid" keeps the classic full-grid view.
	DefaultMediaLayout = "rails"
)

var validKinds = map[string]bool{
	"movie": true, "tvShow": true, "documentary": true,
	"audiobook": true, "ebook": true, "all": true,
	// "plex" is a meta-kind: expanded into child libraries by
	// ExpandPlexLibraries based on Plex-standard subfolder names.
	"plex": true,
}

var validHWAccel = map[string]bool{
	"videotoolbox": true, "none": true, "vaapi": true, "qsv": true,
}

// ValidKind reports whether kind is a supported library kind. It is the single
// source of truth shared by config loading and the settings API.
func ValidKind(kind string) bool { return validKinds[kind] }

type Library struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Transcode struct {
	MaxConcurrent int    `json:"maxConcurrent"`
	HWAccel       string `json:"hwaccel"`
	Preset        string `json:"preset"`
}

type Config struct {
	// Port is retained as the backwards-compatible web port key. WebPort is
	// the preferred name for new installs; Load normalizes the two so the
	// rest of the application can continue to expose the existing API shape.
	Port              int       `json:"port"`
	WebPort           int       `json:"webPort,omitempty"`
	APIPort           int       `json:"apiPort,omitempty"`
	DataDir           string    `json:"dataDir"`
	Libraries         []Library `json:"libraries"`
	AllowLAN          bool      `json:"allowLAN"`
	PairingToken      string    `json:"pairingToken"`
	ThemePreset       string    `json:"themePreset"`
	AudiobookLayout   string    `json:"audiobookLayout"`
	MoviesLayout      string    `json:"moviesLayout"`
	TVLayout          string    `json:"tvLayout"`
	FFmpegPath        string    `json:"ffmpegPath"`
	FFprobePath       string    `json:"ffprobePath"`
	ProbeWorkers      int       `json:"probeWorkers,omitempty"`
	ThumbWorkers      int       `json:"thumbWorkers,omitempty"`
	SafeScan          bool      `json:"safeScan"`
	Transcode         Transcode `json:"transcode"`
	LogDir            string    `json:"logDir"`
	CaddyPath         string    `json:"caddyPath,omitempty"`
	CaddyConfigPath   string    `json:"caddyConfigPath,omitempty"`
	CaddyLaunchdLabel string    `json:"caddyLaunchdLabel,omitempty"`
}

func Default() Config {
	dataDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dataDir = filepath.Join(home, "Library", "Application Support", "TM-Sonder-Server")
	}
	return Config{
		Port:        DefaultPort,
		WebPort:     0,
		APIPort:     0,
		DataDir:     dataDir,
		ThemePreset: DefaultThemePreset,
		AudiobookLayout: DefaultAudiobookLayout,
		MoviesLayout:    DefaultMediaLayout,
		TVLayout:        DefaultMediaLayout,
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
		SafeScan:    true,
		Transcode:   Transcode{MaxConcurrent: 2, HWAccel: "videotoolbox", Preset: "veryfast"},
		LogDir:      filepath.Join(dataDir, "logs"),
	}
}

// ResolvePath returns the config file to use and whether it exists.
// Search order: $SONDER_CONFIG, ./sonder-server.json, ~/.config/sonder/server.json.
// The first path wins even if the file does not exist yet; callers may then
// auto-generate it via WriteTemplate.
func ResolvePath() (string, bool) {
	if p := os.Getenv("SONDER_CONFIG"); p != "" {
		return p, fileExists(p)
	}
	local := "sonder-server.json"
	if fileExists(local) {
		return local, true
	}
	home, err := os.UserHomeDir()
	if err == nil {
		p := filepath.Join(home, ".config", "sonder", "server.json")
		if fileExists(p) {
			return p, true
		}
		return p, false
	}
	return local, false
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
		if err := unmarshalStrict(stripComments(data), &cfg); err != nil {
			return nil, fmt.Errorf("config: %s: %w", path, err)
		}
		cfg.DataDir = expandHome(cfg.DataDir)
		cfg.LogDir = expandHome(cfg.LogDir)
		cfg.CaddyConfigPath = expandHome(cfg.CaddyConfigPath)
		for i := range cfg.Libraries {
			cfg.Libraries[i].Path = expandHome(cfg.Libraries[i].Path)
		}
	}
	cfg.applyEnv()
	// Port predates the explicit web/API split. Keep it synchronized so
	// existing handlers and clients continue to see the same web port while
	// new runtime state can bind the private API independently.
	if cfg.WebPort > 0 && cfg.Port == DefaultPort {
		cfg.Port = cfg.WebPort
	}
	if cfg.WebPort <= 0 {
		cfg.WebPort = cfg.Port
	}
	if cfg.Port <= 0 {
		cfg.Port = cfg.WebPort
	}
	if cfg.APIPort <= 0 {
		cfg.APIPort = cfg.WebPort + 1
	}
	// Resolve "plex" meta-libraries into per-kind child libraries before
	// validation so downstream code only ever sees concrete kinds.
	libs, err := ExpandPlexLibraries(cfg.Libraries)
	if err != nil {
		return nil, err
	}
	cfg.Libraries = libs
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// unmarshalStrict rejects unknown keys so typos in JSON configs fail loudly.
func unmarshalStrict(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing data after JSON object")
	}
	return nil
}

// stripComments removes full-line // comments so the generated template can
// carry documentation while remaining loadable.
func stripComments(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "//") {
			lines[i] = ""
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func (c *Config) applyEnv() {
	if v := os.Getenv("SONDER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
			c.WebPort = n
		}
	}
	if v := os.Getenv("SONDER_WEB_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.WebPort = n
			c.Port = n
		}
	}
	if v := os.Getenv("SONDER_API_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.APIPort = n
		}
	}
	if v := os.Getenv("SONDER_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("SONDER_ALLOW_LAN"); v != "" {
		c.AllowLAN = parseBool(v)
	}
	if v := os.Getenv("SONDER_SAFE_SCAN"); v != "" {
		c.SafeScan = parseBool(v)
	}
	if v := os.Getenv("SONDER_TOKEN"); v != "" {
		c.PairingToken = v
	}
	if v := os.Getenv("SONDER_THEME_PRESET"); v != "" {
		c.ThemePreset = v
	}
	if v := os.Getenv("SONDER_FFMPEG_PATH"); v != "" {
		c.FFmpegPath = v
	}
	if v := os.Getenv("SONDER_FFPROBE_PATH"); v != "" {
		c.FFprobePath = v
	}
	if v := os.Getenv("SONDER_PROBE_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ProbeWorkers = n
		}
	}
	if v := os.Getenv("SONDER_THUMB_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ThumbWorkers = n
		}
	}
	if v := os.Getenv("SONDER_LOG_DIR"); v != "" {
		c.LogDir = v
	}
	if v := os.Getenv("SONDER_CADDY_PATH"); v != "" {
		c.CaddyPath = v
	}
	if v := os.Getenv("SONDER_CADDY_CONFIG"); v != "" {
		c.CaddyConfigPath = v
	}
	if v := os.Getenv("SONDER_CADDY_LAUNCHD_LABEL"); v != "" {
		c.CaddyLaunchdLabel = v
	}
	if v := os.Getenv("SONDER_TRANSCODE_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Transcode.MaxConcurrent = n
		}
	}
	if v := os.Getenv("SONDER_HWACCEL"); v != "" {
		c.Transcode.HWAccel = strings.ToLower(v)
	}
	if v := os.Getenv("SONDER_TRANSCODE_PRESET"); v != "" {
		c.Transcode.Preset = v
	}
	c.AudiobookLayout = NormalizeAudiobookLayout(c.AudiobookLayout)
	c.MoviesLayout = NormalizeMediaLayout(c.MoviesLayout)
	c.TVLayout = NormalizeMediaLayout(c.TVLayout)
}

// NormalizeAudiobookLayout coerces a layout preference to "rails" or
// "classic", defaulting to rails.
func NormalizeAudiobookLayout(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "classic":
		return "classic"
	default:
		return DefaultAudiobookLayout
	}
}

// NormalizeMediaLayout coerces a movies/TV layout preference to "rails"
// or "grid", defaulting to rails.
func NormalizeMediaLayout(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "grid":
		return "grid"
	default:
		return DefaultMediaLayout
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("config: port %d out of range 1-65535", c.Port)
	}
	if c.WebPort < 1 || c.WebPort > 65535 {
		return fmt.Errorf("config: webPort %d out of range 1-65535", c.WebPort)
	}
	if c.APIPort < 1 || c.APIPort > 65535 {
		return fmt.Errorf("config: apiPort %d out of range 1-65535", c.APIPort)
	}
	if c.WebPort != c.Port {
		return fmt.Errorf("config: port (%d) and webPort (%d) must match; use webPort as the preferred key", c.Port, c.WebPort)
	}
	if c.DataDir == "" {
		return fmt.Errorf("config: dataDir is required")
	}
	for i, lib := range c.Libraries {
		if !validKinds[lib.Kind] {
			return fmt.Errorf("config: libraries[%d] (%q) has invalid kind %q", i, lib.Name, lib.Kind)
		}
		if lib.Path == "" {
			return fmt.Errorf("config: libraries[%d] (%q) has empty path", i, lib.Name)
		}
	}
	if !validHWAccel[c.Transcode.HWAccel] {
		return fmt.Errorf("config: transcode.hwaccel %q not in videotoolbox|none|vaapi|qsv", c.Transcode.HWAccel)
	}
	if c.Transcode.MaxConcurrent < 1 {
		c.Transcode.MaxConcurrent = 2
	}
	if c.LogDir == "" {
		c.LogDir = filepath.Join(c.DataDir, "logs")
	}
	return nil
}

// WriteTemplate writes a commented default config at path. Parent directories
// are created as needed. An existing file is never overwritten.
func WriteTemplate(path string) error {
	if fileExists(path) {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, templateBytes, 0o600)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

var templateBytes = []byte(`// TM Sonder Go server configuration.
// Full-line // comments are tolerated on load; all other content is strict
// JSON — unknown keys are rejected.
//
// Environment overrides (highest precedence):
//   SONDER_PORT, SONDER_DATA_DIR, SONDER_ALLOW_LAN, SONDER_TOKEN,
//   SONDER_WEB_PORT, SONDER_API_PORT, SONDER_CADDY_PATH,
//   SONDER_CADDY_CONFIG, SONDER_CADDY_LAUNCHD_LABEL,
//   SONDER_SAFE_SCAN, SONDER_THEME_PRESET, SONDER_FFMPEG_PATH,
//   SONDER_FFPROBE_PATH, SONDER_PROBE_WORKERS, SONDER_THUMB_WORKERS,
//   SONDER_LOG_DIR,
//   SONDER_TRANSCODE_MAX_CONCURRENT, SONDER_HWACCEL,
//   SONDER_TRANSCODE_PRESET
{
  "port": 8096,
  "webPort": 8096,
  "apiPort": 8097,
  "dataDir": "~/Library/Application Support/TM-Sonder-Server",
  "libraries": [
    { "id": "movies", "name": "Movies", "path": "/path/to/media", "kind": "movie" }
  ],
  "allowLAN": false,
  "pairingToken": "",
  "safeScan": true,
  "themePreset": "earthy",
  "ffmpegPath": "ffmpeg",
  "ffprobePath": "ffprobe",
  "probeWorkers": 0,
  "thumbWorkers": 0,
  "transcode": { "maxConcurrent": 2, "hwaccel": "videotoolbox", "preset": "veryfast" },
  "logDir": "",
  "caddyPath": "caddy",
  "caddyConfigPath": "~/Library/Application Support/TM-Sonder-Server/Caddyfile",
  "caddyLaunchdLabel": ""
}
`)
