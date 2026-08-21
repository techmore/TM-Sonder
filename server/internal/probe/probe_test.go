package probe

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const twoAudioOneSub = `{
  "streams": [
    {"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"bit_rate":"5000000"},
    {"index":1,"codec_type":"audio","codec_name":"aac","tags":{"language":"eng","title":"Commentary"}},
    {"index":2,"codec_type":"audio","codec_name":"ac3","tags":{"language":"fre"}},
    {"index":3,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"und"}},
    {"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"de-DE"}}
  ],
  "format": {"duration":"5401.250000","bit_rate":"8000000"}
}`

func TestParseTwoAudioOneSubtitle(t *testing.T) {
	res, err := Parse([]byte(twoAudioOneSub))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.AudioTracks) != 2 || len(res.SubtitleTracks) != 2 {
		t.Fatalf("track counts: %d audio, %d sub", len(res.AudioTracks), len(res.SubtitleTracks))
	}
	a := res.AudioTracks
	if a[0].ID != "embedded-audio:0" || a[0].Label != "Commentary" || a[0].LanguageCode == nil || *a[0].LanguageCode != "en" {
		t.Errorf("audio[0] = %+v", a[0])
	}
	if a[1].ID != "embedded-audio:1" || a[1].Label != "French" || a[1].LanguageCode == nil || *a[1].LanguageCode != "fr" {
		t.Errorf("audio[1] = %+v", a[1])
	}
	s := res.SubtitleTracks
	// und -> nil language, display falls back to "Track N".
	if s[0].ID != "embedded-subtitle:0" || s[0].LanguageCode != nil || s[0].Label != "Track 1" {
		t.Errorf("subtitle[0] = %+v", s[0])
	}
	if s[1].ID != "embedded-subtitle:1" || s[1].LanguageCode == nil || *s[1].LanguageCode != "de" {
		t.Errorf("subtitle[1] = %+v", s[1])
	}
	if res.Width == nil || *res.Width != 1920 || res.Height == nil || *res.Height != 1080 {
		t.Errorf("dimensions = %v x %v", res.Width, res.Height)
	}
	if res.Codec == nil || *res.Codec != "h264" || res.Bitrate == nil || *res.Bitrate != 5000000 {
		t.Errorf("codec/bitrate = %v/%v", res.Codec, res.Bitrate)
	}
	if res.DurationSeconds != 5401.25 {
		t.Errorf("duration = %v", res.DurationSeconds)
	}
}

func TestParseEmpty(t *testing.T) {
	res, err := Parse([]byte(`{"streams":[],"format":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.AudioTracks) != 0 || len(res.SubtitleTracks) != 0 || res.Width != nil {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestCacheInvalidatedByModTime(t *testing.T) {
	c := NewCache("")
	mod := time.Unix(1700000000, 0)
	first := &Result{DurationSeconds: 10}
	c.mu.Lock()
	c.entries["/m.mkv"] = cacheEntry{size: 5, mod: mod, result: first}
	c.mu.Unlock()

	got, err := c.ProbeResult(context.Background(), "/m.mkv", 5, mod)
	if err != nil || got != first {
		t.Fatalf("cache hit failed: %v %v", got, err)
	}
	if _, err := c.ProbeResult(context.Background(), "/m.mkv", 5, mod.Add(time.Second)); err == nil {
		t.Error("stale entry served after mtime change")
	}
}

func TestProbeLiveFfmpegFixture(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}

	dir := t.TempDir()
	mkv := filepath.Join(dir, "sample.mkv")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=128x72:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=1",
		"-map", "0:v", "-map", "1:a", "-map", "2:a",
		"-c:v", "libx264", "-preset", "ultrafast",
		mkv)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not generate fixture: %v: %s", err, out)
	}

	res, err := Probe(context.Background(), ffprobe, mkv)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.AudioTracks) != 2 ||
		res.AudioTracks[0].ID != "embedded-audio:0" ||
		res.AudioTracks[1].ID != "embedded-audio:1" {
		t.Errorf("audio tracks wrong: %+v", res.AudioTracks)
	}
}
