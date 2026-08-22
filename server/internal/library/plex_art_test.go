package library

import (
	"os"
	"path/filepath"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func TestPlexArtPathsMovieDedicatedFolder(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Film.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := &Item{MediaItem: api.MediaItem{Kind: api.KindMovie}, FilePath: file}
	poster, fanart := PlexArtPaths(item)
	if poster != filepath.Join(dir, "poster.jpg") || fanart != filepath.Join(dir, "fanart.jpg") {
		t.Errorf("dedicated folder: %q %q", poster, fanart)
	}
}

func TestPlexArtPathsMovieSharedFolder(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"A.mp4", "B.mp4", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600)
	}
	item := &Item{MediaItem: api.MediaItem{Kind: api.KindMovie}, FilePath: filepath.Join(dir, "B.mp4")}
	poster, _ := PlexArtPaths(item)
	want := filepath.Join(dir, "B-poster.jpg")
	if poster != want {
		t.Errorf("shared folder poster = %q, want %q", poster, want)
	}
}

func TestPlexArtPathsTVShowSeasonFolder(t *testing.T) {
	root := t.TempDir()
	showDir := filepath.Join(root, "Lost")
	seasonDir := filepath.Join(showDir, "Season 3")
	os.MkdirAll(seasonDir, 0o755)
	file := filepath.Join(seasonDir, "S03E01.mkv")
	os.WriteFile(file, []byte("x"), 0o600)

	item := &Item{MediaItem: api.MediaItem{Kind: api.KindTVShow}, FilePath: file}
	poster, fanart := PlexArtPaths(item)
	if poster != filepath.Join(showDir, "poster.jpg") {
		t.Errorf("show poster = %q", poster)
	}
	if fanart != filepath.Join(showDir, "fanart.jpg") {
		t.Errorf("show fanart = %q", fanart)
	}
}

func TestPlexArtPathsTVShowFlatFolder(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "Lost S01E01.mkv")
	os.WriteFile(file, []byte("x"), 0o600)

	item := &Item{MediaItem: api.MediaItem{Kind: api.KindTVShow}, FilePath: file}
	poster, _ := PlexArtPaths(item)
	if poster != filepath.Join(dir, "poster.jpg") {
		t.Errorf("flat show poster = %q (no season dir -> show level)", poster)
	}
}

func TestCopyArtToNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.jpg")
	dst := filepath.Join(dir, "poster.jpg")
	os.WriteFile(src, []byte("official"), 0o600)
	os.WriteFile(dst, []byte("user-art"), 0o600)

	got, err := CopyArtTo(src, dst)
	if err != nil || got != dst {
		t.Fatalf("got %v %v", got, err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "user-art" {
		t.Errorf("existing art was overwritten: %q", data)
	}

	dst2 := filepath.Join(dir, "new-poster.jpg")
	if _, err := CopyArtTo(src, dst2); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(dst2)
	if string(data2) != "official" {
		t.Errorf("copy failed: %q", data2)
	}
}

func TestScannerDiscoversPlexSidecarPoster(t *testing.T) {
	store := New()
	sc := NewScanner(store)

	dir := t.TempDir()
	media := filepath.Join(dir, "Solo Film.mp4")
	os.WriteFile(media, []byte("content-v1"), 0o600)

	lib := []config.Library{{ID: "m", Name: "M", Path: dir, Kind: "movie"}}
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	items := store.InternalItems()
	if len(items) != 1 || items[0].PosterPath != "" {
		t.Fatalf("unexpected initial state: %+v", items)
	}

	// Drop a Plex sidecar poster; next scan must discover it even though the
	// media file itself is unchanged.
	os.WriteFile(filepath.Join(dir, "Solo Film-poster.jpg"), []byte("jpegdata"), 0o600)
	if _, err := sc.ScanAll(lib); err != nil {
		t.Fatal(err)
	}
	it, ok := store.Get(items[0].ID)
	if !ok || it.PosterPath == "" || it.PosterSource != "local" || it.PosterURL == nil {
		t.Fatalf("sidecar poster not discovered: %+v %+v", it, ok)
	}
}
