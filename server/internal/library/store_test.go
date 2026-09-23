package library

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
)

func testItem(id, title string) *Item {
	return &Item{
		MediaItem: api.MediaItem{
			ID: id, Title: title, Kind: api.KindMovie,
			Format: api.FormatMP4, Tags: []string{"a"},
		},
		FilePath: "/media/" + title + ".mp4",
	}
}

func TestGenerationBumpsOnMutation(t *testing.T) {
	s := New()
	g0 := s.Generation()
	s.Upsert(testItem("a", "Alpha"))
	if s.Generation() != g0+1 {
		t.Fatalf("upsert did not bump generation")
	}
	etag1 := s.ETag()
	if etag1 != `"sonder-library-1"` {
		t.Errorf("etag = %s", etag1)
	}
	s.SetProgress(api.ProgressRecord{ItemID: "a", Seconds: 10})
	if s.ETag() == etag1 {
		t.Error("progress write did not bump ETag")
	}
	s.RecordActivity("t", "d", "i")
	s.SetDirectories([]api.MediaDirectory{{ID: "d1"}})
	if s.Remove("nope") != 0 {
		t.Error("remove of missing id reported success")
	}
	gen := s.Generation()
	s.Upsert(testItem("a", "Alpha")) // same ID replace still mutates
	if s.Generation() != gen+1 {
		t.Error("replace upsert did not bump generation")
	}
}

func TestProgressLastWriteWins(t *testing.T) {
	s := New()
	s.Upsert(testItem("a", "Alpha"))

	newer := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Minute)

	s.SetProgress(api.ProgressRecord{ItemID: "a", Seconds: 100, UpdatedAt: newer})
	if ok := s.SetProgress(api.ProgressRecord{ItemID: "a", Seconds: 5, UpdatedAt: older}); ok {
		t.Fatal("stale progress overwrote newer record")
	}
	p, _ := s.ProgressFor("a")
	if p.Seconds != 100 {
		t.Errorf("seconds = %v, want 100", p.Seconds)
	}
	it, _ := s.Get("a")
	if it.ProgressSeconds != 100 {
		t.Errorf("item merge failed: %v", it.ProgressSeconds)
	}
}

func TestActivityRingBuffer(t *testing.T) {
	s := New()
	for i := 0; i < ActivityCap+50; i++ {
		s.RecordActivity("e", "", "")
	}
	if got := len(s.Activity()); got != ActivityCap {
		t.Errorf("activity len = %d, want %d", got, ActivityCap)
	}
}

func TestSaveLoadRoundTripETagStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New()
	s.Upsert(testItem("b", "Beta"), testItem("a", "Alpha"))
	s.SetProgress(api.ProgressRecord{ItemID: "a", Seconds: 42, Duration: 100})
	s.RecordActivity("scanned", "2 items", "magazine")
	etag := s.ETag()

	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}

	fresh := New()
	if err := fresh.Load(path); err != nil {
		t.Fatal(err)
	}
	if got := fresh.ETag(); got != etag {
		t.Errorf("etag after restart = %s, want %s (304 must survive restart)", got, etag)
	}
	items := fresh.Items()
	if len(items) != 2 || items[0].Title != "Alpha" || items[1].Title != "Beta" {
		t.Errorf("items not restored/sorted: %+v", items)
	}
	it, ok := fresh.Get("a")
	if !ok || it.FilePath != "/media/Alpha.mp4" {
		t.Errorf("internal fields lost: %+v %+v", it, ok)
	}
	if _, ok := fresh.ProgressFor("a"); !ok {
		t.Error("progress not restored")
	}
}

func TestConcurrentProgressAndReads(t *testing.T) {
	s := New()
	for i := 0; i < 20; i++ {
		id := string(rune('a' + i))
		s.Upsert(testItem(id, "Item"+string(rune('A'+i))))
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.SetProgress(api.ProgressRecord{
					ID:        api.NewID(),
					ItemID:    string(rune('a' + (w+i)%20)),
					Seconds:   float64(i),
					UpdatedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
				})
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = s.Items()
				_ = s.ETag()
				_ = s.Activity()
			}
		}()
	}
	wg.Wait()
	if n := len(s.Progress()); n != 20 {
		t.Errorf("progress records = %d, want 20 (LWW per item)", n)
	}
}

