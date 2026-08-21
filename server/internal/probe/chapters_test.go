package probe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

const chaptersJSON = `{
  "chapters": [
    {"id":0,"start":0,"start_time":"0.000000","end":120.5,"end_time":"120.500000","tags":{"title":"Arrival"}},
    {"id":1,"start":120.5,"start_time":"120.500000","end":300,"end_time":"300.000000"},
    {"id":2,"start":300,"start_time":"300.000000","end":0,"end_time":"0.000000","tags":{"title":"Finale"}}
  ]
}`

func TestParseChapters(t *testing.T) {
	got := ParseChapters([]byte(chaptersJSON))
	if len(got) != 3 {
		t.Fatalf("chapters = %d", len(got))
	}
	c0 := got[0]
	if c0.Index != 0 || c0.Title != "Arrival" || c0.StartSeconds != 0 ||
		c0.EndSeconds == nil || *c0.EndSeconds != 120.5 {
		t.Errorf("c0 = %+v", c0)
	}
	c1 := got[1]
	if c1.Title != "Chapter 2" || c1.StartSeconds != 120.5 || *c1.EndSeconds != 300 {
		t.Errorf("c1 fallback title/start wrong: %+v", c1)
	}
	c2 := got[2]
	if c2.EndSeconds != nil {
		t.Errorf("zero end should be nil (open-ended): %+v", c2)
	}
}

func TestParseChaptersEmpty(t *testing.T) {
	if got := ParseChapters([]byte(`{"chapters":[]}`)); len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
	if got := ParseChapters([]byte(`not json`)); got != nil {
		t.Errorf("bad json should yield nil, got %+v", got)
	}
}

func TestChaptersLiveFfmpegFixture(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	m4b := filepath.Join(dir, "book.m4b")
	meta := filepath.Join(dir, "chapters.txt")
	content := ";FFMETADATA1\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=Part One\n" +
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=2000\ntitle=Part Two\n"
	if err := writeFile(meta, content); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=330:duration=2",
		"-i", meta, "-map", "0:a", "-map_metadata", "1",
		"-c:a", "aac", "-f", "ipod", m4b)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("fixture render failed: %v: %s", err, out)
	}

	chapters, err := Chapters(context.Background(), ffprobe, m4b)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 2 || chapters[0].Title != "Part One" ||
		chapters[1].Title != "Part Two" || chapters[1].StartSeconds != 1.0 {
		t.Errorf("live chapters wrong: %+v", chapters)
	}
}
