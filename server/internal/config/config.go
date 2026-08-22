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
	DefaultPort        = 8797
	DefaultThemePreset = "earthy"
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
	Port         int       `json:"port"`
	DataDir      string    `json:"dataDir"`
	Libraries    []Library `json:"libraries"`
	AllowLAN     bool      `json:"allowLAN"`
	PairingToken string    `json:"pairingToken"`
	ThemePreset  string    `json:"themePreset"`
	FFmpegPath   string    `json:"ffmpegPath"`
	FFprobePath  string    `json:"ffprobePath"`
	Transcode    Transcode `json:"transcode"`
	LogDir       string    `json:"logDir"`
}

func Default() Config {
	dataDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		dataDir = filepath.Join(home, "Library", "Application Support", "TM-Sonder-Server")
	}
	return Config{
		Port:        DefaultPort,
		DataDir:     dataDir,
		ThemePreset: DefaultThemePreset,
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
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
		for i := range cfg.Libraries {
			cfg.Libraries[i].Path = expandHome(cfg.Libraries[i].Path)
		}
	}
	cfg.applyEnv()
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
		}
	}
	if v := os.Getenv("SONDER_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("SONDER_ALLOW_LAN"); v != "" {
		c.AllowLAN = parseBool(v)
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
	if v := os.Getenv("SONDER_LOG_DIR"); v != "" {
		c.LogDir = v
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
//   SONDER_THEME_PRESET, SONDER_FFMPEG_PATH, SONDER_FFPROBE_PATH,
//   SONDER_LOG_DIR, SONDER_TRANSCODE_MAX_CONCURRENT, SONDER_HWACCEL,
//   SONDER_TRANSCODE_PRESET
{
  "port": 8797,
  "dataDir": "~/Library/Application Support/TM-Sonder-Server",
  "libraries": [
    { "id": "movies", "name": "Movies", "path": "/path/to/media", "kind": "movie" }
  ],
  "allowLAN": false,
  "pairingToken": "",
  "themePreset": "earthy",
  "ffmpegPath": "ffmpeg",
  "ffprobePath": "ffprobe",
  "transcode": { "maxConcurrent": 2, "hwaccel": "videotoolbox", "preset": "veryfast" },
  "logDir": ""
}
`)
