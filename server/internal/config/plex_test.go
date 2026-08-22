package config

import (
	"os"
	"path/filepath"
	"testing"
)

func makePlexRoot(t *testing.T, children ...string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "plex")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range children {
		if err := os.MkdirAll(filepath.Join(root, c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestExpandPlexLibraries(t *testing.T) {
	root := makePlexRoot(t, "Movies", "TV Shows", "Audiobooks", "Ebooks", "random_junk", ".hidden")
	libs, err := ExpandPlexLibraries([]Library{
		{ID: "plex-root", Name: "Plex", Path: root, Kind: "plex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{} // name -> kind
	for _, l := range libs {
		got[l.Name] = l.Kind
	}
	want := map[string]string{
		"Movies":     "movie",
		"TV Shows":   "tvShow",
		"Audiobooks": "audiobook",
		"Ebooks":     "ebook",
	}
	if len(got) != len(want) {
		t.Fatalf("expanded to %d libraries (%v), want %d", len(got), got, len(want))
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("child %q kind = %q, want %q", name, got[name], kind)
		}
	}
	// Deterministic IDs so restarts keep library/item identity.
	again, _ := ExpandPlexLibraries([]Library{{Name: "Plex", Path: root, Kind: "plex"}})
	for i := range libs {
		if libs[i].ID != again[i].ID || libs[i].Path != again[i].Path {
			t.Errorf("expansion not deterministic: %+v vs %+v", libs[i], again[i])
		}
		if libs[i].Path != filepath.Join(root, libs[i].Name) {
			t.Errorf("path = %q, want child of root", libs[i].Path)
		}
	}
}

func TestExpandPlexLibrariesSingularAndCaseForms(t *testing.T) {
	root := makePlexRoot(t, "Movie", "tv", "audiobook", "Books")
	libs, err := ExpandPlexLibraries([]Library{{Path: root, Kind: "plex"}})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, l := range libs {
		kinds[l.Kind] = true
	}
	for _, k := range []string{"movie", "tvShow", "audiobook", "ebook"} {
		if !kinds[k] {
			t.Errorf("missing expanded kind %q (got %v)", k, kinds)
		}
	}
}

func TestExpandPlexLibrariesKeepsConcreteLibraries(t *testing.T) {
	libs := []Library{
		{ID: "m1", Name: "Movies", Path: "/data/movies", Kind: "movie"},
	}
	out, err := ExpandPlexLibraries(libs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Kind != "movie" || out[0].ID != "m1" {
		t.Fatalf("concrete library altered: %+v", out)
	}
}

func TestExpandPlexLibrariesMissingRoot(t *testing.T) {
	if _, err := ExpandPlexLibraries([]Library{
		{Path: filepath.Join(t.TempDir(), "nope"), Kind: "plex"},
	}); err == nil {
		t.Error("expected error for missing plex root")
	}
}

func TestLoadExpandsPlexKind(t *testing.T) {
	root := makePlexRoot(t, "Movies", "Audiobooks")
	p := write(t, `{"port": 8797, "dataDir": "`+t.TempDir()+`", "libraries": [{"name": "Plex", "path": "`+root+`", "kind": "plex"}]}`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Libraries) != 2 {
		t.Fatalf("libraries = %+v, want 2 expanded entries", cfg.Libraries)
	}
	if cfg.Libraries[0].Kind != "audiobook" || cfg.Libraries[1].Kind != "movie" {
		t.Errorf("kinds wrong (expect sorted names Audiobooks, Movies): %+v", cfg.Libraries)
	}
}
