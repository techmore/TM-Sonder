package httpapi

// This file exposes the small Audiobookshelf-compatible surface BookPlayer
// uses: ping, login, library discovery, covers, and file downloads.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
)

const (
	audiobookshelfLibraryID = "lib_sonder_audiobooks"
	audiobookshelfUserID    = "sonder-user"
)

func (s *Server) handleAudiobookshelfPing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) handleAudiobookshelfStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"serverVersion": Version,
		"authMethods":   []string{"local"},
		"authFormData":  map[string]any{},
	})
}

func (s *Server) handleAudiobookshelfLogin(w http.ResponseWriter, r *http.Request) {
	var login struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if r.ContentLength != 0 {
		if err := jsonDecode(w, r, &login); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid login request")
			return
		}
	}
	token, _, err := s.issueAccountSession(login.Username, login.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	now := time.Now().UnixMilli()
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id": audiobookshelfUserID, "username": "sonder", "type": "root",
			"token": token, "isActive": true, "isLocked": false,
			"permissions": map[string]bool{"download": true, "accessAllLibraries": true, "accessAllTags": true},
		},
		"userDefaultLibraryId": audiobookshelfLibraryID,
		"serverSettings":       map[string]any{"version": Version, "language": "en-us"},
		"Source":               "TM Sonder", "createdAt": now,
	})
}

func (s *Server) handleAudiobookshelfLibraries(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UnixMilli()
	writeJSON(w, http.StatusOK, map[string]any{"libraries": []map[string]any{{
		"id": audiobookshelfLibraryID, "name": "Sonder Audiobooks", "folders": []any{},
		"displayOrder": 1, "icon": "audiobookshelf", "mediaType": "book", "provider": "sonder",
		"settings": map[string]any{"coverAspectRatio": 1}, "createdAt": now, "lastUpdate": now,
	}}})
}

func (s *Server) handleAudiobookshelfItems(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("libraryID") != audiobookshelfLibraryID {
		writeError(w, http.StatusNotFound, "Library not found")
		return
	}
	items := s.store.InternalItemsOfKind(api.KindAudiobook)
	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	if query != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Title), query) ||
				strings.Contains(strings.ToLower(derefStr(item.Author)), query) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	total := len(items)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 0 {
		page = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	start := page * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pageItems := items[start:end]
	results := make([]map[string]any, 0, len(pageItems))
	for _, item := range pageItems {
		// Keep the catalog request fast. BookPlayer fetches the detail route
		// when it needs chapter atoms for an individual book.
		results = append(results, s.audiobookshelfItem(item, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results, "total": total, "limit": limit, "page": page,
		"sortBy": "media.metadata.title", "sortDesc": false, "mediaType": "book", "minified": false,
		"collapseseries": false, "include": "",
	})
}

func sonderItemID(id string) string { return strings.TrimPrefix(id, "li_") }

func (s *Server) findAudiobookshelfItem(id string) (*library.Item, bool) {
	return s.store.Get(sonderItemID(id))
}

func (s *Server) handleAudiobookshelfItem(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findAudiobookshelfItem(r.PathValue("id"))
	if !ok || item.Kind != api.KindAudiobook {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.audiobookshelfItem(item, true))
}

func (s *Server) handleAudiobookshelfCover(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findAudiobookshelfItem(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	s.serveArtwork(w, r, item.PosterPath)
}

func (s *Server) handleAudiobookshelfDownload(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findAudiobookshelfItem(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	st, err := os.Stat(item.FilePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "Media file missing")
		return
	}
	f, err := os.Open(item.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Cannot open media")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", item.Format.ContentType())
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(item.FilePath), st.ModTime(), f)
}

func (s *Server) audiobookshelfItem(item *library.Item, includeChapters bool) map[string]any {
	remoteID := "li_" + item.ID
	ino := "file_" + item.ID
	filename := filepath.Base(item.FilePath)
	size := item.SizeBytes
	if st, err := os.Stat(item.FilePath); err == nil {
		size = st.Size()
	}
	metadata := map[string]any{
		"title": item.Title, "subtitle": item.Subtitle, "authorName": derefStr(item.Author),
		"narratorName": derefStr(item.Narrator), "seriesName": item.Series, "genres": item.Genres,
		"publisher": item.Studio, "description": item.Summary, "publishedYear": item.Year,
	}
	chapters := []map[string]any{}
	if includeChapters {
		chapters = s.audiobookshelfChapters(item.ID, item.DurationSeconds)
	}
	audioFile := map[string]any{
		"index": 1, "ino": ino, "metadata": map[string]any{
			"filename": filename, "ext": filepath.Ext(filename),
			"path": filename, "relPath": filename, "size": size,
		}, "duration": item.DurationSeconds, "codec": string(item.Format),
		"chapters": chapters, "mimeType": item.Format.ContentType(),
	}
	return map[string]any{
		"id": remoteID, "ino": ino, "libraryId": audiobookshelfLibraryID, "folderId": "sonder",
		"path": filename, "relPath": filename, "isFile": true, "isMissing": false, "isInvalid": false,
		"mediaType": "book", "addedAt": item.ModTime.UnixMilli(), "updatedAt": item.ModTime.UnixMilli(),
		"size": size, "media": map[string]any{
			"libraryItemId": remoteID, "metadata": metadata, "coverPath": nil,
			"tags": item.Tags, "numTracks": 1, "numAudioFiles": 1,
			"numChapters": len(chapters), "duration": item.DurationSeconds,
			"size": size, "audioFiles": []any{audioFile},
		},
	}
}

func (s *Server) audiobookshelfChapters(itemID string, duration float64) []map[string]any {
	if s.chapters == nil {
		return []map[string]any{}
	}
	chapters, err := s.chapters.ChaptersFor(context.Background(), itemID)
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(chapters))
	for _, ch := range chapters {
		end := any(duration)
		if ch.EndSeconds != nil {
			end = *ch.EndSeconds
		}
		out = append(out, map[string]any{"id": ch.Index, "index": ch.Index, "start": ch.StartSeconds, "end": end, "title": ch.Title})
	}
	return out
}
