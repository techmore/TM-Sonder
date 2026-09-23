package proxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tm-sonder/server/internal/network"
)

func TestRenderCaddyfilePreservesTLSAndUpdatesUpstream(t *testing.T) {
	state := network.RuntimeState{
		PublicDomain:     "books.example.com",
		CaddyBindAddress: "0.0.0.0",
		CaddyUpstream:    "192.168.1.22:8797",
	}
	existing := []byte("books.example.com {\n    tls internal\n    reverse_proxy 127.0.0.1:8797\n}\n")
	got, err := RenderCaddyfile(state, existing)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{
		"books.example.com {", "bind 0.0.0.0", "tls internal", "reverse_proxy 192.168.1.22:8797",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Caddyfile missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "reverse_proxy 127.0.0.1:8797") {
		t.Error("old upstream survived the update")
	}
}

func TestRenderCaddyfileRequiresDomainAndUpstream(t *testing.T) {
	if _, err := RenderCaddyfile(network.RuntimeState{CaddyUpstream: "127.0.0.1:8797"}, nil); err == nil {
		t.Fatal("accepted missing public domain")
	}
	if _, err := RenderCaddyfile(network.RuntimeState{PublicDomain: "sonder.example"}, nil); err == nil {
		t.Fatal("accepted missing upstream")
	}
}

func TestApplyRestoresCaddyfileWhenReloadFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	original := []byte("old.example {\n    tls internal\n    reverse_proxy 127.0.0.1:8797\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	m := Manager{
		Enabled:    true,
		Binary:     "caddy",
		ConfigPath: path,
		Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "validate" {
				return nil, nil
			}
			return []byte("reload failed"), os.ErrInvalid
		},
	}
	state := network.RuntimeState{PublicDomain: "new.example", CaddyUpstream: "192.168.1.20:8797"}
	if err := m.Apply(context.Background(), state); err == nil {
		t.Fatal("expected reload failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("Caddyfile was not restored:\n%s", got)
	}
}
