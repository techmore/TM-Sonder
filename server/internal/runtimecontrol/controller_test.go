package runtimecontrol

import (
	"context"
	"testing"

	"tm-sonder/server/internal/network"
)

func TestPlanExposureRetainsSelectedInterfaceForPortOnlyChanges(t *testing.T) {
	interfaces := []network.Interface{
		{ID: "lo0", Type: network.TypeLoopback, IPv4: "127.0.0.1", Active: true, Loopback: true},
		{ID: "en0", Type: network.TypeWiFiLAN, IPv4: "192.168.1.20", Active: true},
		{ID: "en1", Type: network.TypeWiFiLAN, IPv4: "192.168.1.21", Active: true},
	}
	controller := &Controller{
		State: network.RuntimeState{
			SelectedMode: network.ModeWiFiLAN, SelectedInterface: "en1", SelectedIPv4: "192.168.1.21",
			WebBindAddress: "192.168.1.21", APIBindAddress: "127.0.0.1", WebPort: 8797, APIPort: 8798,
		},
		InterfacesFn: func() ([]network.Interface, error) { return interfaces, nil },
	}
	got, err := controller.PlanExposure(context.Background(), "", "", 18897, 18898)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedInterface != "en1" || got.WebBindAddress != "192.168.1.21" || got.WebPort != 18897 || got.APIPort != 18898 {
		t.Fatalf("planned port-only exposure changed binding: %+v", got)
	}
}
