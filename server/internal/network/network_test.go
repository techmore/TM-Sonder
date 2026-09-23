package network

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectOnlyActiveIPv4Interfaces(t *testing.T) {
	ifs := []net.Interface{
		{Name: "lo0", Flags: net.FlagUp | net.FlagLoopback},
		{Name: "en0", Flags: net.FlagUp},
		{Name: "en1", Flags: 0},
		{Name: "utun4", Flags: net.FlagUp},
		{Name: "en2", Flags: net.FlagUp},
	}
	addresses := map[string][]net.Addr{
		"lo0":   {&net.IPNet{IP: net.ParseIP("127.0.0.1")}},
		"en0":   {&net.IPNet{IP: net.ParseIP("192.168.1.20")}},
		"en1":   {&net.IPNet{IP: net.ParseIP("192.168.1.21")}},
		"utun4": {&net.IPNet{IP: net.ParseIP("10.8.0.2")}},
		"en2":   {&net.IPNet{IP: net.ParseIP("2001:db8::2")}},
	}
	out := collect(ifs, func(name string) ([]net.Addr, error) { return addresses[name], nil })
	if len(out) != 3 {
		t.Fatalf("collected %d interfaces, want 3: %+v", len(out), out)
	}
	if out[0].Type != TypeLoopback || out[0].IPv4 != "127.0.0.1" {
		t.Fatalf("loopback = %+v", out[0])
	}
	for _, item := range out {
		if item.ID == "en1" || item.ID == "en2" {
			t.Fatalf("inactive/IPv6 interface leaked into result: %+v", item)
		}
	}
}

func TestResolveBindingModes(t *testing.T) {
	interfaces := []Interface{
		{ID: "lo0", Type: TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true},
		{ID: "en0", Type: TypeWiFiLAN, IPv4: "192.168.1.20", Active: true},
		{ID: "en1", Type: TypeEthernet, IPv4: "192.168.1.21", Active: true},
		{ID: "utun4", Type: TypeVPN, IPv4: "10.8.0.2", Active: true},
		{ID: "en9", Type: TypeEthernet, IPv4: "", Active: true},
	}
	tests := []struct {
		name, id string
		mode     BindingMode
		web      string
		selected string
	}{
		{"loopback", "", ModeLoopback, "127.0.0.1", "lo0"},
		{"wifi", "", ModeWiFiLAN, "192.168.1.20", "en0"},
		{"ethernet", "", ModeEthernet, "192.168.1.21", "en1"},
		{"vpn", "", ModeVPN, "10.8.0.2", "utun4"},
		{"exact", "en1", ModeExact, "192.168.1.21", "en1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.mode, tc.id, interfaces)
			if err != nil {
				t.Fatal(err)
			}
			if got.WebBindAddress != tc.web || got.InterfaceID != tc.selected {
				t.Fatalf("binding = %+v", got)
			}
			if got.APIBindAddress != "127.0.0.1" {
				t.Fatalf("API bind = %q", got.APIBindAddress)
			}
		})
	}
	public, err := Resolve(ModePublic, "", interfaces)
	if err != nil || public.WebBindAddress != "0.0.0.0" || public.IPv4 != "192.168.1.20" {
		t.Fatalf("public binding = %+v, err=%v", public, err)
	}
	if _, err := Resolve(ModeExact, "en9", interfaces); err == nil {
		t.Fatal("accepted active interface without IPv4")
	}
}

func TestRuntimeStateRoundTripIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime-state.json")
	want := RuntimeState{
		SelectedMode: ModeWiFiLAN, SelectedInterface: "en0", SelectedIPv4: "192.168.1.20",
		WebBindAddress: "192.168.1.20", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
		PublicDomain: "sonder.example.com", PublicHealthCheckURL: "https://sonder.example.com/api/health",
		CaddyEnabled: true, CaddyBindAddress: "0.0.0.0", CaddyUpstream: "192.168.1.20:8797",
		ProcessStartTime: time.Now().UTC().Truncate(time.Second),
	}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedInterface != want.SelectedInterface || got.CaddyUpstream != want.CaddyUpstream ||
		!got.ProcessStartTime.Equal(want.ProcessStartTime) {
		t.Fatalf("state round trip = %+v, want %+v", got, want)
	}
}
