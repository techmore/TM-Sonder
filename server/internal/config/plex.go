package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// plexKindByFolder maps the standard Plex library folder names to Sonder
// library kinds. Matching is case-insensitive and tolerant of plural forms
// ("Movie", "Movies", "TV Shows", "Audiobooks", ...).
var plexKindByFolder = map[string]string{
	"movies":        "movie",
	"movie":         "movie",
	"films":         "movie",
	"film":          "movie",
	"tv shows":      "tvShow",
	"tv show":       "tvShow",
	"tvshows":       "tvShow",
	"tv":            "tvShow",
	"shows":         "tvShow",
	"show":          "tvShow",
	"documentary":   "documentary",
	"documentaries": "documentary",
	"audiobooks":    "audiobook",
	"audiobook":     "audiobook",
	"ebooks":        "ebook",
	"ebook":         "ebook",
	"books":         "ebook",
	"book":          "ebook",
}

// normalizePlexFolder canonicalizes a child-directory name for lookup.
func normalizePlexFolder(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimSuffix(n, "s")
	return strings.TrimSpace(n)
}

// ExpandPlexLibraries resolves any library whose kind is "plex" into one
// child library per recognized Plex-standard subfolder (Movies, TV Shows,
// Audiobooks, Ebooks, ...). The root itself is never scanned. Unrecognized
// children are ignored so stray folders (metadata caches, extras) don't get
// misclassified. A "plex" entry whose directory doesn't exist yields an
// error, matching sanitizeLibraries behavior for ordinary libraries.
//
// IDs are deterministic (stable across restarts): "plex-<kind>-<basename>",
// lower-cased and slugified, so rescans keep item/library identity stable.
func ExpandPlexLibraries(libs []Library) ([]Library, error) {
	out := make([]Library, 0, len(libs))
	for i, lib := range libs {
		if lib.Kind != "plex" {
			out = append(out, lib)
			continue
		}
		path := filepath.Clean(strings.TrimSpace(lib.Path))
		if path == "" || path == "/" {
			return nil, fmt.Errorf("libraries[%d] (%q): plex root path required", i, lib.Name)
		}
		st, err := os.Stat(path)
		if err != nil || !st.IsDir() {
			return nil, fmt.Errorf("libraries[%d] (%q): plex root is not a readable directory", i, lib.Name)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("libraries[%d] (%q): cannot read plex root: %w", i, lib.Name, err)
		}
		// Deterministic ordering regardless of filesystem order.
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			// Stat (not DirEntry.IsDir) so symlinked library folders
			// (e.g. NAS mounts linked into the plex root) count as dirs.
			if st, err := os.Stat(filepath.Join(path, name)); err == nil && st.IsDir() {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		found := 0
		for _, name := range names {
			kind, ok := plexKindByFolder[normalizePlexFolder(name)]
			if !ok || kind == "plex" {
				continue
			}
			slug := strings.ToLower(strings.Join(strings.Fields(
				strings.NewReplacer(".", "", "_", " ", "-", " ").Replace(filepath.Base(path))), "-"))
			out = append(out, Library{
				ID:   fmt.Sprintf("plex-%s-%s", kind, slug),
				Name: name,
				Path: filepath.Join(path, name),
				Kind: kind,
			})
			found++
		}
		if found == 0 {
			return nil, fmt.Errorf("libraries[%d] (%q): no Plex-standard folders (Movies, TV Shows, Audiobooks, Ebooks) found in %s",
				i, lib.Name, path)
		}
	}
	return out, nil
}
