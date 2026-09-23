// Package runtimecontrol coordinates network binding swaps and the optional
// Caddy process. It contains no catalog or scanner logic, which keeps a swap
// from touching the library database.
package runtimecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tm-sonder/server/internal/network"
	"tm-sonder/server/internal/proxy"
)

const DefaultLaunchdLabel = "com.tm-sonder.server"

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type InterfaceSource func() ([]network.Interface, error)

type Status struct {
	State               network.RuntimeState `json:"state"`
	Interfaces          []network.Interface  `json:"interfaces"`
	WebHealthy          bool                 `json:"webHealthy"`
	APIHealthy          bool                 `json:"apiHealthy"`
	Rebinding           bool                 `json:"rebinding"`
	Caddy               proxy.Status         `json:"caddy"`
	DatabasePublic      bool                 `json:"databasePublic"`
	PublicPrerequisites []string             `json:"publicPrerequisites"`
	UptimeSeconds       int64                `json:"uptimeSeconds"`
	LastError           string               `json:"lastError,omitempty"`
}

type Controller struct {
	StatePath    string
	State        network.RuntimeState
	Caddy        proxy.Manager
	LaunchdLabel string
	PairingToken string
	InterfacesFn InterfaceSource
	Run          CommandRunner
	RestartFn    func(context.Context, network.RuntimeState) error

	mu        sync.Mutex
	rebinding atomic.Bool
}

func (c *Controller) interfaces() ([]network.Interface, error) {
	if c.InterfacesFn != nil {
		return c.InterfacesFn()
	}
	return network.Enumerate()
}

// InterfacesForAPI exposes the same active IPv4-only inventory used by a
// binding switch. It is intentionally a snapshot; callers must revalidate a
// target immediately before changing anything.
func (c *Controller) InterfacesForAPI() ([]network.Interface, error) {
	return c.interfaces()
}

// ValidateBinding performs the no-side-effect portion of a switch. HTTP
// callers use it before returning 202 so an unavailable adapter or malformed
// Caddy candidate is reported immediately to the menu bar.
func (c *Controller) ValidateBinding(ctx context.Context, mode network.BindingMode, interfaceID string) error {
	interfaces, err := c.interfaces()
	if err != nil {
		return fmt.Errorf("enumerate interfaces: %w", err)
	}
	binding, err := network.Resolve(mode, interfaceID, interfaces)
	if err != nil {
		return fmt.Errorf("validate target interface: %w", err)
	}
	c.mu.Lock()
	state := c.State
	caddy := c.Caddy
	c.mu.Unlock()
	state.SelectedMode = binding.Mode
	state.SelectedInterface = binding.InterfaceID
	state.SelectedIPv4 = binding.IPv4
	state.WebBindAddress = binding.WebBindAddress
	state.APIBindAddress = binding.APIBindAddress
	state.CaddyUpstream = net.JoinHostPort(binding.IPv4, strconv.Itoa(state.WebPort))
	if state.CaddyEnabled {
		if err := caddy.Validate(ctx, state); err != nil {
			return fmt.Errorf("validate Caddy before swap: %w", err)
		}
	}
	return nil
}

func (c *Controller) save(state network.RuntimeState) error {
	if strings.TrimSpace(c.StatePath) == "" {
		return errors.New("runtime state path is not configured")
	}
	return network.SaveState(c.StatePath, state)
}

func (c *Controller) runner() CommandRunner {
	if c.Run != nil {
		return c.Run
	}
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
}

