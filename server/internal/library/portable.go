package library

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

const (
	DataBundleFormat  = "tm-sonder-data"
	DataBundleVersion = 1
)

// DataBundle is the portable, media-independent Sonder backup format. It
// contains catalog and user data, but never copies the media files themselves.
type DataBundle struct {
	Format     string           `json:"format"`
	Version    int              `json:"version"`
	ExportedAt time.Time        `json:"exportedAt"`
	Libraries  []config.Library `json:"libraries,omitempty"`
	Snapshot   Snapshot         `json:"snapshot"`
}

type DataImportResult struct {
	Mode          string `json:"mode"`
	Items         int    `json:"items"`
	Lists         int    `json:"lists"`
	Progress      int    `json:"progress"`
	PathsRemapped int    `json:"pathsRemapped"`
}

// ExportBundle returns a deep copy suitable for JSON encoding or backup.
func (s *Store) ExportBundle(libs []config.Library) DataBundle {
	return DataBundle{
		Format:     DataBundleFormat,
		Version:    DataBundleVersion,
		ExportedAt: time.Now().UTC(),
		Libraries:  append([]config.Library(nil), libs...),
		Snapshot:   s.Snapshot(),
	}
}

// RemapBundlePaths updates imported media paths to the current machine or
// container mount. mappings is keyed by exported library root, for example
// {"/Users/sean/NAS": "/media"}. When no explicit mapping is supplied, a
// library with the same stable ID supplies the target root.
func RemapBundlePaths(bundle *DataBundle, target []config.Library, mappings map[string]string) (int, error) {
	if bundle == nil {
		return 0, fmt.Errorf("nil data bundle")
	}
	if bundle.Format != DataBundleFormat {
		return 0, fmt.Errorf("unsupported bundle format %q", bundle.Format)
	}
	if bundle.Version > DataBundleVersion {
		return 0, fmt.Errorf("bundle version %d is newer than supported version %d", bundle.Version, DataBundleVersion)
	}
	if err := migrateSnapshot(&bundle.Snapshot); err != nil {
		return 0, err
	}

	sourceByID := make(map[string]config.Library, len(bundle.Libraries))
	for _, lib := range bundle.Libraries {
		sourceByID[lib.ID] = lib
	}
	targetByID := make(map[string]config.Library, len(target))
	for _, lib := range target {
		targetByID[lib.ID] = lib
	}

	remapped := 0
	for _, item := range bundle.Snapshot.Items {
		if item == nil {
			continue
		}
		libraryID := ""
		if item.LibraryID != nil {
			libraryID = *item.LibraryID
		}
		sourceRoot := ""
		if source, ok := sourceByID[libraryID]; ok {
			sourceRoot = source.Path
		}
		targetRoot := mappedRoot(sourceRoot, mappings)
		if targetRoot == "" {
			if current, ok := targetByID[libraryID]; ok {
				targetRoot = current.Path
			}
		}
		relative := item.SourceRelativePath
		if relative == "" && sourceRoot != "" {
			if rel, ok := RelativeMediaPath(sourceRoot, item.FilePath); ok {
				relative = rel
			}
		}
		if targetRoot != "" && relative != "" {
			if rel, ok := RelativeMediaPath(".", filepath.FromSlash(relative)); ok {
				item.FilePath = filepath.Join(targetRoot, filepath.FromSlash(rel))
				item.SourceRelativePath = rel
				item.StableKey = StableMediaKey(libraryID, rel)
				remapped++
			}
		} else if mapped := mappedPath(item.FilePath, mappings); mapped != item.FilePath {
			item.FilePath = mapped
			remapped++
		}
		item.PosterPath = mappedPath(item.PosterPath, mappings)
		item.BackdropPath = mappedPath(item.BackdropPath, mappings)
	}
	return remapped, nil
}

func mappedRoot(root string, mappings map[string]string) string {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return ""
	}
	if mapped := strings.TrimSpace(mappings[root]); mapped != "" {
		return filepath.Clean(mapped)
	}
	return ""
}

func mappedPath(path string, mappings map[string]string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	for from, to := range mappings {
		from, to = filepath.Clean(strings.TrimSpace(from)), filepath.Clean(strings.TrimSpace(to))
		if from == "" || to == "" || from == "." || to == "." {
			continue
		}
		if rel, ok := RelativeMediaPath(from, clean); ok {
			return filepath.Join(to, filepath.FromSlash(rel))
		}
	}
	return path
}

// ImportBundle replaces or merges the in-memory catalog without touching the
// filesystem. A later explicit scan can reconcile new or missing media.
func (s *Store) ImportBundle(bundle DataBundle, mode string) (DataImportResult, error) {
	if bundle.Format != DataBundleFormat {
		return DataImportResult{}, fmt.Errorf("unsupported bundle format %q", bundle.Format)
	}
	if bundle.Version > DataBundleVersion {
		return DataImportResult{}, fmt.Errorf("bundle version %d is newer than supported version %d", bundle.Version, DataBundleVersion)
	}
	if err := migrateSnapshot(&bundle.Snapshot); err != nil {
		return DataImportResult{}, err
	}
	if mode == "" {
		mode = "replace"
	}
	if mode != "replace" && mode != "merge" {
		return DataImportResult{}, fmt.Errorf("unsupported import mode %q", mode)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == "replace" {
		s.items = make(map[string]*Item, len(bundle.Snapshot.Items))
		for _, item := range bundle.Snapshot.Items {
			if item != nil && item.ID != "" {
				s.items[item.ID] = item.clone()
			}
		}
		progress := make(map[string]*api.ProgressRecord, len(bundle.Snapshot.Progress))
		for i := range bundle.Snapshot.Progress {
			record := bundle.Snapshot.Progress[i]
			if record.ItemID != "" {
				progress[record.ItemID] = &record
			}
		}
		s.progress = progress
		s.directories = append([]api.MediaDirectory(nil), bundle.Snapshot.Directories...)
		s.activity = append([]api.ActivityEvent(nil), bundle.Snapshot.Activity...)
		s.lists = cloneLists(bundle.Snapshot.Lists)
	} else {
		for _, item := range bundle.Snapshot.Items {
			if item != nil && item.ID != "" {
				s.items[item.ID] = item.clone()
			}
		}
		if s.progress == nil {
			s.progress = make(map[string]*api.ProgressRecord)
		}
		for i := range bundle.Snapshot.Progress {
			record := bundle.Snapshot.Progress[i]
			if record.ItemID == "" {
				continue
			}
			if previous, ok := s.progress[record.ItemID]; !ok || record.UpdatedAt.After(previous.UpdatedAt) {
				s.progress[record.ItemID] = &record
			}
		}
		byID := make(map[string]int, len(s.lists))
		for i := range s.lists {
			byID[s.lists[i].ID] = i
		}
		for _, list := range bundle.Snapshot.Lists {
			if i, ok := byID[list.ID]; ok {
				s.lists[i] = cloneBookList(list)
			} else {
				s.lists = append(s.lists, cloneBookList(list))
			}
		}
	}
	s.gen = max64(s.gen+1, bundle.Snapshot.Generation+1)
	return DataImportResult{
		Mode:     mode,
		Items:    len(s.items),
		Lists:    len(s.lists),
		Progress: len(s.progress),
	}, nil
}

func cloneLists(in []BookList) []BookList {
	out := make([]BookList, len(in))
	for i, list := range in {
		out[i] = cloneBookList(list)
	}
	return out
}
