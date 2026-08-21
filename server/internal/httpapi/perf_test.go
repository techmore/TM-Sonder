package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

// BenchmarkLibrary10k targets the roadmap perf goal: <50ms warm, gzipped.
func BenchmarkLibrary10k(b *testing.B) {
	benchmarkLibraryN(b, 10000)
}

func BenchmarkLibrary1k(b *testing.B) {
	benchmarkLibraryN(b, 1000)
}

func benchmarkLibraryN(b *testing.B, n int) {
	cfg := config.Default()
	cfg.DataDir = b.TempDir()
	store := library.New()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("item-%06d", i)
		store.Upsert(&library.Item{
			MediaItem: api.MediaItem{
				ID:    id,
				Title: "Title " + id,
				Kind:  api.KindMovie, Format: api.FormatMP4,
				Tags:                []string{"bench"},
				EmbeddedAudioTracks: []api.PlaybackTrack{{ID: "embedded-audio:0", Label: "En", Kind: api.TrackEmbedded}},
			},
			FilePath: "/media/" + id + ".mp4",
		})
	}
	srv := New(&cfg, store, library.NewScanner(store), nil)
	srv.logger.SetOutput(io.Discard)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/api/library", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		buf := make([]byte, 64<<10)
		for {
			_, err := resp.Body.Read(buf)
			if err != nil {
				break
			}
		}
		resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 304 {
			b.Fatalf("status %d", resp.StatusCode)
		}
	}
}

// TestGzipAndPprofGuard verifies the middleware behaviors added for perf.
func TestGzipAndPprofGuard(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "g1", "Gzip Film")

	// Gzipped library response decompresses to valid JSON with items key.
	req, _ := http.NewRequest("GET", f.ts.URL+"/api/library", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("content-encoding = %q", resp.Header.Get("Content-Encoding"))
	}
	body := readAll(t, resp)
	if !strings.Contains(body, "\"items\"") || !strings.Contains(body, "Gzip Film") {
		t.Errorf("gzipped body wrong: %.80s", body)
	}

	// Streams bypass gzip.
	req2, _ := http.NewRequest("GET", f.ts.URL+"/stream/g1", nil)
	req2.Header.Set("Accept-Encoding", "gzip")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, resp2)
	if resp2.Header.Get("Content-Encoding") == "gzip" {
		t.Error("stream should not be gzipped")
	}

	// pprof is loopback-only: forged loopback peer reaches the profile index.
	req3 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req3.RemoteAddr = "127.0.0.1:5555"
	rec3 := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec3, req3)
	if rec3.Code != 200 {
		t.Errorf("loopback pprof status = %d", rec3.Code)
	}
	// ...but a remote peer is rejected before auth even runs.
	req4 := httptest.NewRequest("GET", "/debug/pprof/", nil)
	req4.RemoteAddr = "10.9.9.9:1234"
	rec4 := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusNotFound {
		t.Errorf("remote pprof status = %d, want 404", rec4.Code)
	}
}