func (c *Controller) restart(ctx context.Context, state network.RuntimeState) error {
	if c.RestartFn != nil {
		return c.RestartFn(ctx, state)
	}
	label := strings.TrimSpace(c.LaunchdLabel)
	if label == "" {
		label = DefaultLaunchdLabel
	}
	if _, err := os.Stat("/bin/launchctl"); err != nil {
		return fmt.Errorf("launchd restart unavailable: %w", err)
	}
	launchLabel := "gui/" + strconv.Itoa(os.Getuid()) + "/" + label
	output, err := c.runner()(ctx, "launchctl", "kickstart", "-k", launchLabel)
	if err != nil {
		return fmt.Errorf("launchd restart failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// Switch validates the requested target before persisting or restarting. The
// returned state is only authoritative after all health checks pass.
func (c *Controller) Switch(ctx context.Context, mode network.BindingMode, interfaceID string) (network.RuntimeState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rebinding.Load() {
		return c.State, errors.New("another interface switch is already in progress")
	}
	c.rebinding.Store(true)
	defer c.rebinding.Store(false)

	interfaces, err := c.interfaces()
	if err != nil {
		return c.State, fmt.Errorf("enumerate interfaces: %w", err)
	}
	state := c.State
	result, err := network.SwitchBinding(ctx, state, mode, interfaceID, interfaces, network.SwitchHooks{
		SaveState:     c.save,
		ValidateCaddy: c.Caddy.Validate,
		Restart:       c.restart,
		WaitWeb: func(ctx context.Context, state network.RuntimeState) error {
			return waitService(ctx, state.WebBindAddress, state.WebPort, "/api/health", c.PairingToken)
		},
		WaitAPI: func(ctx context.Context, state network.RuntimeState) error {
			return waitService(ctx, state.APIBindAddress, state.APIPort, "/api/health", "")
		},
		ApplyCaddy:  c.Caddy.Apply,
		VerifyCaddy: c.Caddy.WaitHealthy,
		VerifyPublicURL: func(ctx context.Context, state network.RuntimeState) error {
			if strings.TrimSpace(state.PublicHealthCheckURL) == "" {
				return nil
			}
			return waitURL(ctx, state.PublicHealthCheckURL, "")
		},
	})
	if err != nil {
		c.State.LastError = err.Error()
		return c.State, err
	}
	c.State = result
	return result, nil
}

// ConfigureProxy changes only the managed Caddy state. The library catalog
// and the web/API listeners are untouched; binding swaps use Switch instead.
func (c *Controller) ConfigureProxy(ctx context.Context, enabled bool, domain, healthURL, bindAddress string) (network.RuntimeState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.State
	candidate := previous
	candidate.CaddyEnabled = enabled
	if strings.TrimSpace(domain) != "" {
		candidate.PublicDomain = strings.TrimSpace(domain)
	}
	if strings.TrimSpace(healthURL) != "" || !enabled {
		candidate.PublicHealthCheckURL = strings.TrimSpace(healthURL)
	}
	if strings.TrimSpace(bindAddress) != "" {
		candidate.CaddyBindAddress = strings.TrimSpace(bindAddress)
	}
	manager := c.Caddy
	manager.Enabled = enabled
	if enabled {
		if err := manager.Validate(ctx, candidate); err != nil {
			return c.proxyFailure(previous, err)
		}
		if err := manager.Apply(ctx, candidate); err != nil {
			return c.proxyFailure(previous, err)
		}
	} else if previous.CaddyEnabled {
		if err := c.Caddy.Stop(ctx); err != nil {
			return c.proxyFailure(previous, err)
		}
	}
	if err := c.save(candidate); err != nil {
		if enabled {
			rollbackManager := c.Caddy
			rollbackManager.Enabled = previous.CaddyEnabled
			if previous.CaddyEnabled {
				_ = rollbackManager.Apply(context.Background(), previous)
			} else {
				_ = rollbackManager.Stop(context.Background())
			}
		} else if previous.CaddyEnabled {
			rollbackManager := c.Caddy
			rollbackManager.Enabled = true
			_ = rollbackManager.Apply(context.Background(), previous)
		}
		return previous, err
	}
	c.Caddy = manager
	c.State = candidate
	return candidate, nil
}

func (c *Controller) proxyFailure(previous network.RuntimeState, err error) (network.RuntimeState, error) {
	previous.LastError = err.Error()
	c.State = previous
	_ = c.save(previous)
	return previous, err
}

func (c *Controller) Status(ctx context.Context) (Status, error) {
	c.mu.Lock()
	state := c.State
	caddy := c.Caddy
	c.mu.Unlock()
	interfaces, err := c.interfaces()
	if err != nil {
		return Status{}, err
	}
	webHealthy := checkService(ctx, state.WebBindAddress, state.WebPort, "/api/health", c.PairingToken) == nil
	apiHealthy := checkService(ctx, state.APIBindAddress, state.APIPort, "/api/health", "") == nil
	status := Status{
		State:               state,
		Interfaces:          interfaces,
		WebHealthy:          webHealthy,
		APIHealthy:          apiHealthy,
		Rebinding:           c.rebinding.Load(),
		Caddy:               caddy.Status(ctx, state),
		DatabasePublic:      false,
		PublicPrerequisites: []string{"DNS for the configured domain must point to this network", "router/firewall TCP 80 and 443 forwarding must target this host", "WireGuard/VPN routes must be configured externally"},
		LastError:           state.LastError,
	}
	if !state.ProcessStartTime.IsZero() {
		status.UptimeSeconds = int64(time.Since(state.ProcessStartTime).Seconds())
		if status.UptimeSeconds < 0 {
			status.UptimeSeconds = 0
		}
	}
	return status, nil
}

func waitService(ctx context.Context, bind string, port int, path, token string) error {
	host := bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
	if token != "" {
		url += "?token=" + token
	}
	return waitURL(ctx, url, token)
}

func checkService(ctx context.Context, bind string, port int, path, token string) error {
	host := bind
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + path
	return checkURL(ctx, url, token)
}

func waitURL(ctx context.Context, rawURL, token string) error {
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	var last error
	for {
		last = checkURL(ctx, rawURL, token)
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return last
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func checkURL(ctx context.Context, rawURL, token string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
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

// MarshalStatus is a small convenience for command-line callers that need a
// stable JSON representation without coupling to an HTTP response writer.
func MarshalStatus(status Status) ([]byte, error) {
	return json.MarshalIndent(status, "", "  ")
}
