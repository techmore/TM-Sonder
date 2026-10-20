package subtitles

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLanguageMatches(t *testing.T) {
	cases := []struct {
		v, lang string
		want    bool
	}{
		{"en", "en", true}, {"EN-US", "en", true}, {"eng", "en", true},
		{"es", "en", false}, {"eng", "es", false}, {"", "en", false},
	}
	for _, c := range cases {
		if got := languageMatches(c.v, c.lang); got != c.want {
			t.Errorf("languageMatches(%q,%q)=%v want %v", c.v, c.lang, got, c.want)
		}
	}
}

func TestValidSubtitle(t *testing.T) {
	good := []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello there\n")
	if !validSubtitle(good) {
		t.Error("rejected a valid cue file")
	}
	for name, b := range map[string][]byte{
		"empty":     nil,
		"too short": []byte("-->"),
		"no cues":   []byte(strings.Repeat("plain text without timing ", 4)),
		"html":      []byte("<HTML><body>blocked --> by captive portal</body>"),
		"oversize":  []byte("-->" + strings.Repeat("x", maxSubtitleBytes)),
	} {
		if validSubtitle(b) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestInstallNeverOverwritesExistingSidecar(t *testing.T) {
	dir := t.TempDir()
	tmp, target := filepath.Join(dir, "tmp"), filepath.Join(dir, "movie.en.vtt")
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := install(tmp, target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("newer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := install(tmp, target); err == nil {
		t.Fatal("second install replaced an existing sidecar")
	}
}

func TestMovieHash(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small")
	if err := os.WriteFile(small, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := movieHash(small); err == nil {
		t.Error("expected error for media under 128 KiB")
	}
	// All-zero content: the hash is just the file size.
	big := filepath.Join(dir, "big")
	data := make([]byte, 200000)
	if err := os.WriteFile(big, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := mustHash(t, big), fmt.Sprintf("%016x", 200000); got != want {
		t.Errorf("zero hash = %s want %s", got, want)
	}
	// A word inside the first 64 KiB must change the hash by exactly that word.
	binary.LittleEndian.PutUint64(data[8:], 7)
	if err := os.WriteFile(big, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := mustHash(t, big), fmt.Sprintf("%016x", 200000+7); got != want {
		t.Errorf("hash = %s want %s", got, want)
	}
}

func mustHash(t *testing.T, p string) string {
	t.Helper()
	h, err := movieHash(p)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNormalized(t *testing.T) {
	if got := normalized("The Matrix (1999)!"); got != "thematrix1999" {
		t.Errorf("normalized = %q", got)
	}
}
