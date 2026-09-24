package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
)

const (
	jellyfinUserID    = "sonder-user"
	jellyfinLibraryID = "jf_audiobooks"
)

func (s *Server) handleJellyfinPublicInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"LocalAddress": "", "ServerName": AppName, "Version": Version,
		"ProductName": AppName, "Id": ServerID, "StartupWizardCompleted": true,
	})
}

func (s *Server) handleJellyfinQuickConnectEnabled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, false)
}

func (s *Server) handleJellyfinAuthenticate(w http.ResponseWriter, r *http.Request) {
	var login struct {
		Username string `json:"Username"`
		Password string `json:"Pw"`
	}
	if err := jsonDecode(w, r, &login); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid authentication request")
		return
	}
	token, _, err := s.issueAccountSession(login.Username, login.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"AccessToken": token,
		"ServerId":    ServerID,
		"User": map[string]any{
			"Id": jellyfinUserID, "Name": nonEmpty(login.Username, "sonder"), "ServerId": ServerID,
			"Policy": map[string]any{
				"AuthenticationProviderId": "TM Sonder",
				"PasswordResetProviderId":  "TM Sonder",
				"IsAdministrator":          true, "EnableAllFolders": true,
				"EnableMediaPlayback": true, "EnableContentDownloading": true,
				"EnableRemoteAccess": true, "EnableAudioPlaybackTranscoding": true,
			},
		},
	})
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (s *Server) handleJellyfinViews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"Items":            []map[string]any{jellyfinContainer(jellyfinLibraryID, "Sonder Audiobooks")},
		"TotalRecordCount": 1,
	})
}

func jellyfinContainer(id, name string) map[string]any {
	return map[string]any{
		"Name": name, "ServerId": ServerID, "Id": id, "Type": "CollectionFolder",
		"IsFolder": true, "CollectionType": "books", "SortName": name,
	}
}

func (s *Server) handleJellyfinItems(w http.ResponseWriter, r *http.Request) {
	items := s.store.InternalItemsOfKind(api.KindAudiobook)
	artistFilter := strings.TrimSpace(r.URL.Query().Get("AlbumArtistIds"))
	if artistFilter == "" {
		artistFilter = strings.TrimSpace(r.URL.Query().Get("albumArtistIds"))
	}
	if artistFilter != "" {
		wanted := map[string]bool{}
		for _, id := range strings.Split(artistFilter, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
		filtered := items[:0]
		for _, item := range items {
			if wanted[jellyfinAuthor(item)] {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	q := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("SearchTerm")))
	if q == "" {
		q = strings.TrimSpace(strings.ToLower(r.URL.Query().Get("searchTerm")))
	}
	if q != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Title), q) || strings.Contains(strings.ToLower(jellyfinAuthor(item)), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
	if start < 0 {
		start = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	total := len(items)
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	result := make([]map[string]any, 0, end-start)
	for _, item := range items[start:end] {
		result = append(result, s.jellyfinItem(item, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"Items": result, "TotalRecordCount": total, "StartIndex": start,
	})
}

func (s *Server) handleJellyfinAlbumArtists(w http.ResponseWriter, r *http.Request) {
	items := s.store.InternalItemsOfKind(api.KindAudiobook)
	seen := map[string]bool{}
	artists := make([]map[string]any, 0)
	for _, item := range items {
		name := jellyfinAuthor(item)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		artists = append(artists, map[string]any{
			"Name": name, "SortName": name, "Id": name, "Type": "MusicArtist", "IsFolder": true,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"Items": artists, "TotalRecordCount": len(artists)})
}

func jellyfinAuthor(item *library.Item) string {
	if author := strings.TrimSpace(derefStr(item.Author)); author != "" {
		return author
	}
	// The current catalog often has the author encoded by the conventional
	// Audiobooks/<Author>/<Book>/<file> folder layout even when metadata
	// enrichment has not populated MediaItem.Author yet.
	bookDir := filepath.Dir(item.FilePath)
	author := strings.TrimSpace(filepath.Base(filepath.Dir(bookDir)))
	if author == "" || author == "." || author == string(filepath.Separator) {
		return ""
	}
	return author
}

func (s *Server) handleJellyfinPersons(w http.ResponseWriter, r *http.Request) {
	// BookPlayer derives narrator entries from audiobook People metadata. Return
	// an empty valid Jellyfin result for clients that probe /Persons directly.
	writeJSON(w, http.StatusOK, map[string]any{"Items": []any{}, "TotalRecordCount": 0})
}

func jellyfinItemID(id string) string { return strings.TrimPrefix(id, "jf_") }

func (s *Server) findJellyfinItem(id string) (*library.Item, bool) {
	item, ok := s.store.Get(jellyfinItemID(id))
	return item, ok && item.Kind == api.KindAudiobook
}

func (s *Server) handleJellyfinItem(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findJellyfinItem(r.PathValue("itemID"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	writeJSON(w, http.StatusOK, s.jellyfinItem(item, true))
}

func (s *Server) jellyfinItem(item *library.Item, includeChapters bool) map[string]any {
	id := "jf_" + item.ID
	filename := filepath.Base(item.FilePath)
	size := item.SizeBytes
	chapters := []map[string]any{}
	if includeChapters {
		for _, ch := range s.audiobookshelfChapters(item.ID, item.DurationSeconds) {
			chapters = append(chapters, map[string]any{
				"StartPositionTicks": int64(ch["start"].(float64) * 10_000_000),
				"Name":               ch["title"], "Index": ch["index"],
			})
		}
	}
	author := jellyfinAuthor(item)
	metadata := map[string]any{
		"Name": item.Title, "SortName": item.Title, "Type": "AudioBook", "Id": id,
		"Album": item.Title, "AlbumArtist": author, "Artists": []string{author},
		"Overview": item.Summary, "Genres": item.Genres, "Tags": item.Tags,
		"ProductionYear": item.Year, "RunTimeTicks": int64(item.DurationSeconds * 10_000_000),
		"Path": item.FilePath, "Container": strings.TrimPrefix(filepath.Ext(filename), "."),
		"MediaSources": []map[string]any{{"Id": id, "Path": item.FilePath, "Size": size, "Container": strings.TrimPrefix(filepath.Ext(filename), ".")}},
		"Chapters":     chapters, "ImageTags": map[string]string{}, "IsFolder": false,
	}
	if author != "" {
		metadata["AlbumArtists"] = []map[string]any{{"Name": author, "Id": author, "Type": "MusicArtist"}}
	}
	if item.PosterPath != "" {
		metadata["ImageTags"] = map[string]string{"Primary": "sonder"}
	}
	return metadata
}

func (s *Server) handleJellyfinImage(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findJellyfinItem(r.PathValue("itemID"))
	if !ok {
		writeError(w, http.StatusNotFound, "Item not found")
		return
	}
	s.serveArtwork(w, r, item.PosterPath)
}

func (s *Server) handleJellyfinDownload(w http.ResponseWriter, r *http.Request) {
	item, ok := s.findJellyfinItem(r.PathValue("itemID"))
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
