package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

func TestDataExportImportRemapsWithoutScanning(t *testing.T) {
	oldRoot := "/old/nas/Audiobooks"
	newRoot := "/media/Audiobooks"
	const libraryID = "audio"

	source := newFixture(t, func(cfg *config.Config) {
		cfg.Libraries = []config.Library{{ID: libraryID, Name: "Audiobooks", Path: oldRoot, Kind: string(api.KindAudiobook)}}
	})
	source.store.Upsert(&library.Item{
		MediaItem: api.MediaItem{ID: "book-1", Title: "A Book", Kind: api.KindAudiobook, LibraryID: stringPointer(libraryID)},
		FilePath:  filepath.Join(oldRoot, "A Book.m4b"), StableKey: library.StableMediaKey(libraryID, "A Book.m4b"), SourceRelativePath: "A Book.m4b",
	})
	list, ok := source.store.CreateList("To read", "", nil)
	if !ok {
		t.Fatal("create list failed")
	}
	if _, ok := source.store.AddListItem(list.ID, "book-1", -1, []string{"weekend"}); !ok {
		t.Fatal("add list item failed")
	}

	resp, body := get(t, source.ts.URL+"/api/data/export")
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Disposition"), "sonder-data-") {
		t.Fatalf("export response = %d, disposition=%q", resp.StatusCode, resp.Header.Get("Content-Disposition"))
	}
	var bundle library.DataBundle
	if err := json.Unmarshal([]byte(body), &bundle); err != nil {
		t.Fatal(err)
	}

	destination := newFixture(t, func(cfg *config.Config) {
		cfg.Libraries = []config.Library{{ID: libraryID, Name: "Audiobooks", Path: newRoot, Kind: string(api.KindAudiobook)}}
	})
	payload := map[string]any{
		"format": bundle.Format, "version": bundle.Version, "exportedAt": bundle.ExportedAt,
		"libraries": bundle.Libraries, "snapshot": bundle.Snapshot, "mode": "replace",
		"pathMappings": map[string]string{oldRoot: newRoot},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	importResp, err := http.Post(destination.ts.URL+"/api/data/import", "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	importBody := readAll(t, importResp)
	if importResp.StatusCode != http.StatusOK {
		t.Fatalf("import response = %d: %s", importResp.StatusCode, importBody)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(importBody), &result); err != nil {
		t.Fatal(err)
	}
	if result["scanStarted"] != false {
		t.Fatalf("import unexpectedly started a scan: %v", result["scanStarted"])
	}
	item, ok := destination.store.Get("book-1")
	if !ok || item.FilePath != filepath.Join(newRoot, "A Book.m4b") {
		t.Fatalf("imported item = %+v, ok=%v", item, ok)
	}
	if got := destination.store.Lists()[0].ItemIDs; len(got) != 1 || got[0] != "book-1" {
		t.Fatalf("imported list = %v", got)
	}
}

func stringPointer(value string) *string { return &value }
