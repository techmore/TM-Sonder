package zeroconf

import (
	"os/exec"
	"testing"
	"time"
)

func TestStartStopLifecycle(t *testing.T) {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		t.Skip("dns-sd not available")
	}
	a, err := Start("TM Sonder Test", 18799)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Running() {
		t.Fatal("advertiser not running after Start")
	}
	time.Sleep(300 * time.Millisecond) // let dns-sd register
	a.Stop()
	a.Stop() // double-stop must not panic
	if a.Running() {
		t.Error("still running after Stop")
	}
}

func TestStartInvalidPortFails(t *testing.T) {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		t.Skip("dns-sd not available")
	}
	// Port 0 is rejected by dns-sd; we only assert no hang/panic.
	a, err := Start("TM Sonder Bad", 0)
	if err == nil && a != nil {
		a.Stop()
	}
}
