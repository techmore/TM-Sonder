package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

type curatedPoster struct {
	Filename   string `json:"filename"`
	SourcePage string `json:"sourcePage"`
	License    string `json:"license"`
	Note       string `json:"note"`
}

var curatedFilename = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|jpeg)$`)

func (s *Server) curatedPosters() map[string]curatedPoster {
	data, err := os.ReadFile(filepath.Join(s.cfg().DataDir, "curated-posters", "manifest.json"))
	if err != nil {
		return nil
	}
	var posters map[string]curatedPoster
	if json.Unmarshal(data, &posters) != nil {
		return nil
	}
	for id, p := range posters {
		if !curatedFilename.MatchString(p.Filename) {
			delete(posters, id)
			continue
		}
		st, err := os.Stat(filepath.Join(s.cfg().DataDir, "curated-posters", p.Filename))
		if err != nil || !st.Mode().IsRegular() {
			delete(posters, id)
		}
	}
	return posters
}

func (s *Server) handleCuratedPoster(w http.ResponseWriter, r *http.Request) {
	poster, ok := s.curatedPosters()[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "Artwork not found")
		return
	}
	s.serveArtwork(w, r, filepath.Join(s.cfg().DataDir, "curated-posters", poster.Filename))
}
