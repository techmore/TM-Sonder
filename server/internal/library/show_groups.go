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

// GroupedItems adds browsing identity without overwriting parsed identity.
// A TV library follows Show/Season/Files. Flat files retain legacy grouping.
// Opaque IDs never reveal absolute filesystem paths to clients.
func (s *Store) GroupedItems(libraries []config.Library) []api.MediaItem {
	roots := map[string]string{}
	for _, lib := range libraries {
		if lib.Kind == "tvShow" {
			roots[lib.ID] = lib.Path
		}
	}
	s.mu.RLock()
	result := make([]api.MediaItem, 0, len(s.items))
	for _, item := range s.items {
		wire := item.MediaItem
		wire.CoverEmbedded = item.ProbedHasCover
		wire.CoverAvailable = item.PosterPath != "" && item.PosterSource != "thumbnail"
		if item.PosterSource == "thumbnail" && (item.Kind == api.KindMovie || item.Kind == api.KindTVShow || item.Kind == api.KindDocumentary) {
			wire.PosterURL = nil
		}
		if item.Kind == api.KindTVShow && item.LibraryID != nil {
			if root := roots[*item.LibraryID]; root != "" {
				relative, err := filepath.Rel(root, item.FilePath)
				parts := strings.Split(relative, string(filepath.Separator))
				if err == nil && len(parts) >= 2 && parts[0] != ".." && parts[0] != "." {
					title := parts[0]
					id := fmt.Sprintf("tv-%x", sha256.Sum256([]byte(*item.LibraryID+"\x00"+title)))
					wire.ShowGroupID, wire.ShowGroupTitle = &id, &title
				}
			}
		}
		result = append(result, wire)
	}
	s.mu.RUnlock()
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
