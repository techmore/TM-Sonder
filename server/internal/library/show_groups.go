package library

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

type browsingRoot struct {
	path     string
	realPath string
}

// GroupedItems adds browsing identity without overwriting parsed identity.
// A TV library follows Show/Season/Files; an audiobook follows
// Author/Book/Files, where one book may be many files. Flat files retain
// legacy grouping. Opaque IDs never reveal absolute filesystem paths to
// clients.
func (s *Store) GroupedItems(libraries []config.Library) []api.MediaItem {
	roots := map[string]browsingRoot{}
	for _, lib := range libraries {
		if lib.Kind == "tvShow" {
			root := browsingRoot{path: filepath.Clean(lib.Path), realPath: filepath.Clean(lib.Path)}
			// Plex imports can retain the resolved NAS mount while the active
			// library points at a symlink (for example /Users/.../NAS ->
			// /Volumes/14tb). Resolve the root once so the API can still derive
			// the show folder without touching every media file on each request.
			if resolved, err := filepath.EvalSymlinks(root.path); err == nil {
				root.realPath = filepath.Clean(resolved)
			}
			roots[lib.ID] = root
		}
	}
	s.mu.RLock()
	internal := make([]*Item, 0, len(s.items))
	for _, item := range s.items {
		internal = append(internal, item)
	}
	s.mu.RUnlock()

	// Book grouping needs every audiobook at once to order the parts, so it is
	// derived from the snapshot taken above rather than inside the loop.
	books := BookGroupings(internal, libraries)

	result := make([]api.MediaItem, 0, len(internal))
	for _, item := range internal {
		wire := item.MediaItem
		wire.CoverEmbedded = item.ProbedHasCover
		wire.CoverAvailable = item.PosterPath != "" && item.PosterSource != "thumbnail"
		if item.Kind == api.KindMovie && wire.PosterURL == nil {
			u := "/artwork/poster/" + wire.ID
			wire.PosterURL = &u
		}
		if item.PosterSource == "thumbnail" && (item.Kind == api.KindTVShow || item.Kind == api.KindDocumentary) {
			wire.PosterURL = nil
		}
		if item.Kind == api.KindTVShow && item.LibraryID != nil {
			if root, ok := roots[*item.LibraryID]; ok {
				if title, ok := showFolderTitle(item, root); ok {
					id := fmt.Sprintf("tv-%x", sha256.Sum256([]byte(*item.LibraryID+"\x00"+title)))
					wire.ShowGroupID, wire.ShowGroupTitle = &id, &title
				}
			}
		}
		if g, ok := books[item.ID]; ok {
			id, title := g.ID, g.Title
			wire.BookGroupID, wire.BookGroupTitle = &id, &title
			wire.BookPartIndex, wire.BookPartCount = g.Index, g.Count
		}
		// Ordering metadata is derived for every kind, not just books: the same
		// folder-derived names appear in the movie and TV listings.
		if parts := SplitTitle(item.Title); parts.Sort != "" {
			wire.SortTitle = parts.Sort
			// Stored values win: once the series marker is stripped from the
			// title, re-deriving it from that title would lose it.
			if item.SeriesPosition != "" {
				wire.SeriesPosition = item.SeriesPosition
			} else {
				wire.SeriesPosition = parts.SeriesPosition
			}
			if item.Series != "" {
				wire.SeriesName = item.Series
			} else {
				wire.SeriesName = parts.Series
			}
			wire.PublicationYear = parts.Year
		}
		result = append(result, wire)
	}
	// Store updates are copy-on-write, so the wire values and their nested
	// slices remain stable after the read lock is released. Sorting outside the
	// lock keeps catalog assembly from blocking individual artwork requests.
	sort.Slice(result, func(a, b int) bool {
		if result[a].Title != result[b].Title {
			return result[a].Title < result[b].Title
		}
		return result[a].ID < result[b].ID
	})
	return result
}

// showFolderTitle returns the first directory in a TV item's library-relative
// path. SourceRelativePath is preferred because it survives Plex imports and
// volume/symlink remaps; the filesystem path remains a fallback for older
// snapshots and freshly scanned items.
func showFolderTitle(item *Item, root browsingRoot) (string, bool) {
	relative := ""
	if item.SourceRelativePath != "" {
		relative = filepath.Clean(filepath.FromSlash(item.SourceRelativePath))
		if filepath.IsAbs(relative) || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			relative = ""
		}
	}
	if relative == "" && item.FilePath != "" {
		for _, base := range []string{root.path, root.realPath} {
			if base == "" {
				continue
			}
			candidate, err := filepath.Rel(base, item.FilePath)
			if err == nil && candidate != "." && candidate != ".." && !strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
				relative = candidate
				break
			}
		}
	}
	if relative == "" {
		return "", false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
		return "", false
	}
	return parts[0], true
}
