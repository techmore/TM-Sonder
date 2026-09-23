package library

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
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
	items := s.InternalItems()
	result := make([]api.MediaItem, 0, len(items))
	for _, item := range items {
		wire := item.MediaItem
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
	return result
}
