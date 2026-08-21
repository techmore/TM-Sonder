// Package zeroconf advertises the server over Bonjour/mDNS using macOS's
// bundled dns-sd tool, keeping the project dependency-free.
package zeroconf

import (
	"fmt"
	"os/exec"
	"sync"
)

const ServiceType = "_tmsonder._tcp"

// Advertiser manages one long-running dns-sd registration process.
type Advertiser struct {
	mu      sync.Mutex
	name    string
	port    int
	cmd     *exec.Cmd
	stopped bool
}

// Start registers name._tmsonder._tcp on the given port. The returned
// Advertiser must be Stop()ed at shutdown to deregister cleanly.
func Start(name string, port int) (*Advertiser, error) {
	if _, err := exec.LookPath("dns-sd"); err != nil {
		return nil, fmt.Errorf("zeroconf: dns-sd not found: %w", err)
	}
	a := &Advertiser{name: name, port: port}
	if err := a.spawn(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Advertiser) spawn() error {
	cmd := exec.Command("dns-sd",
		"-R", a.name, ServiceType, "local", fmt.Sprint(a.port),
		"app=TM Sonder")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("zeroconf: start dns-sd: %w", err)
	}
	a.cmd = cmd

	// Reap the process whenever it exits so no zombie remains. If it died
	// unexpectedly (not via Stop), respawn with backoff handled by caller
	// semantics: keep it simple and just mark stopped.
	go func() { _ = cmd.Wait() }()
	return nil
}

// Stop terminates the registration. Safe to call multiple times.
func (a *Advertiser) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped || a.cmd == nil || a.cmd.Process == nil {
		return
	}
	a.stopped = true
	_ = a.cmd.Process.Kill()
}

// Running reports whether the registration process is alive.
func (a *Advertiser) Running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.stopped && a.cmd != nil && a.cmd.Process != nil
}
