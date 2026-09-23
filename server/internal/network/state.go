package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RuntimeStateFilename = "runtime-state.json"

// RuntimeState is intentionally separate from server.json. It changes as the
// user switches adapters and as the process restarts, while library settings
// and credentials remain in the existing config file.
type RuntimeState struct {
	SelectedMode         BindingMode `json:"selectedMode"`
	SelectedInterface    string      `json:"selectedInterface"`
	SelectedIPv4         string      `json:"selectedIPv4"`
	WebBindAddress       string      `json:"webBindAddress"`
	APIBindAddress       string      `json:"apiBindAddress"`
	WebPort              int         `json:"webPort"`
	APIPort              int         `json:"apiPort"`
	PublicDomain         string      `json:"publicDomain"`
	PublicHealthCheckURL string      `json:"publicHealthCheckURL"`
	CaddyEnabled         bool        `json:"caddyEnabled"`
	CaddyBindAddress     string      `json:"caddyBindAddress"`
	CaddyUpstream        string      `json:"caddyUpstream"`
	ProcessStartTime     time.Time   `json:"processStartTime"`
	LastError            string      `json:"lastError,omitempty"`
}

func StatePath(dataDir string) string { return filepath.Join(dataDir, RuntimeStateFilename) }

func LoadState(path string) (RuntimeState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeState{}, err
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeState{}, fmt.Errorf("parse runtime state: %w", err)
	}
	return state, nil
}

func SaveState(path string, state RuntimeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime state: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create runtime state directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".sonder-runtime-*.tmp")
	if err != nil {
		return fmt.Errorf("create runtime state temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write runtime state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync runtime state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close runtime state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install runtime state: %w", err)
	}
	return nil
}

func DefaultState(webPort, apiPort int, allowLAN bool, interfaces []Interface) RuntimeState {
	if webPort <= 0 {
		webPort = 8797
	}
	if apiPort <= 0 {
		apiPort = webPort + 1
	}
	mode := ModeLoopback
	if allowLAN {
		mode = ModeWiFiLAN
	}
	state := RuntimeState{
		SelectedMode:     mode,
		APIBindAddress:   "127.0.0.1",
		WebPort:          webPort,
		APIPort:          apiPort,
		CaddyBindAddress: "0.0.0.0",
	}
	if binding, err := Resolve(mode, "", interfaces); err == nil {
		state.SelectedInterface = binding.InterfaceID
		state.SelectedIPv4 = binding.IPv4
		state.WebBindAddress = binding.WebBindAddress
	} else {
		state.SelectedMode = ModeLoopback
		if loopback := findType(interfaces, TypeLoopback); loopback != nil {
			state.SelectedInterface = loopback.ID
			state.SelectedIPv4 = loopback.IPv4
			state.WebBindAddress = loopback.IPv4
		} else {
			state.SelectedInterface = "lo0"
			state.SelectedIPv4 = "127.0.0.1"
			state.WebBindAddress = "127.0.0.1"
		}
	}
	state.CaddyUpstream = net.JoinHostPort(state.SelectedIPv4, fmt.Sprint(state.WebPort))
	return state
}

func Normalize(state *RuntimeState, webPort, apiPort int) {
	if state.SelectedMode == "" {
		state.SelectedMode = ModeLoopback
	}
	if state.WebPort <= 0 {
		state.WebPort = webPort
	}
	if state.WebPort <= 0 {
		state.WebPort = 8797
	}
	if state.APIPort <= 0 {
		state.APIPort = apiPort
	}
	if state.APIPort <= 0 {
		state.APIPort = state.WebPort + 1
	}
	if state.APIPort == state.WebPort {
		state.APIPort = state.WebPort + 1
		if state.APIPort > 65535 {
			state.APIPort = 1
		}
	}
	if strings.TrimSpace(state.APIBindAddress) == "" {
		state.APIBindAddress = "127.0.0.1"
	}
	if strings.TrimSpace(state.CaddyBindAddress) == "" {
		state.CaddyBindAddress = "0.0.0.0"
	}
	if state.SelectedIPv4 != "" {
		state.CaddyUpstream = net.JoinHostPort(state.SelectedIPv4, fmt.Sprint(state.WebPort))
	}
}
