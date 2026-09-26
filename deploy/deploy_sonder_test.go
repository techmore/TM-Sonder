package main

// Exercises deploy/deploy-sonder.sh without touching the real server: a fake
// systemctl, a fake curl, and a stand-in binary whose ELF header says whatever
// the test wants. The architecture guard and the rollback are the two paths that
// would do real damage if they were wrong, so they are driven directly rather
// than trusted.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// elfHeader writes a minimal ELF header with the given e_machine value.
func elfHeader(t *testing.T, path string, machine uint16) {
	t.Helper()
	b := make([]byte, 64)
	b[0], b[1], b[2], b[3] = 0x7f, 'E', 'L', 'F'
	b[4] = 2 // 64-bit
	b[5] = 1 // little endian
	b[18] = byte(machine)
	b[19] = byte(machine >> 8)
	if err := os.WriteFile(path, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

const EM_X86_64 = 0x3e
const EM_AARCH64 = 0xb7

type harness struct {
	dir      string
	repo     string
	config   string
	binDir   string
	version  string // what the fake API reports
	failExec bool   // make `systemctl --user restart` fail
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{
		dir:     dir,
		repo:    filepath.Join(dir, "TM-Sonder"),
		version: "abc123def456",
	}
	h.binDir = filepath.Join(h.repo, "bin")
	if err := os.MkdirAll(h.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	h.config = filepath.Join(dir, "server.json")
	if err := os.WriteFile(h.config, []byte(`{"pairingToken":"tok","apiPort":8097}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The binary already in service.
	elfHeader(t, filepath.Join(h.binDir, "sonder-linux-amd64"), EM_X86_64)
	return h
}

// stubs writes fake systemctl and curl onto PATH.
func (h *harness) stubs(t *testing.T) {
	t.Helper()
	bindir := filepath.Join(h.dir, "stubs")
	if err := os.MkdirAll(bindir, 0o755); err != nil {
		t.Fatal(err)
	}
	// `systemctl --user restart tm-sonder` puts "restart" in $2, and the
	// rollback path calls it again, so the stub looks for the verb anywhere in
	// the arguments rather than at a fixed position.
	systemctl := "#!/usr/bin/env bash\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = restart ]; then\n" +
		"    if [ -n \"$FAKE_RESTART_FAILS\" ]; then echo 'stub: restart refused' >&2; exit 1; fi\n" +
		"    exit 0\n" +
		"  fi\n" +
		"  if [ \"$a\" = is-active ]; then echo active; exit 0; fi\n" +
		"done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(bindir, "systemctl"), []byte(systemctl), 0o755); err != nil {
		t.Fatal(err)
	}
	curl := "#!/usr/bin/env bash\n" +
		"if [ -n \"$FAKE_CURL_FAILS\" ]; then exit 7; fi\n" +
		"printf '{\"version\":\"%s\",\"itemCount\":21738}' \"$FAKE_VERSION\"\n"
	if err := os.WriteFile(filepath.Join(bindir, "curl"), []byte(curl), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bindir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// run returns stdout and stderr joined, because the script's failures are
// reported with die(), which writes to stderr. Asserting on stdout alone would
// have every rejection test pass vacuously.
func (h *harness) run(t *testing.T, expectSuccess bool, args ...string) (string, string) {
	t.Helper()
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot, "deploy", "deploy-sonder.sh")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(),
		"SONDER_REPO="+h.repo,
		"SONDER_CONFIG="+h.config,
		"FAKE_VERSION="+h.version,
		"SONDER_HEALTH_ATTEMPTS=2",
		"SONDER_HEALTH_INTERVAL=0",
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if expectSuccess && err != nil {
		t.Fatalf("expected success, got %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !expectSuccess && err == nil {
		t.Fatalf("expected failure, got success\nstdout: %s", stdout.String())
	}
	return stdout.String(), stderr.String()
}

func (h *harness) stage(t *testing.T, machine uint16) {
	t.Helper()
	elfHeader(t, filepath.Join(h.binDir, ".sonder-linux-amd64.incoming"), machine)
}

func TestDeployRefusesAnArmBinaryOnThisX86Host(t *testing.T) {
	// Without this guard the failure surfaces at exec time as a permission or
	// exec-format error, which reads like a broken filesystem rather than a
	// binary built for the wrong architecture.
	h := newHarness(t)
	h.stubs(t)
	h.stage(t, EM_AARCH64)

	out, errOut := h.run(t, false)
	out += errOut
	if !strings.Contains(out, "aarch64") {
		t.Errorf("output should name the wrong architecture, got: %s", out)
	}
	// The running binary must be untouched.
	if _, err := os.Stat(filepath.Join(h.binDir, "sonder-linux-amd64")); err != nil {
		t.Errorf("the existing binary was removed on a rejected deploy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.binDir, ".sonder-linux-amd64.incoming")); err != nil {
		t.Errorf("the rejected artifact should be left for inspection: %v", err)
	}
}

func TestDeployRefusesWhenThereIsNoBinaryInService(t *testing.T) {
	h := newHarness(t)
	h.stubs(t)
	if err := os.Remove(filepath.Join(h.binDir, "sonder-linux-amd64")); err != nil {
		t.Fatal(err)
	}
	h.stage(t, EM_X86_64)
	h.run(t, false)
}

func TestDeployInstallsAndConfirmsTheReportedVersion(t *testing.T) {
	h := newHarness(t)
	h.stubs(t)
	h.stage(t, EM_X86_64)

	out, _ := h.run(t, true, "abc123def456")
	if !strings.Contains(out, "serving abc123def456") {
		t.Errorf("expected the served version to be confirmed, got: %s", out)
	}
	// The staged artifact is consumed, and a backup of the old binary remains.
	if _, err := os.Stat(filepath.Join(h.binDir, ".sonder-linux-amd64.incoming")); !os.IsNotExist(err) {
		t.Errorf("staged artifact should be moved into place, err=%v", err)
	}
	entries, _ := filepath.Glob(filepath.Join(h.binDir, "sonder-linux-amd64.bak-*"))
	if len(entries) != 1 {
		t.Errorf("expected exactly one backup, got %v", entries)
	}
}

func TestDeployRollsBackWhenTheVersionDoesNotMatch(t *testing.T) {
	// A process can be up and still serving the old build, so the check is on
	// the version the API reports rather than on liveness.
	h := newHarness(t)
	h.stubs(t)
	h.version = "theoldbuild"
	before, err := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	if err != nil {
		t.Fatal(err)
	}
	h.stage(t, EM_X86_64)

	out, errOut := h.run(t, false, "thenewbuild")
	out += errOut
	if !strings.Contains(out, "version mismatch") {
		t.Errorf("expected a version-mismatch rollback, got: %s", out)
	}
	after, err := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the previous binary was not restored")
	}
}

func TestDeployRollsBackWhenTheServerNeverAnswers(t *testing.T) {
	h := newHarness(t)
	h.stubs(t)
	before, _ := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	t.Setenv("FAKE_CURL_FAILS", "1")
	h.stage(t, EM_X86_64)

	out, errOut := h.run(t, false, "v1")
	out += errOut
	if !strings.Contains(out, "rolling back") {
		t.Errorf("expected a rollback when the API never answers, got: %s", out)
	}
	after, _ := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	if string(before) != string(after) {
		t.Error("the previous binary was not restored")
	}
}

func TestDeployRollsBackWhenTheServiceWillNotRestart(t *testing.T) {
	h := newHarness(t)
	h.stubs(t)
	before, _ := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	t.Setenv("FAKE_RESTART_FAILS", "1")
	h.stage(t, EM_X86_64)

	out, errOut := h.run(t, false, "v1")
	out += errOut
	if !strings.Contains(out, "restart failed") {
		t.Errorf("expected a rollback on restart failure, got: %s", out)
	}
	after, _ := os.ReadFile(filepath.Join(h.binDir, "sonder-linux-amd64"))
	if string(before) != string(after) {
		t.Error("the previous binary was not restored")
	}
}

func TestDeployKeepsEveryBackupRatherThanOverwriting(t *testing.T) {
	// Each deploy leaves its own dated backup; two deploys must not collapse
	// into one, or there is no way back further than one revision.
	h := newHarness(t)
	h.stubs(t)
	// Both deploys expect the version the stub reports, or the version guard
	// (correctly) rolls them back and there is nothing to compare.
	h.stage(t, EM_X86_64)
	h.run(t, true, h.version)
	h.stage(t, EM_X86_64)
	h.run(t, true, h.version)

	entries, _ := filepath.Glob(filepath.Join(h.binDir, "sonder-linux-amd64.bak-*"))
	if len(entries) != 2 {
		t.Errorf("expected two dated backups after two deploys, got %d: %v", len(entries), entries)
	}
	fmt.Fprintln(os.Stderr, "backups:", entries)
}
