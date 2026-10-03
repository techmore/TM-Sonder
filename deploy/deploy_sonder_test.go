package main

// These tests exercise the Incus deployment script using a disposable fake
// instance filesystem. They cover architecture rejection, success, health
// checks, and rollback without contacting the production daemon.
import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const emX8664 = 0x3e
const emAarch64 = 0xb7

type deployHarness struct {
	dir, repo, instanceRoot string
	version                 string
}

func writeELF(t *testing.T, path string, machine uint16) {
	t.Helper()
	b := make([]byte, 64)
	b[0], b[1], b[2], b[3] = 0x7f, 'E', 'L', 'F'
	b[4], b[5] = 2, 1
	b[18], b[19] = byte(machine), byte(machine>>8)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o755); err != nil {
		t.Fatal(err)
	}
}

func newDeployHarness(t *testing.T) *deployHarness {
	t.Helper()
	dir := t.TempDir()
	h := &deployHarness{dir: dir, repo: filepath.Join(dir, "repo"), instanceRoot: filepath.Join(dir, "instance"), version: "abc123def456"}
	writeELF(t, filepath.Join(h.repo, "bin", ".sonder-linux-amd64.incoming"), emX8664)
	writeELF(t, filepath.Join(h.instanceRoot, "usr/local/bin/sonder"), emX8664)
	for _, p := range []string{"etc/sonder", "tmp", "var/lib/sonder"} {
		if err := os.MkdirAll(filepath.Join(h.instanceRoot, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	stubDir := filepath.Join(dir, "stubs")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := `#!/usr/bin/env bash
set -euo pipefail
op="$1"; shift
if [[ "$op" == info ]]; then exit 0; fi
if [[ "$op" == restart ]]; then [[ -z "${FAKE_RESTART_FAILS:-}" ]]; exit; fi
if [[ "$op" == file ]]; then
  sub="$1"; shift
  while [[ "$1" == --* ]]; do flag="$1"; shift; [[ "$flag" == --create-dirs ]] || shift; done
  if [[ "$sub" == pull ]]; then src="$1"; dest="$2"; src="${src#sonder}"; cp "$FAKE_INSTANCE_ROOT$src" "$dest"; exit; fi
  src="$1"; dest="$2"; dest="${dest#sonder}"; mkdir -p "$(dirname "$FAKE_INSTANCE_ROOT$dest")"; cp "$src" "$FAKE_INSTANCE_ROOT$dest"; exit
fi
[[ "$op" == exec ]] || exit 90
instance="$1"; shift; [[ "$1" == -- ]] && shift
cmd="$1"; shift
case "$cmd" in
  test) [[ -x "$FAKE_INSTANCE_ROOT$2" ]] ;;
  getent) echo 'ubuntu:x:1000:1000:ubuntu:/home/ubuntu:/bin/bash' ;;
  runuser) exit 0 ;;
  install) while [[ "$1" == -* ]]; do case "$1" in -o|-g) shift 2;; -m) shift 2;; *) shift;; esac; done; cp "$FAKE_INSTANCE_ROOT$1" "$FAKE_INSTANCE_ROOT$2"; chmod 755 "$FAKE_INSTANCE_ROOT$2" ;;
  mv) shift; mv -f "$FAKE_INSTANCE_ROOT$1" "$FAKE_INSTANCE_ROOT$2" ;;
  rm) shift; rm -f "$FAKE_INSTANCE_ROOT$1" ;;
  systemctl) if [[ "$1" == is-active ]]; then echo active; fi ;;
  /usr/local/bin/sonder) printf '{"version":"%s","status":{"webHealthy":%s,"apiHealthy":%s}}' "$FAKE_VERSION" "${FAKE_WEB_HEALTHY:-true}" "${FAKE_API_HEALTHY:-true}" ;;
  *) echo "unexpected incus exec command: $cmd $*" >&2; exit 91 ;;
esac
`
	if err := os.WriteFile(filepath.Join(stubDir, "incus"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return h
}

func (h *deployHarness) run(t *testing.T, wantSuccess bool, version string) string {
	t.Helper()
	script, err := filepath.Abs("../deploy/deploy-sonder.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, version)
	cmd.Env = append(os.Environ(), "SONDER_REPO="+h.repo, "FAKE_INSTANCE_ROOT="+h.instanceRoot, "FAKE_VERSION="+h.version, "SONDER_HEALTH_ATTEMPTS=2", "SONDER_HEALTH_INTERVAL=0")
	out, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("expected success: %v\n%s", err, out)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("expected deployment failure\n%s", out)
	}
	return string(out)
}

func TestDeployRejectsArmArtifactWithoutChangingContainer(t *testing.T) {
	h := newDeployHarness(t)
	writeELF(t, filepath.Join(h.repo, "bin", ".sonder-linux-amd64.incoming"), emAarch64)
	out := h.run(t, false, "abc123def456")
	if !strings.Contains(out, "aarch64") {
		t.Fatalf("expected architecture error, got %s", out)
	}
	if _, err := os.Stat(filepath.Join(h.instanceRoot, "usr/local/bin/sonder")); err != nil {
		t.Fatal("existing binary was changed")
	}
}

func TestDeployReplacesIncusBinaryAndConfirmsHealth(t *testing.T) {
	h := newDeployHarness(t)
	out := h.run(t, true, "abc123def456")
	if !strings.Contains(out, "serving abc123def456") {
		t.Fatalf("missing healthy version confirmation: %s", out)
	}
	if _, err := os.Stat(filepath.Join(h.repo, "bin", ".sonder-linux-amd64.incoming")); !os.IsNotExist(err) {
		t.Fatalf("staged file not consumed: %v", err)
	}
	backups, _ := filepath.Glob(filepath.Join(h.repo, "bin", "sonder-linux-amd64.incus-bak-*"))
	if len(backups) != 1 {
		t.Fatalf("expected retained host backup, got %v", backups)
	}
}

func TestDeployRollsBackWhenVersionDoesNotMatch(t *testing.T) {
	h := newDeployHarness(t)
	before, _ := os.ReadFile(filepath.Join(h.instanceRoot, "usr/local/bin/sonder"))
	h.version = "oldversion"
	out := h.run(t, false, "newversion")
	if !strings.Contains(out, "rolled back") {
		t.Fatalf("expected rollback, got %s", out)
	}
	after, _ := os.ReadFile(filepath.Join(h.instanceRoot, "usr/local/bin/sonder"))
	if string(before) != string(after) {
		t.Fatal("prior binary was not restored")
	}
}

func TestDeployRollsBackWhenAppHealthIsFalse(t *testing.T) {
	h := newDeployHarness(t)
	t.Setenv("FAKE_API_HEALTHY", "false")
	out := h.run(t, false, "abc123def456")
	if !strings.Contains(out, "health check failed") {
		t.Fatalf("expected health-check rollback, got %s", out)
	}
}
