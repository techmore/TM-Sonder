package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// SwitchHooks contains the side effects needed to move a running instance to
// a different interface. Keeping them as callbacks makes the ordering and
// rollback behavior testable without stopping a real launchd service.
type SwitchHooks struct {
	SaveState       func(RuntimeState) error
	ValidateCaddy   func(context.Context, RuntimeState) error
	Restart         func(context.Context, RuntimeState) error
	WaitWeb         func(context.Context, RuntimeState) error
	WaitAPI         func(context.Context, RuntimeState) error
	ApplyCaddy      func(context.Context, RuntimeState) error
	VerifyCaddy     func(context.Context, RuntimeState) error
	VerifyPublicURL func(context.Context, RuntimeState) error
}

// SwitchBinding validates the target before any process or file is touched,
// then performs the swap in a fixed order. If a post-save stage fails, the
// previous runtime state and proxy configuration are restored on a best-effort
// basis and the original failure is returned with its stage name.
func SwitchBinding(ctx context.Context, current RuntimeState, mode BindingMode, interfaceID string, interfaces []Interface, hooks SwitchHooks) (RuntimeState, error) {
	return SwitchExposure(ctx, current, mode, interfaceID, current.WebPort, current.APIPort, interfaces, hooks)
}

// SwitchPorts applies a port-only change while retaining the current binding
// mode and selected interface. It is the narrow operation used by clients
// that only need to change what port is exposed.
func SwitchPorts(ctx context.Context, current RuntimeState, webPort, apiPort int, interfaces []Interface, hooks SwitchHooks) (RuntimeState, error) {
	mode := current.SelectedMode
	if mode == "" {
		mode = ModeLoopback
	}
	return SwitchExposure(ctx, current, mode, current.SelectedInterface, webPort, apiPort, interfaces, hooks)
}

// SwitchExposure atomically changes the selected interface and/or listener
// ports. Validation happens before state is saved or a process is restarted.
func SwitchExposure(ctx context.Context, current RuntimeState, mode BindingMode, interfaceID string, webPort, apiPort int, interfaces []Interface, hooks SwitchHooks) (RuntimeState, error) {
	candidate, err := PlanExposure(current, mode, interfaceID, webPort, apiPort, interfaces)
	if err != nil {
		return current, fmt.Errorf("validate network exposure: %w", err)
	}
	return applyExposure(ctx, current, candidate, hooks)
}

// PlanExposure produces the complete runtime state for a prospective exposure
// change without causing any side effects. A zero port means retain the
// currently active port, which lets PATCH-style clients change only one side.
func PlanExposure(current RuntimeState, mode BindingMode, interfaceID string, webPort, apiPort int, interfaces []Interface) (RuntimeState, error) {
	if webPort <= 0 {
		webPort = current.WebPort
	}
	if apiPort <= 0 {
		apiPort = current.APIPort
	}
	if err := validatePorts(webPort, apiPort); err != nil {
		return current, err
	}
	if mode == "" {
		mode = current.SelectedMode
	}
	if mode == "" {
		mode = ModeLoopback
	}
	if interfaceID == "" && mode == ModeExact {
		interfaceID = current.SelectedInterface
	}
	binding, err := Resolve(mode, interfaceID, interfaces)
	if err != nil {
		return current, err
	}
	candidate := current
	candidate.SelectedMode = binding.Mode
	candidate.SelectedInterface = binding.InterfaceID
	candidate.SelectedIPv4 = binding.IPv4
	candidate.WebBindAddress = binding.WebBindAddress
	// The API remains private even when the web listener is moved to LAN/VPN.
	candidate.APIBindAddress = "127.0.0.1"
	candidate.WebPort = webPort
	candidate.APIPort = apiPort
	candidate.CaddyUpstream = net.JoinHostPort(binding.IPv4, strconv.Itoa(webPort))
	candidate.ProcessStartTime = time.Now().UTC()
	candidate.LastError = ""
	return candidate, nil
}

func validatePorts(webPort, apiPort int) error {
	if webPort < 1 || webPort > 65535 {
		return fmt.Errorf("web port must be between 1 and 65535")
	}
	if apiPort < 1 || apiPort > 65535 {
		return fmt.Errorf("API port must be between 1 and 65535")
	}
	if webPort == apiPort {
		return fmt.Errorf("web port and API port must be different")
	}
	return nil
}

func applyExposure(ctx context.Context, current, candidate RuntimeState, hooks SwitchHooks) (RuntimeState, error) {
	if hooks.SaveState == nil {
		return current, fmt.Errorf("switch exposure: runtime-state writer is not configured")
	}
	if hooks.Restart == nil {
		return current, fmt.Errorf("switch exposure: process restart is not configured")
	}

	// Caddy syntax is checked before persisting state or asking launchd to stop
	// anything. This is the important guard against a bad proxy edit taking the
	// web service down with it.
	if candidate.CaddyEnabled && hooks.ValidateCaddy != nil {
		if err := hooks.ValidateCaddy(ctx, candidate); err != nil {
			return current, fmt.Errorf("validate Caddy before swap: %w", err)
		}
	}

	if err := hooks.SaveState(candidate); err != nil {
		return current, fmt.Errorf("save candidate runtime state: %w", err)
	}

	fail := func(stage string, cause error) (RuntimeState, error) {
		message := fmt.Errorf("%s: %w", stage, cause)
		rollback := current
		rollback.LastError = message.Error()
		_ = hooks.SaveState(rollback)
		if candidate.CaddyEnabled && hooks.ApplyCaddy != nil {
			_ = hooks.ApplyCaddy(context.Background(), current)
		}
		_ = hooks.Restart(context.Background(), current)
		return current, message
	}

	if err := hooks.Restart(ctx, candidate); err != nil {
		return fail("restart web + API", err)
	}
	if hooks.WaitWeb != nil {
		if err := hooks.WaitWeb(ctx, candidate); err != nil {
			return fail("verify local web health", err)
		}
	}
	if hooks.WaitAPI != nil {
		if err := hooks.WaitAPI(ctx, candidate); err != nil {
			return fail("verify API readiness", err)
		}
	}
	if candidate.CaddyEnabled {
		if hooks.ApplyCaddy != nil {
			if err := hooks.ApplyCaddy(ctx, candidate); err != nil {
				return fail("update Caddy", err)
			}
		}
		if hooks.VerifyCaddy != nil {
			if err := hooks.VerifyCaddy(ctx, candidate); err != nil {
				return fail("verify Caddy listener", err)
			}
		}
		if strings.TrimSpace(candidate.PublicHealthCheckURL) != "" && hooks.VerifyPublicURL != nil {
			if err := hooks.VerifyPublicURL(ctx, candidate); err != nil {
				return fail("verify public HTTPS health", err)
			}
		}
	}
	return candidate, nil
}
