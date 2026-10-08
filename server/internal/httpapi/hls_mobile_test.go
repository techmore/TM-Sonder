package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/transcode"
)

const phoneSafariUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1"

func TestNativeHLSDelivery(t *testing.T) {
	for _, tt := range []struct {
		name, ua, query string
		want            bool
	}{
		{"legacy iPhone", phoneSafariUA, "transcode=1", true},
		{"desktop Safari", "Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15", "transcode=1", true},
		{"desktop Chrome", "Mozilla/5.0 (Macintosh) Chrome/140.0 Safari/537.36", "transcode=1", false},
		{"explicit MP4", phoneSafariUA, "transcode=1&delivery=mp4", false},
		{"explicit HLS", "Android Chrome/140.0", "transcode=1&delivery=hls", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/stream/movie?"+tt.query, nil)
			r.Header.Set("User-Agent", tt.ua)
			if got := useNativeHLS(r); got != tt.want {
				t.Fatalf("HLS delivery = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLegacyIPhonePlaybackProbesReuseHLSAndSeekRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake encoder uses a shell")
	}
	encoder := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
for output do :; done
dir=${output%/*}
printf '#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nsegment000000.ts\n#EXTINF:2,\nsegment000001.ts\n#EXTINF:2,\nsegment000002.ts\n' > "$output"
printf 'test video segment' > "$dir/segment000000.ts"
exec sleep 60
`
	if err := os.WriteFile(encoder, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, func(c *config.Config) {
		c.FFmpegPath = encoder
		c.Transcode.MaxConcurrent = 1
		c.Transcode.HWAccel = "none"
	})
	f.s.tm = transcode.NewManager(transcode.Config{FFmpegPath: encoder, HWAccel: "none", MaxConcurrent: 1})
	t.Cleanup(f.s.tm.StopAll)
	f.addItem(t, "phone", "Phone movie")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(path string) *http.Response {
		t.Helper()
		r, _ := http.NewRequest("GET", f.ts.URL+path, nil)
		r.Header.Set("User-Agent", phoneSafariUA)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	first := request("/stream/phone?transcode=1&ss=285.180")
	location := first.Header.Get("Location")
	if first.StatusCode != http.StatusTemporaryRedirect || !strings.HasPrefix(location, "/stream/phone/hls/") {
		t.Fatalf("legacy phone request returned %d %q", first.StatusCode, location)
	}
	second := request("/stream/phone?transcode=1&ss=285.180")
	if second.StatusCode != http.StatusTemporaryRedirect || second.Header.Get("Location") != location {
		t.Fatalf("Safari probe did not reuse its encoder: %d %q", second.StatusCode, second.Header.Get("Location"))
	}
	playlist := request(location)
	data, _ := io.ReadAll(playlist.Body)
	if playlist.StatusCode != http.StatusOK || !strings.Contains(string(data), "segment000000.ts") {
		t.Fatalf("playlist unavailable: %d %s", playlist.StatusCode, data)
	}
	seek := request("/stream/phone?transcode=1&ss=600")
	if seek.StatusCode != http.StatusTemporaryRedirect || seek.Header.Get("Location") == location {
		t.Fatalf("seek did not replace the encoder: %d %q", seek.StatusCode, seek.Header.Get("Location"))
	}
}

func TestLegacyHLSIdentityIsolatesAccountsAndTracks(t *testing.T) {
	r := httptest.NewRequest("GET", "/stream/movie?token=first-secret", nil)
	r.Header.Set("User-Agent", phoneSafariUA)
	owner, generation := hlsPlaybackIdentity(r, "movie", 20, -1, 0)
	r2 := httptest.NewRequest("GET", "/stream/movie?token=second-secret", nil)
	r2.Header.Set("User-Agent", phoneSafariUA)
	other, _ := hlsPlaybackIdentity(r2, "movie", 20, -1, 0)
	if owner == other || strings.Contains(owner, "first-secret") {
		t.Fatal("accounts share an encoder identity or retain credentials")
	}
	_, changed := hlsPlaybackIdentity(r, "movie", 20, -1, 1)
	if changed == generation {
		t.Fatal("audio track change reused the old encoder")
	}
}
