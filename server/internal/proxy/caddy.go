// Package proxy manages Sonder's optional Caddy reverse proxy without making
// Caddy, DNS, or router configuration a hard dependency of the server.
package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tm-sonder/server/internal/network"
)

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type Manager struct {
	Enabled      bool
	Binary       string
	ConfigPath   string
	LaunchdLabel string
	Run          CommandRunner
}

type Status struct {
	Enabled       bool   `json:"enabled"`
	Configured    bool   `json:"configured"`
	Binary        string `json:"binary"`
	ConfigPath    string `json:"configPath"`
	Domain        string `json:"domain"`
	Upstream      string `json:"upstream"`
	Listening     bool   `json:"listening"`
	PublicHealthy bool   `json:"publicHealthy"`
	Error         string `json:"error,omitempty"`
}

func DefaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (m Manager) runner() CommandRunner {
	if m.Run != nil {
		return m.Run
	}
	return DefaultRunner
}

// RenderCaddyfile generates only the site block Sonder owns. Existing tls
// directives are copied verbatim so `tls internal`, a custom issuer, or an
// email option is not silently replaced by a reload.
func RenderCaddyfile(state network.RuntimeState, existing []byte) ([]byte, error) {
	domain := strings.TrimSpace(state.PublicDomain)
	if domain == "" {
		return nil, fmt.Errorf("public domain is required when Caddy is enabled")
	}
	upstream := strings.TrimSpace(state.CaddyUpstream)
	if upstream == "" {
		return nil, fmt.Errorf("Caddy upstream is empty")
	}
	bind := strings.TrimSpace(state.CaddyBindAddress)
	if bind == "" {
		bind = "0.0.0.0"
	}
	lines := []string{domain + " {", "    bind " + bind}
	for _, line := range preservedTLS(existing) {
		lines = append(lines, "    "+line)
	}
	lines = append(lines, "    reverse_proxy "+upstream, "}", "")
	return []byte(strings.Join(lines, "\n")), nil
}

func preservedTLS(existing []byte) []string {
	var out []string
	for _, line := range strings.Split(string(existing), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "tls" || strings.HasPrefix(trimmed, "tls ") {
			out = append(out, trimmed)
		}
	}
	return out
}

// Validate writes no production file. It validates a temporary candidate so a
// failed Caddy parse cannot disrupt an already-running proxy.
func (m Manager) Validate(ctx context.Context, state network.RuntimeState) error {
	if !m.Enabled {
		return nil
	}
	if strings.TrimSpace(m.Binary) == "" {
		return fmt.Errorf("Caddy is enabled but no caddy binary is configured")
	}
	if strings.TrimSpace(m.ConfigPath) == "" {
		return fmt.Errorf("Caddy is enabled but no Caddyfile path is configured")
	}
	existing, err := os.ReadFile(m.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Caddyfile: %w", err)
	}
	data, err := RenderCaddyfile(state, existing)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.ConfigPath), ".sonder-caddy-*.Caddyfile")
	if err != nil {
		return fmt.Errorf("create Caddy validation file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	output, err := m.runner()(ctx, m.Binary, "validate", "--config", tmpName, "--adapter", "caddyfile")
	if err != nil {
		return fmt.Errorf("Caddy validation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Apply validates, atomically installs, and restarts/reloads Caddy. The old
// file is restored if the managed process cannot be restarted.
func (m Manager) Apply(ctx context.Context, state network.RuntimeState) error {
	if !m.Enabled {
		return nil
	}
	if err := m.Validate(ctx, state); err != nil {
		return err
	}
	existing, err := os.ReadFile(m.ConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Caddyfile: %w", err)
	}
	data, err := RenderCaddyfile(state, existing)
	if err != nil {
		return err
	}
	if err := writeAtomic(m.ConfigPath, data); err != nil {
		return err
	}
	if err := m.restart(ctx); err != nil {
		if len(existing) > 0 {
			_ = writeAtomic(m.ConfigPath, existing)
		} else {
			_ = os.Remove(m.ConfigPath)
		}
		return err
	}
	return nil
}

// Stop asks a managed LaunchAgent to exit. Caddy can also be run manually; in
// that case there is no safe assumption about ownership, so the caller keeps
// the state disabled and reports any externally managed process through the
// normal status probe.
func (m Manager) Stop(ctx context.Context) error {
	if strings.TrimSpace(m.LaunchdLabel) == "" {
		return nil
	}
	label := "gui/" + strconv.Itoa(os.Getuid()) + "/" + strings.TrimSpace(m.LaunchdLabel)
	output, err := m.runner()(ctx, "launchctl", "bootout", label)
	if err != nil {
		return fmt.Errorf("Caddy stop failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m Manager) restart(ctx context.Context) error {
	var output []byte
	var err error
	if strings.TrimSpace(m.LaunchdLabel) != "" {
		label := "gui/" + strconv.Itoa(os.Getuid()) + "/" + strings.TrimSpace(m.LaunchdLabel)
		output, err = m.runner()(ctx, "launchctl", "kickstart", "-k", label)
	} else {
		output, err = m.runner()(ctx, m.Binary, "reload", "--config", m.ConfigPath, "--adapter", "caddyfile")
	}
	if err != nil {
		return fmt.Errorf("Caddy restart/reload failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m Manager) Status(ctx context.Context, state network.RuntimeState) Status {
	status := Status{
		Enabled:    m.Enabled,
		Configured: strings.TrimSpace(state.PublicDomain) != "" && strings.TrimSpace(state.CaddyUpstream) != "",
		Binary:     m.Binary,
		ConfigPath: m.ConfigPath,
		Domain:     state.PublicDomain,
		Upstream:   state.CaddyUpstream,
	}
	if !m.Enabled {
		return status
	}
	if _, err := exec.LookPath(m.Binary); err != nil {
		status.Error = "Caddy binary is not installed"
		return status
	}
	if _, err := os.Stat(m.ConfigPath); err != nil {
		status.Error = "Caddyfile is missing"
		return status
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if strings.TrimSpace(state.PublicHealthCheckURL) != "" {
		status.Listening = waitHTTP(probeCtx, state.PublicHealthCheckURL) == nil
	} else {
		status.Listening = waitTCP(probeCtx, caddyListenAddress(state)) == nil
	}
	if url := strings.TrimSpace(state.PublicHealthCheckURL); url != "" {
		status.PublicHealthy = waitHTTP(probeCtx, url) == nil
	}
	return status
}

// WaitHealthy waits for the managed HTTPS listener. A configured health URL
// is preferred because it verifies the public route and certificate; without
// one, the local TCP listener is the strongest check available.
func (m Manager) WaitHealthy(ctx context.Context, state network.RuntimeState) error {
	if !m.Enabled {
		return nil
	}
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		var err error
		if strings.TrimSpace(state.PublicHealthCheckURL) != "" {
			err = waitHTTP(probeCtx, state.PublicHealthCheckURL)
		} else {
			err = waitTCP(probeCtx, caddyListenAddress(state))
		}
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Caddy listener did not become healthy: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("Caddy listener did not become healthy: %w", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func caddyListenAddress(state network.RuntimeState) string {
	host := strings.TrimSpace(state.CaddyBindAddress)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, "443")
}

func waitTCP(ctx context.Context, address string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}

func waitHTTP(ctx context.Context, rawURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return fmt.Errorf("health check returned HTTP %d", response.StatusCode)
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sonder-caddy-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
