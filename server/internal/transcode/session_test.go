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

	r, cleanup, err := m.Attach(ctx, item, ModeEncode, 0, -1, -1)
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
	r, cleanup, err := m.Attach(ctx, item, ModeEncode, 0, -1, -1)
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
	_, c1, err := m.Attach(ctx, item, ModeEncode, 0, -1, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer c1()
	first := m.sessionCount()
	if first != 1 {
		t.Fatalf("sessions = %d", first)
	}

	// Far seek -> old session replaced.
	r2, c2, err := m.Attach(ctx, item, ModeEncode, 5.0, -1, -1)
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

	// Sample live sessions (one session == one ffmpeg process). Readers on the
	// same warm session are expected and must not be mistaken for extra
	// processes, so count sessions rather than readers.
	stop := make(chan struct{})
	samplerDone := make(chan struct{})
	var peak atomic.Int32
	go func() {
		defer close(samplerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n := int32(m.sessionCount())
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	done := make(chan struct{}, 3)
	run := func(it *library.Item) {
		defer func() { done <- struct{}{} }()
		r, cleanup, err := m.Attach(context.Background(), it, ModeEncode, 0, -1, -1)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup()
		buf := make([]byte, 4096)
		for {
			if _, err := r.Read(buf); err != nil {
				return
			}
		}
	}
	for _, it := range []*library.Item{a, b, a} {
		go run(it)
	}
	<-done
	<-done
	<-done
	m.StopAll()
	close(stop)
	<-samplerDone

	if p := peak.Load(); p > 1 {
		t.Errorf("peak concurrent ffmpeg sessions = %d, want <= MaxConcurrent(1)", p)
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

// newTestSession builds a Session that needs no ffmpeg process: stop() only
// requires a working cancel/done pair.
func newTestSession() (*Session, chan struct{}) {
	stopped := make(chan struct{})
	s := &Session{
		readers: make(map[*reader]struct{}),
		done:    make(chan struct{}),
	}
	s.cancel = func() {
		close(stopped)
		close(s.done)
	}
	return s, stopped
}

// TestIdleSessionReapedAfterLastReaderDetaches covers the orphan-ffmpeg fix:
// once the last reader detaches the session must stop after SessionIdleTTL.
func TestIdleSessionReapedAfterLastReaderDetaches(t *testing.T) {
	old := SessionIdleTTL
	SessionIdleTTL = 50 * time.Millisecond
	defer func() { SessionIdleTTL = old }()

	s, stopped := newTestSession()
	r := s.attach()
	s.closeReader(r)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("idle session was not stopped")
	}
}

// TestReattachCancelsIdleReap verifies warm-session reuse: attaching a new
// reader before the TTL expires cancels the pending reap.
func TestReattachCancelsIdleReap(t *testing.T) {
	old := SessionIdleTTL
	SessionIdleTTL = 150 * time.Millisecond
	defer func() { SessionIdleTTL = old }()

	s, stopped := newTestSession()
	r1 := s.attach()
	s.closeReader(r1)

	// Reattach within the grace window.
	time.Sleep(20 * time.Millisecond)
	r2 := s.attach()

	select {
	case <-stopped:
		t.Fatal("session was reaped despite a live reader")
	case <-time.After(300 * time.Millisecond):
	}

	// Detaching the last reader arms the reap again.
	s.closeReader(r2)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("idle session was not stopped after final detach")
	}
}

func last(args []string) string { return args[len(args)-1] }

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestBuildArgsSelectsAudioTrack(t *testing.T) {
	cfg := Config{FFmpegPath: "ffmpeg", HWAccel: "none", Preset: "veryfast"}

	first := buildArgs(Request{Path: "/m.mkv", Mode: ModeEncode, AudioTrackN: 0}, cfg)
	if !containsArg(first, "0:a:0?") {
		t.Errorf("default audio map wrong: %v", first)
	}

	third := buildArgs(Request{Path: "/m.mkv", Mode: ModeEncode, AudioTrackN: 2}, cfg)
	if !containsArg(third, "0:a:2?") {
		t.Errorf("audio map did not honor track 2: %v", third)
	}
	if containsArg(third, "0:a:0?") {
		t.Errorf("stale default audio map present: %v", third)
	}

	// Sessions with different audio selections must not share a key.
	a := Request{Path: "/m.mkv", Mode: ModeRemux, AudioTrackN: 0}
	b := Request{Path: "/m.mkv", Mode: ModeRemux, AudioTrackN: 1}
	if a.key() == b.key() {
		t.Error("session key ignores audio track selection")
	}
}

func hasPair(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}

func TestAudioCopyableDecisions(t *testing.T) {
	cases := []struct {
		codecs []string
		track  int
		want   bool
	}{
		{nil, 0, false},                    // unknown: stay safe
		{[]string{}, 0, false},             // no codec info: stay safe
		{[]string{"aac"}, 0, true},         // MP4-safe
		{[]string{"ac3"}, 0, true},         // MP4-safe
		{[]string{"dts"}, 0, false},        // must re-encode
		{[]string{"truehd"}, 0, false},     // must re-encode
		{[]string{"aac", "dts"}, 0, true},  // selected track is safe
		{[]string{"aac", "dts"}, 1, false}, // selected track is not
		{[]string{"aac", "dts"}, 9, false}, // out of range: all must be safe
		{[]string{"aac", "ac3"}, 9, true},  // out of range but all safe
	}
	for _, tc := range cases {
		item := &library.Item{}
		item.ProbedAudioCodecs = tc.codecs
		if got := audioCopyable(item, tc.track); got != tc.want {
			t.Errorf("audioCopyable(%v, %d) = %v, want %v", tc.codecs, tc.track, got, tc.want)
		}
	}
}

func TestRemuxReencodesUnsafeAudio(t *testing.T) {
	cfg := Config{HWAccel: "none", Preset: "veryfast"}

	safe := buildArgs(Request{Path: "/m.mkv", Mode: ModeRemux, CopyAudio: true}, cfg)
	if !hasPair(safe, "-c:v", "copy") || !hasPair(safe, "-c:a", "copy") {
		t.Errorf("safe remux should copy both streams: %v", safe)
	}

	unsafe := buildArgs(Request{Path: "/m.mkv", Mode: ModeRemux, CopyAudio: false}, cfg)
	if !hasPair(unsafe, "-c:v", "copy") {
		t.Errorf("video must still be remuxed: %v", unsafe)
	}
	if !hasPair(unsafe, "-c:a", "aac") {
		t.Errorf("unsafe audio should be re-encoded to aac: %v", unsafe)
	}

	// Session key must separate copy vs re-encode decisions.
	a := Request{Path: "/m.mkv", Mode: ModeRemux, CopyAudio: true}
	b := Request{Path: "/m.mkv", Mode: ModeRemux, CopyAudio: false}
	if a.key() == b.key() {
		t.Error("session key ignores audio copy decision")
	}
}
