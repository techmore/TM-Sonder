package network

import (
	"context"
	"fmt"
	"net"
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
	binding, err := Resolve(mode, interfaceID, interfaces)
	if err != nil {
		return current, fmt.Errorf("validate target interface: %w", err)
	}
	if hooks.SaveState == nil {
		return current, fmt.Errorf("switch binding: runtime-state writer is not configured")
	}
	if hooks.Restart == nil {
		return current, fmt.Errorf("switch binding: process restart is not configured")
	}

	candidate := current
	candidate.SelectedMode = binding.Mode
	candidate.SelectedInterface = binding.InterfaceID
	candidate.SelectedIPv4 = binding.IPv4
	candidate.WebBindAddress = binding.WebBindAddress
	candidate.APIBindAddress = binding.APIBindAddress
	candidate.CaddyUpstream = net.JoinHostPort(binding.IPv4, strings.TrimSpace(fmt.Sprint(candidate.WebPort)))
	candidate.ProcessStartTime = time.Now().UTC()
	candidate.LastError = ""

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
