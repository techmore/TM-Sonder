package transcode

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tm-sonder/server/internal/library"
)

func ffmpegPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	return p
}

// makeFixture renders a 6s 320x180 h264/aac mp4 for transcoding.
func makeFixture(t *testing.T) *library.Item {
	t.Helper()
	ff := ffmpegPath(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ff, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=6:size=320x180:rate=15",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=6",
		"-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest",
		path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("fixture render failed: %v: %s", err, out)
	}
	return &library.Item{FilePath: path}
}

func TestAttachEncodeProducesFMP4(t *testing.T) {
	item := makeFixture(t)
	m := NewManager(Config{FFmpegPath: ffmpegPath(t), HWAccel: "none", MaxConcurrent: 1})
	defer m.StopAll()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, cleanup, err := m.Attach(ctx, item, ModeEncode, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	head := make([]byte, 64)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatal(err)
	}
	// fMP4 starts with a size + 'ftyp' box.
	if n < 12 || string(head[4:8]) != "ftyp" {
		t.Fatalf("output does not start with ftyp box: %q", head[:n])
	}

	total := int64(n)
	buf := make([]byte, 32<<10)
	for {
		k, err := r.Read(buf)
		total += int64(k)
		if err != nil {
			break
		}
		if total > 200<<10 {
			break
		}
	}
	if total < 50<<10 {
		t.Errorf("suspiciously small transcode output: %d bytes", total)
	}
}

func TestClientDisconnectKillsFfmpegFast(t *testing.T) {
	item := makeFixture(t)
	m := NewManager(Config{FFmpegPath: ffmpegPath(t), HWAccel: "none", MaxConcurrent: 1})
	defer m.StopAll()

	ctx := context.Background()
	r, cleanup, err := m.Attach(ctx, item, ModeEncode, 0, -1)
	if err != nil {
		t.Fatal(err)
	}

	// Consume a little so ffmpeg is mid-stream.
	buf := make([]byte, 16<<10)
	io.ReadFull(r, buf[:1024])

	start := time.Now()
	cleanup() // client disconnect path
	r.Close() // defensive double-close must not panic

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.sessionCount() == 0 {
			elapsed := time.Since(start)
			if elapsed > time.Second {
				t.Errorf("ffmpeg reaped in %v, want <= 1s", elapsed)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("session still registered 2s after disconnect")
}

func TestSeekOutsideWindowRespawns(t *testing.T) {
	item := makeFixture(t)
	m := NewManager(Config{FFmpegPath: ffmpegPath(t), HWAccel: "none", MaxConcurrent: 1})
	defer m.StopAll()

	ctx := context.Background()
	_, c1, err := m.Attach(ctx, item, ModeEncode, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer c1()
	first := m.sessionCount()
	if first != 1 {
		t.Fatalf("sessions = %d", first)
	}

	// Far seek -> old session replaced.
	r2, c2, err := m.Attach(ctx, item, ModeEncode, 5.0, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer c2()
	if got := m.sessionCount(); got != 1 {
		t.Fatalf("after respawn sessions = %d, want 1 (old must be gone)", got)
	}
	head := make([]byte, 12)
	if _, err := io.ReadFull(r2, head); err != nil {
		t.Fatal(err)
	}
}

func TestSemaphoreBoundsConcurrency(t *testing.T) {
	a := makeFixture(t)
	b := makeFixture(t)
	m := NewManager(Config{FFmpegPath: ffmpegPath(t), HWAccel: "none", MaxConcurrent: 1})

	var running atomic.Int32
	var peak atomic.Int32
	done := make(chan struct{}, 3)

	run := func(it *library.Item) {
		r, cleanup, err := m.Attach(context.Background(), it, ModeEncode, 0, -1)
		if err != nil {
			t.Error(err)
			done <- struct{}{}
			return
		}
		cur := running.Add(1)
		for {
			if old := peak.Load(); cur > old && !peak.CompareAndSwap(old, cur) {
				continue
			}
			break
		}
		buf := make([]byte, 4096)
		for {
			if _, err := r.Read(buf); err != nil {
				break
			}
		}
		running.Add(-1)
		cleanup()
		done <- struct{}{}
	}
	for _, it := range []*library.Item{a, b, a} {
		go run(it)
	}
	<-done
	<-done
	<-done
	m.StopAll()
	if p := peak.Load(); p > 1 {
		t.Errorf("peak concurrent ffmpeg = %d, want <= MaxConcurrent(1)", p)
	}
}

func TestBuildArgsShapes(t *testing.T) {
	cfg := Config{Preset: "veryfast", HWAccel: "none"}
	got := buildArgs(Request{Path: "/m/Film.mp4", Mode: ModeRemux, StartSeconds: 3.5}, cfg)
	want := []string{"-ss", "3.500", "-i", "/m/Film.mp4", "-c:v", "copy", "-c:a", "copy"}
	for _, w := range want {
		if !containsArg(got, w) {
			t.Errorf("remux args missing %q: %v", w, got)
		}
	}
	if !containsArg(got, "frag_keyframe+empty_moov") || last(got) != "pipe:1" {
		t.Errorf("fMP4 stdout flags wrong: %v", got)
	}

	enc := buildArgs(Request{Path: "/m.mkv", Mode: ModeEncode, BurnSubtitleN: 1}, cfg)
	if !containsArg(enc, "libx264") || !containsArg(enc, "veryfast") || !containsArg(enc, "20") {
		t.Errorf("encode args wrong: %v", enc)
	}
	if !containsArg(enc, "subtitles='/m.mkv':si=1") {
		t.Errorf("burn-in vf missing: %v", enc)
	}

	vt := buildArgs(Request{Path: "/m.mkv", Mode: ModeEncode}, Config{HWAccel: "videotoolbox"})
	if !containsArg(vt, "h264_videotoolbox") {
		t.Errorf("videotoolbox args wrong: %v", vt)
	}
}

func containsArg(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func last(args []string) string { return args[len(args)-1] }

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
