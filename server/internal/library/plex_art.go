package library

import (
	"os"
	"path/filepath"
	"strings"

	"tm-sonder/server/internal/api"
)

// Plex local-assets naming (https://support.plex.tv local media assets):
//
//	Movies & singles:  "Film (2010)/poster.jpg"      (dedicated folder)
//	                   "Film (2010)-poster.jpg"      (beside file, shared folder)
//	TV:                "Show Name/poster.jpg"        (show level)
//	                   "Show Name/Season 1/Season1-poster.jpg" (season)
//	Fanart equivalents use "fanart(.jpg)" / "-fanart.jpg".
//
// PlexArtPaths returns where this item's poster and fanart must be written.
// Empty strings mean "no sensible Plex location" (never expected).
func PlexArtPaths(item *Item) (posterDest, fanartDest string) {
	if item.FilePath == "" {
		return "", ""
	}
	dir := filepath.Dir(item.FilePath)
	base := strings.TrimSuffix(filepath.Base(item.FilePath), filepath.Ext(item.FilePath))
	if base == "" || base == "." {
		return "", ""
	}

	if item.Kind == api.KindTVShow {
		showDir := dir
		dirBase := strings.ToLower(filepath.Base(dir))
		if strings.Contains(dirBase, "season") || strings.Contains(dirBase, "specials") {
			showDir = filepath.Dir(dir)
		}
		return filepath.Join(showDir, "poster.jpg"), filepath.Join(showDir, "fanart.jpg")
	}

	if countMediaFiles(dir) <= 1 {
		// Dedicated folder: classic "Folder/poster.jpg" layout.
		return filepath.Join(dir, "poster.jpg"), filepath.Join(dir, "fanart.jpg")
	}
	// Shared folder: sidecar naming keyed to the file.
	return filepath.Join(dir, base+"-poster.jpg"), filepath.Join(dir, base+"-fanart.jpg")
}

// countMediaFiles counts regular files in dir recognized as catalog media.
func countMediaFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.Name()), "."))
		if api.FormatForExtension(ext) != api.FormatUnknown {
			n++
		}
	}
	return n
}

// CopyArtTo installs src at dest unless dest already exists (local user art
// is never overwritten). Returns the effective destination on success.
func CopyArtTo(src, dest string) (string, error) {
	if src == "" || dest == "" {
		return "", os.ErrInvalid
	}
	if _, err := os.Stat(dest); err == nil {
		return dest, nil // pre-existing art wins
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}
