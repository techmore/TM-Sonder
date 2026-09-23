package network

import (
	"context"
	"reflect"
	"testing"
)

func TestSwitchBindingValidatesBeforeSideEffectsAndRunsChecksInOrder(t *testing.T) {
	current := RuntimeState{
		SelectedMode: ModeLoopback, SelectedInterface: "lo0", SelectedIPv4: "127.0.0.1",
		WebBindAddress: "127.0.0.1", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
		CaddyEnabled: true, PublicDomain: "books.example", PublicHealthCheckURL: "https://books.example/api/health",
	}
	interfaces := []Interface{
		{ID: "lo0", Type: TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true},
		{ID: "en0", Type: TypeWiFiLAN, IPv4: "192.168.1.20", Active: true},
	}
	var events []string
	hooks := SwitchHooks{
		SaveState: func(RuntimeState) error { events = append(events, "save"); return nil },
		ValidateCaddy: func(context.Context, RuntimeState) error {
			events = append(events, "validate-caddy")
			return nil
		},
		Restart:         func(context.Context, RuntimeState) error { events = append(events, "restart"); return nil },
		WaitWeb:         func(context.Context, RuntimeState) error { events = append(events, "web-health"); return nil },
		WaitAPI:         func(context.Context, RuntimeState) error { events = append(events, "api-ready"); return nil },
		ApplyCaddy:      func(context.Context, RuntimeState) error { events = append(events, "apply-caddy"); return nil },
		VerifyCaddy:     func(context.Context, RuntimeState) error { events = append(events, "caddy-health"); return nil },
		VerifyPublicURL: func(context.Context, RuntimeState) error { events = append(events, "public-health"); return nil },
	}
	got, err := SwitchBinding(context.Background(), current, ModeWiFiLAN, "", interfaces, hooks)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{"validate-caddy", "save", "restart", "web-health", "api-ready", "apply-caddy", "caddy-health", "public-health"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if got.SelectedInterface != "en0" || got.WebBindAddress != "192.168.1.20" || got.CaddyUpstream != "192.168.1.20:8797" {
		t.Fatalf("candidate state = %+v", got)
	}

	events = nil
	if _, err := SwitchBinding(context.Background(), current, ModeExact, "en9", interfaces, hooks); err == nil {
		t.Fatal("accepted an unavailable interface")
	}
	if len(events) != 0 {
		t.Fatalf("side effects ran before validation: %v", events)
	}
}

func TestSwitchBindingRestoresPreviousStateAfterFailedHealthCheck(t *testing.T) {
	current := RuntimeState{
		SelectedMode: ModeLoopback, SelectedInterface: "lo0", SelectedIPv4: "127.0.0.1",
		WebBindAddress: "127.0.0.1", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
	}
	interfaces := []Interface{
		{ID: "lo0", Type: TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true},
		{ID: "en0", Type: TypeWiFiLAN, IPv4: "192.168.1.20", Active: true},
	}
	var saved []RuntimeState
	var restarts int
	_, err := SwitchBinding(context.Background(), current, ModeWiFiLAN, "", interfaces, SwitchHooks{
		SaveState: func(state RuntimeState) error { saved = append(saved, state); return nil },
		Restart:   func(context.Context, RuntimeState) error { restarts++; return nil },
		WaitWeb:   func(context.Context, RuntimeState) error { return context.Canceled },
	})
	if err == nil || len(saved) != 2 || restarts != 2 {
		t.Fatalf("failed swap did not roll back: err=%v saved=%d restarts=%d", err, len(saved), restarts)
	}
	if saved[1].SelectedInterface != current.SelectedInterface || saved[1].LastError == "" {
		t.Fatalf("rollback state = %+v", saved[1])
	}
}
