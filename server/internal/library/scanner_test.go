package library

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/probe"
)

// fixtureTree builds a small on-disk library:
//
//	root/
//	  Shows/Breaking Bad/Season 1/Breaking Bad S01E01 Pilot.mkv (+ .srt, poster)
//	  Movies/Inception (2010)/Inception.2010.mp4 (+ backdrop in parent)
//	  Books/Dune.epub
func fixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, contents string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ep := mk("Shows/Breaking Bad/Season 1/Breaking Bad S01E01 Pilot.mkv", "v")
	mk("Shows/Breaking Bad/Season 1/Breaking Bad S01E01 Pilot.srt", "sub")
	mk("Shows/Breaking Bad/poster.jpg", "art")
	mk("Movies/backdrop.jpg", "fanart")
	mk("Movies/Inception (2010)/Inception.2010.mp4", "m")
	mk("Books/Dune.epub", "b")
	_ = ep
	return root
}

func TestScanFixtureSxxEyyGrouping(t *testing.T) {
	root := fixtureTree(t)
	store := New()
	sc := NewScanner(store)

	res, err := sc.ScanAll([]config.Library{
		{ID: "tv", Name: "TV", Path: filepath.Join(root, "Shows"), Kind: "tvShow"},
		{ID: "movies", Name: "Movies", Path: filepath.Join(root, "Movies"), Kind: "movie"},
		{ID: "books", Name: "Books", Path: filepath.Join(root, "Books"), Kind: "ebook"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Skipped != 0 || res.Removed != 0 {
		t.Fatalf("scan result = %+v", res)
	}
	items := store.Items()
	if len(items) != 3 {
		t.Fatalf("items = %d", len(items))
	}

	var episode, movie, book *Item
	for _, it := range store.InternalItems() {
		switch it.Title {
		case "Pilot":
			episode = it
		case "Inception":
			movie = it
		case "Dune":
			book = it
		}
	}
	if episode == nil || movie == nil || book == nil {
		t.Fatalf("missing parsed items: %+v", items)
	}

	if episode.ShowTitle == nil || *episode.ShowTitle != "Breaking Bad" ||
		*episode.SeasonNumber != 1 || *episode.EpisodeNumber != 1 ||
		episode.Kind != api.KindTVShow {
		t.Errorf("episode fields wrong: %+v", episode.MediaItem)
	}
	if len(episode.EmbeddedSubtitleTracks) != 0 {
		t.Errorf("sidecar leaked into embedded tracks: %+v", episode.EmbeddedSubtitleTracks)
	}
	if len(episode.SidecarPaths) != 1 {
		t.Fatalf("sidecar paths = %+v", episode.SidecarPaths)
	}
	merged := MergedSubtitleTracks(episode)
	wantLabel := "Breaking Bad S01E01 Pilot"
	if len(merged) != 1 || merged[0].ID != "sidecar:0" || merged[0].URL == nil ||
		*merged[0].URL != "/subtitles/"+episode.ID+"/0" || merged[0].Label != wantLabel {
		t.Errorf("merged sidecar track wrong: %+v", merged)
	}
	if episode.SidecarPaths[0] == "" || episode.PosterPath == "" {
		t.Errorf("assets missing: sidecar=%q poster=%q", episode.SidecarPaths, episode.PosterPath)
	}
	if movie.Kind != api.KindMovie || movie.Year != 2010 || movie.BackdropPath == "" {
		t.Errorf("movie fields wrong: %+v", movie.MediaItem)
	}
	if book.Kind != api.KindEbook {
		t.Errorf("book kind = %s", book.Kind)
	}
}

func TestRescanKeepsIDsAndProgress(t *testing.T) {
	root := fixtureTree(t)
	store := New()
	sc := NewScanner(store)
	lib := []config.Library{
		{ID: "tv", Name: "TV", Path: filepath.Join(root, "Shows"), Kind: "tvShow"},
	}

	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	first := store.InternalItems()
	var epID string
	for _, it := range first {
		if it.ShowTitle != nil && *it.ShowTitle == "Breaking Bad" {
			epID = it.ID
		}
	}
	if epID == "" {
		t.Fatal("episode not found after first scan")
	}
	store.SetProgress(api.ProgressRecord{
		ItemID:    epID,
		Seconds:   300,
		Duration:  1800,
		UpdatedAt: time.Now().UTC(),
	})
	_ = store.ETag()

	// Touch nothing; rescan must skip unchanged and keep IDs + progress.
	res, err := sc.ScanAll(lib)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped < 3 {
		t.Logf("rescan result: %+v", res)
	}
	it, ok := store.Get(epID)
	if !ok || it.ID != epID {
		t.Fatal("item id changed across rescan")
	}
	p, _ := store.ProgressFor(epID)
	if p.Seconds != 300 {
		t.Errorf("progress lost across rescan")
	}

	// Modify the file -> must be re-scanned (updated), same ID.
	epPath := filepath.Join(root, "Shows", "Breaking Bad", "Season 1", "Breaking Bad S01E01 Pilot.mkv")
	if err := os.WriteFile(epPath, []byte("changed-content-longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	res2, err := sc.ScanAll(lib)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Updated != 1 {
		t.Errorf("modified file not updated: %+v", res2)
	}
	if _, ok := store.Get(epID); !ok {
		t.Error("id changed after modification rescan")
	}
}

func TestRemovalDetection(t *testing.T) {
	root := fixtureTree(t)
	store := New()
	sc := NewScanner(store)
	lib := []config.Library{
		{ID: "books", Name: "Books", Path: filepath.Join(root, "Books"), Kind: "ebook"},
	}
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	before := store.Count()
	if before != 1 {
		t.Fatalf("count = %d", before)
	}
	if err := os.Remove(filepath.Join(root, "Books", "Dune.epub")); err != nil {
		t.Fatal(err)
	}
	res, err := sc.ScanAll(lib)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 || store.Count() != 0 {
		t.Errorf("removal not detected: %+v count=%d", res, store.Count())
	}
}

// A subtitle added between scans to an otherwise unchanged media file must
// be picked up on the next pass: the per-scan directory-listing memo must
// not serve a listing captured by an earlier scan.
func TestRescanDetectsSidecarAddedBetweenScans(t *testing.T) {
	root := fixtureTree(t)
	store := New()
	sc := NewScanner(store)
	lib := []config.Library{
		{ID: "tv", Name: "TV", Path: filepath.Join(root, "Shows"), Kind: "tvShow"},
	}
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	// Second pass with nothing changed; the memo now holds the Season 1
	// listing from the previous scan.
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	// Drop a new sidecar into the episode folder without touching the media
	// file. Size and mtime are unchanged, so only a fresh directory listing
	// can reveal the new sidecar.
	newSrt := filepath.Join(root, "Shows", "Breaking Bad", "Season 1", "Breaking Bad S01E01 Pilot.en.srt")
	if err := os.WriteFile(newSrt, []byte("sub2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	var episode *Item
	for _, it := range store.InternalItems() {
		if it.Title == "Pilot" {
			episode = it
		}
	}
	if episode == nil {
		t.Fatal("episode missing after rescan")
	}
	if len(episode.SidecarPaths) != 2 {
		t.Errorf("sidecar added between scans not detected: paths = %v", episode.SidecarPaths)
	}
}

// fakeProber records concurrency and call count for pool tests.
type fakeProber struct {
	onProbe func()
	calls   atomic.Int32
}

func (f *fakeProber) ProbeResult(ctx context.Context, path string, size int64, mod time.Time) (*probe.Result, error) {
	f.calls.Add(1)
	if f.onProbe != nil {
		f.onProbe()
	}
	return &probe.Result{DurationSeconds: 42}, nil
}

func TestScanProbesNewAndChangedWithBoundedWorkers(t *testing.T) {
	root := fixtureTree(t)
	store := New()
	sc := NewScanner(store)

	var mu sync.Mutex
	cur, peak := 0, 0
	fp := &fakeProber{onProbe: func() {
		mu.Lock()
		cur++
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		cur--
		mu.Unlock()
	}}
	sc.SetProber(fp, 2)

	lib := []config.Library{
		{ID: "tv", Name: "TV", Path: filepath.Join(root, "Shows"), Kind: "tvShow"},
	}
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	if n := fp.calls.Load(); n < 1 {
		t.Fatalf("prober calls = %d on first scan", n)
	}

	// Unchanged rescan -> no probe calls at all.
	before := fp.calls.Load()
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	if after := fp.calls.Load(); after != before {
		t.Errorf("unchanged rescan probed %d more files", after-before)
	}

	it := store.InternalItems()[0]
	if it.DurationSeconds != 42 || it.TrackProbeUpdatedAt == nil {
		t.Errorf("probe result not applied: dur=%v at=%v", it.DurationSeconds, it.TrackProbeUpdatedAt)
	}
	if peak > 2 {
		t.Errorf("peak concurrent probes = %d, want <= 2", peak)
	}

	// Change a file -> exactly one new probe.
	epPath := filepath.Join(root, "Shows", "Breaking Bad", "Season 1", "Breaking Bad S01E01 Pilot.mkv")
	if err := os.WriteFile(epPath, []byte("bigger content now"), 0o600); err != nil {
		t.Fatal(err)
	}
	before = fp.calls.Load()
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	if after := fp.calls.Load(); after-before != 1 {
		t.Errorf("changed file probed %d times, want 1", after-before)
	}
}
