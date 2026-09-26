package mediacache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
)

func TestAcquireCopiesInBackgroundAndReopensAfterRestart(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "book.m4b")
	want := []byte("audiobook bytes")
	if err := os.WriteFile(source, want, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	media := Media{ID: "book-1", SourcePath: source, Kind: api.KindAudiobook, Size: st.Size(), ModTime: st.ModTime()}
	cacheDir := filepath.Join(root, "cache")

	cache, err := New(Config{Enabled: true, Dir: cacheDir, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	path, release, hit := cache.Acquire(media)
	if hit || path != source {
		t.Fatalf("first acquire = path %q hit %t", path, hit)
	}
	release()
	waitFor(t, func() bool { return cache.Status().CachedFiles == 1 })

	path, release, hit = cache.Acquire(media)
	if !hit || path == source {
		t.Fatalf("second acquire = path %q hit %t", path, hit)
	}
	release()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("cached bytes = %q, want %q", got, want)
	}
	cache.Close()

	cache, err = New(Config{Enabled: true, Dir: cacheDir, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	path, release, hit = cache.Acquire(media)
	defer release()
	if !hit || path == source {
		t.Fatalf("restart acquire = path %q hit %t", path, hit)
	}
}

func TestPriorityEvictsOlderMovieBeforeAudiobook(t *testing.T) {
	root := t.TempDir()
	cache, err := New(Config{Enabled: true, Dir: filepath.Join(root, "cache"), MaxBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	oldMovie := writeMedia(t, root, "movie", api.KindMovie, 1990, []byte("123456"))
	audiobook := writeMedia(t, root, "book", api.KindAudiobook, 0, []byte("abcdef"))
	cache.Acquire(oldMovie)
	waitFor(t, func() bool { return cache.Status().CachedFiles == 1 })
	cache.Acquire(audiobook)
	waitFor(t, func() bool {
		_, release, hit := cache.Acquire(audiobook)
		release()
		return hit
	})

	_, _, movieHit := cache.Acquire(oldMovie)
	_, release, bookHit := cache.Acquire(audiobook)
	release()
	if movieHit || !bookHit {
		t.Fatalf("eviction result: movieHit=%t audiobookHit=%t", movieHit, bookHit)
	}
}

func TestNewerMovieWinsAgainstOlderMovie(t *testing.T) {
	if priority(api.KindMovie, 2025) <= priority(api.KindMovie, 1990) {
		t.Fatal("newer movie did not receive a higher retention priority")
	}
	if priority(api.KindAudiobook, 0) <= priority(api.KindMovie, 2025) {
		t.Fatal("audiobook should outrank a newer movie")
	}
	if priority(api.KindEbook, 0) <= priority(api.KindAudiobook, 0) {
		t.Fatal("ebook should have the highest retention priority")
	}
}

func TestReconfigureChangesCapacityAndEnabledState(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	cache, err := New(Config{Enabled: false, Dir: cacheDir, MaxBytes: 12})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if cache.Status().Enabled {
		t.Fatal("cache should start disabled")
	}

	if err := cache.Reconfigure(Config{Enabled: true, Dir: cacheDir, MaxBytes: 12, MinFreeBytes: 0}); err != nil {
		t.Fatal(err)
	}
	if !cache.Status().Enabled {
		t.Fatal("cache did not enable")
	}

	first := writeMedia(t, root, "first", api.KindMovie, 1990, []byte("123456"))
	second := writeMedia(t, root, "second", api.KindMovie, 2025, []byte("abcdef"))
	cache.Acquire(first)
	cache.Acquire(second)
	waitFor(t, func() bool { return cache.Status().CachedFiles == 2 })

	if err := cache.Reconfigure(Config{Enabled: true, Dir: cacheDir, MaxBytes: 6, MinFreeBytes: 0}); err != nil {
		t.Fatal(err)
	}
	status := cache.Status()
	if status.MaxBytes != 6 || status.CachedBytes > 6 || status.CachedFiles != 1 {
		t.Fatalf("shrunk cache status = %+v", status)
	}

	if err := cache.Reconfigure(Config{Enabled: false, Dir: cacheDir, MaxBytes: 6, MinFreeBytes: 0}); err != nil {
		t.Fatal(err)
	}
	if cache.Status().Enabled {
		t.Fatal("cache did not disable")
	}
	if path, _, hit := cache.Acquire(first); hit || path != first.SourcePath {
		t.Fatalf("disabled acquire = path %q hit %t", path, hit)
	}
	if err := cache.Reconfigure(Config{Enabled: true, Dir: cacheDir, MaxBytes: 6, MinFreeBytes: 0}); err != nil {
		t.Fatal(err)
	}
	status = cache.Status()
	if status.CachedBytes > 6 || status.CachedFiles != 1 {
		t.Fatalf("re-enabled cache status = %+v", status)
	}
}

func writeMedia(t *testing.T, root, id string, kind api.MediaKind, year int, data []byte) Media {
	t.Helper()
	path := filepath.Join(root, id+".media")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return Media{ID: id, SourcePath: path, Kind: kind, Year: year, Size: st.Size(), ModTime: st.ModTime()}
}

// waitFor polls an asynchronous condition.
//
// The deadline is generous on purpose. The cache does its work on a background
// worker, so how long a condition takes depends on I/O and on whatever else the
// machine is doing -- a full test run in the container machine while the server
// is scanning a library is not the same load as running this package alone. A
// 2 second budget was tight enough that this failed intermittently under load
// while passing every time in isolation, which is the worst kind of test: it
// teaches you to re-run rather than to look.
//
// It is still bounded, so a genuine deadlock fails instead of hanging.
const waitForTimeout = 15 * time.Second

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitForTimeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for media cache worker", waitForTimeout)
}
