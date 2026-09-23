package library

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tm-sonder/server/internal/config"
)

func buildPlexFixture(t *testing.T) string {
	t.Helper()
	sqlite, err := findSQLite3()
	if err != nil {
		t.Skip("sqlite3 not available")
	}
	dir := t.TempDir()
	media := filepath.Join(dir, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		t.Fatal(err)
	}
	movie := filepath.Join(media, "Film.mp4")
	ep := filepath.Join(media, "Show S01E01.mkv")
	for _, p := range []string{movie, ep} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	db := filepath.Join(dir, "plex.db")
	sql := `
create table metadata_items (
  id integer primary key, parent_id integer, library_section_id integer,
  metadata_type integer, title text, summary text, studio text,
  year integer, tags_genre text, guid text, refreshed_at integer,
  deleted_at integer, "index" integer
);
create table media_items (id integer primary key, metadata_item_id integer);
create table media_parts (id integer primary key, media_item_id integer, file text, deleted_at integer);
create table directories (id integer primary key, path text);

insert into metadata_items values (1, null, 1, 1, 'Film (1999)', 'A film summary', 'StudioX', 1999, 'Drama|Thriller', 'com.plexapp.agents.imdb://tt0133093?lang=en', 100, null, null);
insert into media_items values (10, 1);
insert into media_parts values (100, 10, '` + movie + `', null);

insert into metadata_items values (2, null, 2, 2, 'The Show', '', '', null, '', 'plex://show/abc', 90, null, null);
insert into metadata_items values (3, 2, 2, 2, 'Season 1', '', '', null, '', null, 80, null, 1);
insert into metadata_items values (4, 3, 2, 4, 'Pilot', 'Ep summary', null, null, '', 'com.plexapp.agents.thetvdb://1234?lang=en', 70, null, 1);
insert into media_items values (20, 4);
insert into media_parts values (200, 20, '` + ep + `', null);

-- a movie whose file is missing on disk: must be skipped
insert into metadata_items values (5, null, 1, 1, 'Ghost', '', '', 2000, '', null, 60, null, null);
insert into media_items values (30, 5);
insert into media_parts values (300, 30, '/definitely/missing/ghost.mkv', null);
`
	cmd := exec.Command(sqlite, db)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture db failed: %v: %s", err, out)
	}
	return db
}

func TestImportPlexLibrary(t *testing.T) {
	db := buildPlexFixture(t)
	store := New()
	res, err := ImportPlexLibrary(store, db, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if res.Movies != 1 || res.Episodes != 1 || res.Shows != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v", res)
	}
	if store.Count() != 2 {
		t.Fatalf("count = %d", store.Count())
	}

	var movie, episode *Item
	for _, it := range store.InternalItems() {
		switch it.Title {
		case "Film":
			movie = it
		case "Pilot":
			episode = it
		}
	}
	if movie == nil || episode == nil {
		t.Fatalf("items missing: %+v", store.Items())
	}

	if movie.Kind != apiKindMovie() || movie.Year != 1999 ||
		movie.MetadataIDSource == nil || *movie.MetadataIDSource != "imdb" ||
		movie.MetadataID == nil || *movie.MetadataID != "tt0133093" ||
		len(movie.Tags) != 2 {
		t.Errorf("movie mapping wrong: %+v", movie.MediaItem)
	}
	if movie.FilePath == "" || movie.SizeBytes == 0 {
		t.Errorf("movie file fields wrong: path=%q size=%d", movie.FilePath, movie.SizeBytes)
	}

	if episode.ShowTitle == nil || *episode.ShowTitle != "The Show" ||
		*episode.SeasonNumber != 1 || *episode.EpisodeNumber != 1 ||
		!strings.Contains(episode.Subtitle, "S01E01") {
		t.Errorf("episode mapping wrong: %+v", episode.MediaItem)
	}

	// Stable IDs reconcile with scanner-derived IDs for the same path.
	if movie.ID != StableID(movie.FilePath) {
		t.Errorf("movie ID not path-stable: %s vs %s", movie.ID, StableID(movie.FilePath))
	}
}

func TestPlexGUIDParsing(t *testing.T) {
	cases := []struct{ in, src, id string }{
		{"com.plexapp.agents.imdb://tt0133093?lang=en", "imdb", "tt0133093"},
		{"com.plexapp.agents.themoviedb://693134?language=en-US", "tmdb", "693134"},
		{"plex://movie/5d776b3ad19a9c001f4a1b2c", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		src, id := plexGUID(c.in)
		if src != c.src || id != c.id {
			t.Errorf("plexGUID(%q) = %q/%q, want %q/%q", c.in, src, id, c.src, c.id)
		}
	}
}

func TestPlexFirstFileHandlesCommasInPaths(t *testing.T) {
	dir := t.TempDir()
	withComma := filepath.Join(dir, "Movie, The (1999).mkv")
	if err := os.WriteFile(withComma, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	joined := filepath.Join(dir, "missing.mkv") + plexFileSep + withComma
	if got := plexFirstFile(joined); got != withComma {
		t.Errorf("plexFirstFile = %q, want %q", got, withComma)
	}
	// A single path with no separator must still work.
	if got := plexFirstFile(withComma); got != withComma {
		t.Errorf("single path = %q, want %q", got, withComma)
	}
}

func TestCanonicalMediaPathResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.mkv")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.mkv")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	want := CanonicalMediaPath(real) // idempotent: also resolves /var -> /private/var
	if got := CanonicalMediaPath(link); got != want {
		t.Errorf("CanonicalMediaPath(link) = %q, want %q", got, want)
	}
}

// TestPlexImportIDMatchesScannerID guards the pruning bug: an imported item's
// StableID must equal the ID the filesystem scanner assigns to the same file,
// otherwise the next scan removes every imported item.
func TestPlexImportIDMatchesScannerID(t *testing.T) {
	root := fixtureTree(t)
	// The scanner resolves symlinks in the library root, so compare on the
	// canonical form (macOS /var is a symlink to /private/var).
	moviePath := CanonicalMediaPath(filepath.Join(root, "Movies", "Inception (2010)", "Inception.2010.mp4"))

	store := New()
	sc := NewScanner(store)
	if _, err := sc.ScanAll([]config.Library{
		{ID: "movies", Name: "Movies", Path: filepath.Join(root, "Movies"), Kind: "movie"},
	}); err != nil {
		t.Fatal(err)
	}
	var scannerID string
	for _, it := range store.InternalItems() {
		if it.FilePath == moviePath {
			scannerID = it.ID
		}
	}
	if scannerID == "" {
		t.Fatalf("scanner did not catalog %s", moviePath)
	}

	imported := plexItem(moviePath, apiKindMovie(), "Inception", "", 2010, "", "", "", "", "")
	if imported.ID != scannerID {
		t.Errorf("imported ID %s != scanner ID %s (item would be pruned)", imported.ID, scannerID)
	}
}
