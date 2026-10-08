package httpapi

import (
	"net/http"
	"sort"

	"tm-sonder/server/internal/api"
)

// Movie shelf visibility belongs to the signed-in account, not its device.
func (s *Server) handleMovieContinue(w http.ResponseWriter, r *http.Request) {
	username := s.accountActivityUsername(r)
	if username == "" && s.accounts != nil {
		username = s.accounts.Username() // Trusted local and paired-owner clients.
	}
	if err := s.ensureAccountActivity(username); err != nil {
		writeError(w, http.StatusInternalServerError, "Movie shelf preferences unavailable")
		return
	}
	if r.Method == http.MethodPatch {
		id := r.PathValue("id")
		item, ok := s.store.Get(id)
		if !ok || item.Kind != api.KindMovie {
			writeError(w, http.StatusNotFound, "Movie not found")
			return
		}
		var body struct {
			Dismissed *bool `json:"dismissed"`
		}
		if jsonDecode(w, r, &body) != nil || body.Dismissed == nil {
			writeError(w, http.StatusBadRequest, "Specify dismissed as true or false")
			return
		}
		if err := s.accountActivity.update(username, func(activity *accountActivity) error {
			if *body.Dismissed {
				activity.DismissedMovies[id] = true
			} else {
				delete(activity.DismissedMovies, id)
			}
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not save movie shelf preference")
			return
		}
	}
	ids := []string{}
	for id, dismissed := range s.accountActivity.snapshot(username).DismissedMovies {
		if dismissed {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"dismissedMovieIDs": ids})
}
