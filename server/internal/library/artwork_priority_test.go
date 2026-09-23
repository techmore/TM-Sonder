package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnFolderCoverWinsOverParentExactCase(t *testing.T) {
	parent := t.TempDir()
	folder := filepath.Join(parent, "Title")
	if err := os.Mkdir(folder, 0755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(folder, "Cover.JPG")
	for _, path := range []string{local, filepath.Join(parent, "poster.jpg")} {
		if err := os.WriteFile(path, []byte("art"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := firstExisting(folder, parent, artworkPosterNames, "Title")
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(local)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("selected %q; want title-local %q", got, local)
	}
}
