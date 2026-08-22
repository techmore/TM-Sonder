package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

// newPageBenchServer builds a store with n items of mixed kinds and a live
// test server, so every page is measured under identical conditions.
func newPageBenchServer(b *testing.B, n int) *httptest.Server {
	b.Helper()
	cfg := config.Default()
	cfg.DataDir = b.TempDir()
	store := library.New()
	kinds := []api.MediaKind{api.KindMovie, api.KindTVShow, api.KindDocumentary}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("item-%06d", i)
		store.Upsert(&library.Item{
			MediaItem: api.MediaItem{
				ID:                  id,
				Title:               "Title " + id,
				Kind:                kinds[i%len(kinds)],
				Format:              api.FormatMP4,
				Tags:                []string{"bench"},
				EmbeddedAudioTracks: []api.PlaybackTrack{{ID: "embedded-audio:0", Label: "En", Kind: api.TrackEmbedded}},
			},
			FilePath: "/media/" + id + ".mp4",
		})
	}
	srv := New(&cfg, store, library.NewScanner(store), nil)
	srv.logger.SetOutput(io.Discard)
	ts := httptest.NewServer(srv.Handler())
	b.Cleanup(ts.Close)
	return ts
}

// BenchmarkPages measures full page load under warm, repeatable conditions:
// same server, same item count, gzip accepted, response fully drained.
func BenchmarkPages(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		ts := newPageBenchServer(b, n)
		pages := []struct {
			name  string
			path  string
			gzip  bool
			token bool
		}{
			{"index_html", "/", true, false},
			{"audiobooks_html", "/audiobooks", true, false},
			{"api_library_gzip", "/api/library", true, false},
			{"api_library_304", "/api/library", true, false}, // second pass sets INM below
			{"api_status", "/api/status", true, false},
			{"api_health", "/api/health", false, false},
		}
		for _, p := range pages {
			b.Run(fmt.Sprintf("%s/items%d", p.name, n), func(b *testing.B) {
				req, _ := http.NewRequest("GET", ts.URL+p.path, nil)
				if p.gzip {
					req.Header.Set("Accept-Encoding", "gzip")
				}
				etag := ""
				for i := 0; i < b.N; i++ {
					if p.name == "api_library_304" && etag != "" {
						req.Header.Set("If-None-Match", etag)
					} else {
						req.Header.Del("If-None-Match")
					}
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						b.Fatal(err)
					}
					if etag == "" {
						etag = resp.Header.Get("ETag")
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode != 200 && resp.StatusCode != 304 {
						b.Fatalf("%s status %d", p.path, resp.StatusCode)
					}
				}
			})
		}
	}
}
