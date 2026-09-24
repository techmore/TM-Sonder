package httpapi

import (
	"context"
	"net/http"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/enrich"
)

// handleMovieMetadata keeps the base library payload small. The browser asks
// for this only when a movie becomes the selected title in the Rails detail
// panel, and the enricher caches the result under the server data directory.
func (s *Server) handleMovieMetadata(w http.ResponseWriter, r *http.Request) {
	item, ok := s.store.Get(r.PathValue("id"))
	if !ok || item.Kind != api.KindMovie {
		writeError(w, http.StatusNotFound, "Movie not found")
		return
	}
	if s.movieMetadata == nil {
		writeError(w, http.StatusServiceUnavailable, "Movie metadata is unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	result, err := s.movieMetadata.MovieMetadata(ctx, enrich.Input{
		Title:            item.Title,
		Kind:             string(item.Kind),
		Year:             item.Year,
		Studio:           item.Studio,
		Edition:          derefStr(item.Edition),
		Summary:          item.Summary,
		MetadataIDSource: derefStr(item.MetadataIDSource),
		MetadataID:       derefStr(item.MetadataID),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "Movie metadata provider unavailable")
		return
	}
	if result == nil {
		result = &enrich.MovieMetadata{Primer: item.Summary, Genres: item.Genres, Provider: "local catalog"}
	} else if result.Primer == "" {
		result.Primer = item.Summary
	}
	if len(result.Genres) == 0 && len(item.Genres) > 0 {
		result.Genres = append([]string(nil), item.Genres...)
	}
	writeJSON(w, http.StatusOK, result)
}
