package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tm-sonder/server/internal/network"
	"tm-sonder/server/internal/runtimecontrol"
)

func TestNetworkInterfacesEndpointReturnsActiveIPv4Inventory(t *testing.T) {
	f := newFixture(t, nil)
	resp, body := get(t, f.ts.URL+"/api/network/interfaces")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	var payload struct {
		Interfaces []network.Interface `json:"interfaces"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Interfaces) == 0 {
		t.Fatal("expected at least loopback on the test host")
	}
	for _, item := range payload.Interfaces {
		if !item.Active || item.IPv4 == "" {
			t.Fatalf("inactive or IPv6-only interface leaked: %+v", item)
		}
	}
}

func TestNetworkStatusEndpointIncludesPrivateDefaults(t *testing.T) {
	f := newFixture(t, nil)
	interfaces := []network.Interface{{ID: "lo0", Type: network.TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true}}
	controller := &runtimecontrol.Controller{
		State: network.RuntimeState{
			SelectedMode: network.ModeLoopback, SelectedInterface: "lo0", SelectedIPv4: "127.0.0.1",
			WebBindAddress: "127.0.0.1", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
		},
		InterfacesFn: func() ([]network.Interface, error) { return interfaces, nil },
	}
	f.s.SetRuntimeControl(controller)
	req := httptest.NewRequest(http.MethodGet, "/api/network/status", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Status struct {
			State struct {
				APIBindAddress string `json:"apiBindAddress"`
				CaddyEnabled   bool   `json:"caddyEnabled"`
			} `json:"state"`
			DatabasePublic bool `json:"databasePublic"`
		} `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status.State.APIBindAddress != "127.0.0.1" || payload.Status.State.CaddyEnabled || payload.Status.DatabasePublic {
		t.Fatalf("private defaults not reflected: %+v", payload)
	}
}

func TestWebHandlerProxiesThroughPrivateAPIListener(t *testing.T) {
	f := newFixture(t, nil)
	private := httptest.NewServer(f.s.Handler())
	defer private.Close()
	target, err := url.Parse(private.URL)
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(f.s.WebHandler(target.Host))
	defer front.Close()
	resp, body := get(t, front.URL+"/api/health")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"id":"tm-sonder"`) {
		t.Fatalf("frontend proxy response = %d %s", resp.StatusCode, body)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	req.Host = "192.168.1.10:8797"
	rec := httptest.NewRecorder()
	f.s.WebHandler(target.Host).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote frontend status = %d, want 403", rec.Code)
	}
}

func TestNetworkExposureRejectsPortCollisionBeforeRestart(t *testing.T) {
	f := newFixture(t, nil)
	interfaces := []network.Interface{{ID: "lo0", Type: network.TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true}}
	f.s.SetRuntimeControl(&runtimecontrol.Controller{
		State: network.RuntimeState{
			SelectedMode: network.ModeLoopback, SelectedInterface: "lo0", SelectedIPv4: "127.0.0.1",
			WebBindAddress: "127.0.0.1", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
		},
		InterfacesFn: func() ([]network.Interface, error) { return interfaces, nil },
	})
	req := httptest.NewRequest(http.MethodPost, "/api/network/exposure", strings.NewReader(`{"webPort":18897,"apiPort":18897}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1:8797"
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "must be different") {
		t.Fatalf("collision response = %d %s", rec.Code, rec.Body.String())
	}
}