func TestDebouncedSaveCoalesces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New()

	writes := make(chan error, 4)
	done := make(chan struct{})
	cb := func(err error) { writes <- err; done <- struct{}{} }

	s.SaveDebouncedWithCallback(path, 30*time.Millisecond, cb)
	s.SaveDebouncedWithCallback(path, 30*time.Millisecond, cb) // reschedules, first timer cancelled
	s.Upsert(testItem("x", "X"))
	s.SaveDebouncedWithCallback(path, 30*time.Millisecond, cb)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("debounced save never ran")
	}
	if err := <-writes; err != nil {
		t.Fatalf("save error: %v", err)
	}
	fresh := New()
	if err := fresh.Load(path); err != nil {
		t.Fatal(err)
	}
	if fresh.Count() != 1 {
		t.Errorf("count after reload = %d", fresh.Count())
	}
}

func TestFlushWithoutPendingSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	s := New()
	s.Upsert(testItem("q", "Quiet"))
	if err := s.Flush(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestImportSwiftLibraryJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "Film.mp4")
	if err := os.WriteFile(src, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	libID := "11111111-2222-3333-4444-555555555555"
	swift := `{
  "items": [
    {
      "id": "A1", "title": "Film", "subtitle": "", "kind": "movie",
      "studio": "", "year": 2020, "durationSeconds": 3600, "format": "mp4",
      "libraryID": "` + libID + `", "tags": ["fav"], "summary": "s",
      "author": "Octavia Butler", "narrator": "Robin Miles",
      "progressSeconds": 12.5,
      "sourcePath": "` + src + `",
      "localPosterPath": "` + filepath.Join(dir, "poster.jpg") + `",
      "subtitlePaths": ["` + filepath.Join(dir, "Film.srt") + `"],
      "embeddedAudioTracks": [{"id":"embedded-audio:0","label":"English","kind":"embedded"}],
      "trackProbeUpdatedAt": "2026-01-02T03:04:05Z",
      "probedWidth": 1920, "probedHeight": 1080
    },
    { "id": "GONE", "title": "Missing", "kind": "movie", "format": "mkv",
      "sourcePath": "/definitely/not/here.mkv" },
    { "id": "NOPATH", "title": "NoPath" }
  ],
  "progress": [
    {"id": "p1", "itemID": "A1", "seconds": 30, "duration": 3600,
     "updatedAt": "2026-02-03T04:05:06Z"}
  ],
  "mediaDirectories": [
    {"id": "d1", "name": "Movies", "path": "/media/movies", "bookmark": "AA==",
     "kind": "movies", "libraryID": "` + libID + `"}
  ],
  "serverSettings": {"isEnabled": true, "allowLAN": false, "port": 8797}
}`
	p := filepath.Join(dir, "library.json")
	if err := os.WriteFile(p, []byte(swift), 0o600); err != nil {
		t.Fatal(err)
	}

	s := New()
	res, err := ImportSwiftLibraryJSON(s, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items != 1 || res.MissingOnDisk != 1 || res.SkippedNoFile != 1 || res.Progress != 1 || res.Directories != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	it, ok := s.Get("A1")
	if !ok {
		t.Fatal("imported item missing")
	}
	if it.FilePath != src || it.PosterPath == "" || len(it.SidecarPaths) != 1 ||
		len(it.EmbeddedAudioTracks) != 1 || it.ProbedWidth == nil || *it.ProbedWidth != 1920 {
		t.Errorf("swift fields not mapped: %+v", it)
	}
	if it.Author == nil || *it.Author != "Octavia Butler" ||
		it.Narrator == nil || *it.Narrator != "Robin Miles" {
		t.Errorf("author/narrator dropped in migration: author=%v narrator=%v", it.Author, it.Narrator)
	}
	if it.PosterURL == nil || *it.PosterURL != "/artwork/poster/A1" {
		t.Errorf("poster URL not derived: %v", it.PosterURL)
	}
	if it.TrackProbeUpdatedAt == nil || it.TrackProbeUpdatedAt.UTC() != time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) {
		t.Errorf("ISO8601 date parse failed: %v", it.TrackProbeUpdatedAt)
	}
	dirs := s.Activity()
	if len(dirs) == 0 {
		t.Error("no import activity recorded")
	}
}

