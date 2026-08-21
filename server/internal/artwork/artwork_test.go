package artwork

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPickTimestamp(t *testing.T) {
	cases := []struct {
		dur, want float64
	}{
		{0, 3},
		{-5, 3},
		{10, 2},     // 10% = 1s -> clamped to 2s floor
		{60, 6},     // 10%
		{3600, 180}, // clamped ceiling
	}
	for _, c := range cases {
		if got := pickTimestamp(c.dur); got != c.want {
			t.Errorf("pickTimestamp(%v) = %v, want %v", c.dur, got, c.want)
		}
	}
}

func TestArgsShape(t *testing.T) {
	args := Args("ffmpeg", "/m/Film.mp4", "/out/x.jpg", 12.5)
	want := []string{"-hide_banner", "-loglevel", "error", "-ss", "12.50", "-i", "/m/Film.mp4", "-frames:v", "1"}
	for i, w := range want {
		if args[i] != w {
			t.Errorf("args[%d] = %q, want %q (full: %v)", i, args[i], w, args)
		}
	}
	if last := args[len(args)-1]; last != "/out/x.jpg" {
		t.Errorf("output not last: %v", args)
	}
}

func TestGenerateLiveFfmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=4:size=320x180:rate=10",
		"-c:v", "libx264", "-preset", "ultrafast", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("fixture render failed: %v: %s", err, out)
	}

	g := &Generator{FFmpegPath: ffmpeg, OutDir: filepath.Join(dir, "art")}
	got, err := g.Generate(context.Background(), src, "item-1", 4)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(got)
	if err != nil || st.Size() == 0 {
		t.Fatalf("poster missing/empty: %v %v", got, err)
	}

	// Second generate returns the cached file without re-running ffmpeg.
	before := st.ModTime()
	got2, err := g.Generate(context.Background(), src, "item-1", 4)
	if err != nil || got2 != got {
		t.Fatalf("second generate: %v %v", got2, err)
	}
	if st2, _ := os.Stat(got); !st2.ModTime().Equal(before) {
		t.Error("cached poster was regenerated")
	}

	// Audio-only input must fail cleanly.
	audio := filepath.Join(dir, "tone.m4a")
	cmd = exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "aac", audio)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("audio fixture failed: %v: %s", err, out)
	}
	if _, err := g.Generate(context.Background(), audio, "item-2", 1); err == nil {
		t.Error("expected error for audio-only input")
	}
}
