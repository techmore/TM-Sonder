package library

import (
	"path/filepath"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

func TestPortableBundleRemapsPathsAndPreservesUserData(t *testing.T) {
	const itemID = "book-1"
	const libraryID = "audio"
	oldRoot := "/old/nas/Audiobooks"
	newRoot := "/media/Audiobooks"

	source := New()
	source.Upsert(&Item{
		MediaItem:          api.MediaItem{ID: itemID, Title: "A Book", Kind: api.KindAudiobook, LibraryID: stringPtr(libraryID)},
		FilePath:           filepath.Join(oldRoot, "A Book.m4b"),
		StableKey:          StableMediaKey(libraryID, "A Book.m4b"),
		SourceRelativePath: "A Book.m4b",
	})
	if _, ok := source.CreateList("To read", "Portable list", []string{"priority"}); !ok {
		t.Fatal("create list failed")
	}
	list := source.Lists()[0]
	if _, ok := source.AddListItem(list.ID, itemID, -1, []string{"weekend"}); !ok {
		t.Fatal("add list item failed")
	}
	source.SetProgress(api.ProgressRecord{ItemID: itemID, Seconds: 42, Duration: 100, UpdatedAt: time.Now().UTC()})

	bundle := source.ExportBundle([]config.Library{{ID: libraryID, Name: "Audiobooks", Path: oldRoot, Kind: string(api.KindAudiobook)}})
	remapped, err := RemapBundlePaths(&bundle, []config.Library{{ID: libraryID, Name: "Audiobooks", Path: newRoot, Kind: string(api.KindAudiobook)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if remapped != 1 {
		t.Fatalf("paths remapped = %d, want 1", remapped)
	}
	if got := bundle.Snapshot.Items[0].FilePath; got != filepath.Join(newRoot, "A Book.m4b") {
		t.Fatalf("remapped path = %q", got)
	}

	destination := New()
	result, err := destination.ImportBundle(bundle, "replace")
	if err != nil {
		t.Fatal(err)
	}
	if result.Items != 1 || result.Lists != 1 || result.Progress != 1 {
		t.Fatalf("import result = %+v", result)
	}
	item, ok := destination.Get(itemID)
	if !ok || item.FilePath != filepath.Join(newRoot, "A Book.m4b") || item.StableKey != StableMediaKey(libraryID, "A Book.m4b") {
		t.Fatalf("imported item = %+v, ok=%v", item, ok)
	}
	if got := destination.Lists()[0].ItemIDs; len(got) != 1 || got[0] != itemID {
		t.Fatalf("imported list items = %v", got)
	}
}

func TestSnapshotWithoutVersionMigrates(t *testing.T) {
	s := New()
	path := filepath.Join(t.TempDir(), "library.json")
	data := []byte(`{"generation":7,"items":[],"progress":[],"lists":[{"id":"l1","name":"List","itemIDs":null,"itemTags":null}]}`)
	if err := writeAtomic(path, data, ".test-"); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(path); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().SchemaVersion; got != CurrentSnapshotVersion {
		t.Fatalf("schema version = %d, want %d", got, CurrentSnapshotVersion)
	}
	if got := s.Lists()[0].ItemIDs; got == nil {
		t.Fatal("migrated list item IDs remained nil")
	}
}

func stringPtr(value string) *string { return &value }