// TestUpdateMergesConcurrentWriters guards the lost-update fix: a probe-style
// Update must not revert progress written concurrently by SetProgress, and
// neither writer may lose its own field.
func TestUpdateMergesConcurrentWriters(t *testing.T) {
	s := New()
	s.Upsert(testItem("x", "X"))

	const n = 200
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.Update("x", func(it *Item) bool {
				it.Tags = append(it.Tags, "probe")
				return true
			})
		}(i)
		go func(i int) {
			defer wg.Done()
			s.SetProgress(api.ProgressRecord{
				ItemID:    "x",
				Seconds:   float64(i + 1),
				Duration:  100,
				UpdatedAt: time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
			})
		}(i)
	}
	wg.Wait()

	it, ok := s.Get("x")
	if !ok {
		t.Fatal("item vanished")
	}
	if it.ProgressSeconds <= 0 {
		t.Errorf("progress clobbered by Update: %v", it.ProgressSeconds)
	}
	// One initial tag plus one per Update call.
	if len(it.Tags) != n+1 {
		t.Errorf("probe updates lost: %d tags, want %d", len(it.Tags), n+1)
	}
}

// TestUpdateReportsExistenceAndChange verifies Update's contract: it reports
// whether the item existed, and skips the generation bump when unchanged.
func TestUpdateReportsExistenceAndChange(t *testing.T) {
	s := New()
	s.Upsert(testItem("x", "X"))

	if s.Update("missing", func(*Item) bool { return true }) {
		t.Error("Update reported existence for a missing item")
	}

	gen := s.Generation()
	if !s.Update("x", func(*Item) bool { return false }) {
		t.Error("Update reported missing for an existing item")
	}
	if s.Generation() != gen {
		t.Errorf("no-op update bumped generation: %d -> %d", gen, s.Generation())
	}
	if !s.Update("x", func(it *Item) bool { it.Year = 1999; return true }) {
		t.Error("Update failed")
	}
	if s.Generation() == gen {
		t.Error("changing update did not bump generation")
	}
}

// TestProgressSidecarRoundTrip verifies progress is persisted independently of
// the catalog and overlays newer positions on load.
func TestProgressSidecarRoundTrip(t *testing.T) {
	dir := t.TempDir()
	catalogPath := filepath.Join(dir, "library.json")
	progressPath := filepath.Join(dir, "progress.json")

	s := New()
	s.Upsert(&Item{
		MediaItem: api.MediaItem{ID: "a", Title: "ZZCatalogOnlyZZ", Kind: api.KindMovie, Format: api.FormatMP4},
		FilePath:  "/media/zz-catalog-only.mp4",
	})
	if err := s.Save(catalogPath); err != nil {
		t.Fatal(err)
	}
	if !s.SetProgress(api.ProgressRecord{
		ItemID: "a", Seconds: 99, Duration: 100, UpdatedAt: time.Now().UTC(),
	}) {
		t.Fatal("progress not applied")
	}
	if err := s.SaveProgress(progressPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(progressPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("ZZCatalogOnlyZZ")) || bytes.Contains(data, []byte("zz-catalog-only.mp4")) {
		t.Errorf("progress sidecar unexpectedly contains catalog data: %s", data)
	}

	// Fresh store: catalog snapshot (no progress) then the newer sidecar.
	s2 := New()
	if err := s2.Load(catalogPath); err != nil {
		t.Fatal(err)
	}
	if err := s2.LoadProgress(progressPath); err != nil {
		t.Fatal(err)
	}
	rec, ok := s2.ProgressFor("a")
	if !ok || rec.Seconds != 99 {
		t.Fatalf("progress not restored: %+v ok=%v", rec, ok)
	}
	it, ok := s2.Get("a")
	if !ok || it.ProgressSeconds != 99 {
		t.Fatalf("item progress not overlaid: %+v", it)
	}
}
