package transcode

import (
	"os"
	"testing"

	"tm-sonder/server/internal/library"
)

func strp(s string) *string { return &s }

func TestPickModeRemuxesOnlyH264(t *testing.T) {
	m := &Manager{}
	cases := []struct {
		codec *string
		want  Mode
	}{
		{strp("h264"), ModeRemux}, {strp("avc1"), ModeRemux},
		{strp("hevc"), ModeEncode}, {strp("mpeg4"), ModeEncode}, {nil, ModeEncode},
	}
	for _, c := range cases {
		if got := m.pickMode(itemWithCodec(c.codec)); got != c.want {
			t.Errorf("pickMode(%v) = %v want %v", c.codec, got, c.want)
		}
	}
}

func TestAudioCopyable(t *testing.T) {
	item := func(codecs ...string) *library.Item { return itemWithAudio(codecs) }
	cases := []struct {
		name  string
		item  *library.Item
		track int
		want  bool
	}{
		{"no probe data", item(), 0, false},
		{"aac selected", item("aac", "dts"), 0, true},
		{"dts selected", item("aac", "dts"), 1, false},
		{"case-insensitive", item("AAC"), 0, true},
		{"unknown track needs all safe", item("aac", "ac3"), -1, true},
		{"unknown track with dts", item("aac", "dts"), -1, false},
		{"out of range falls back to all", item("truehd"), 5, false},
	}
	for _, c := range cases {
		if got := audioCopyable(c.item, c.track); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestVODKeyIsStableAndVersioned(t *testing.T) {
	a, b := VODKey("id", "v1"), VODKey("id", "v1")
	if a != b || len(a) != 64 {
		t.Fatalf("unstable key %q %q", a, b)
	}
	if a == VODKey("id", "v2") || a == VODKey("id2", "v1") {
		t.Fatal("key ignores id or version")
	}
}

func TestVODLimitEnvOverride(t *testing.T) {
	t.Setenv("SONDER_VOD_CACHE_MAX_BYTES", "")
	def := VODLimit()
	t.Setenv("SONDER_VOD_CACHE_MAX_BYTES", "5368709120")
	if got := VODLimit(); got != 5368709120 {
		t.Errorf("override ignored: %d", got)
	}
	t.Setenv("SONDER_VOD_CACHE_MAX_BYTES", "1024") // below the 1 GiB floor
	if got := VODLimit(); got != def {
		t.Errorf("sub-floor value accepted: %d", got)
	}
	t.Setenv("SONDER_VOD_CACHE_MAX_BYTES", "garbage")
	if got := VODLimit(); got != def {
		t.Errorf("garbage accepted: %d", got)
	}
}

func TestAbsDiffAndDirectoryBytes(t *testing.T) {
	if absDiff(1, 4) != 3 || absDiff(4, 1) != 3 || absDiff(2, 2) != 0 {
		t.Fatal("absDiff wrong")
	}
	dir := t.TempDir()
	writeFile(t, dir+"/a", 10)
	writeFile(t, dir+"/b", 5)
	if got := directoryBytes(dir); got != 15 {
		t.Fatalf("directoryBytes = %d", got)
	}
	if directoryBytes(dir+"/missing") != 0 {
		t.Fatal("missing dir should count zero")
	}
}

func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatal(err)
	}
}

func itemWithCodec(c *string) *library.Item {
	it := &library.Item{}
	it.ProbedCodec = c
	return it
}

func itemWithAudio(c []string) *library.Item {
	it := &library.Item{}
	it.ProbedAudioCodecs = c
	return it
}
