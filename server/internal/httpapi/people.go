package httpapi

import (
	"net/http"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/auth"
)

type sharedBookmark struct {
	ItemID    string        `json:"itemID"`
	Title     string        `json:"title"`
	Author    string        `json:"author,omitempty"`
	Kind      api.MediaKind `json:"kind"`
	PosterURL *string       `json:"posterURL,omitempty"`
	QueuedAt  *time.Time    `json:"queuedAt,omitempty"`
}

type sharedProgress struct {
	ItemID    string        `json:"itemID"`
	Title     string        `json:"title"`
	Author    string        `json:"author,omitempty"`
	Kind      api.MediaKind `json:"kind"`
	Seconds   float64       `json:"seconds"`
	Duration  float64       `json:"duration"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type connectedPerson struct {
	auth.Profile
	Bookmarks []sharedBookmark `json:"bookmarks,omitempty"`
	Progress  []sharedProgress `json:"progress,omitempty"`
}

type peopleResponse struct {
	Profile auth.Profile      `json:"profile"`
	People  []connectedPerson `json:"people"`
}

type sharingUpdate struct {
	ShareBookmarks *bool `json:"shareBookmarks"`
	ShareProgress  *bool `json:"shareProgress"`
}

func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	username := s.accountActivityUsername(r)
	if username == "" || !s.accountReady() {
		writeError(w, http.StatusUnauthorized, "Sign in to view connected accounts")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := s.ensureAccountActivity(username); err != nil {
		writeError(w, http.StatusInternalServerError, "Account activity is unavailable")
		return
	}
	profile, ok := s.accounts.Profile(username)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Account is unavailable")
		return
	}
	response := peopleResponse{Profile: profile, People: []connectedPerson{}}
	for _, connected := range s.accounts.ConnectedProfiles(username) {
		if err := s.ensureAccountActivity(connected.Username); err != nil {
			writeError(w, http.StatusInternalServerError, "Account activity is unavailable")
			return
		}
		person := connectedPerson{Profile: connected}
		activity := s.accountActivity.snapshot(connected.Username)
		if connected.ShareBookmarks {
			person.Bookmarks = make([]sharedBookmark, 0, len(activity.Reading.Queue))
			for _, itemID := range activity.Reading.Queue {
				item, found := s.store.Get(itemID)
				if !found || (item.Kind != api.KindAudiobook && item.Kind != api.KindEbook) {
					continue
				}
				var queuedAt *time.Time
				for _, record := range activity.Reading.Records {
					if record.ItemID == itemID {
						queuedAt = record.QueuedAt
						break
					}
				}
				author := ""
				if item.Author != nil {
					author = *item.Author
				} else if len(item.Tags) > 0 {
					author = item.Tags[0]
				}
				person.Bookmarks = append(person.Bookmarks, sharedBookmark{
					ItemID: item.ID, Title: item.Title, Author: author, Kind: item.Kind,
					PosterURL: item.PosterURL, QueuedAt: queuedAt,
				})
			}
		}
		if connected.ShareProgress {
			person.Progress = make([]sharedProgress, 0, len(activity.Progress))
			for _, record := range activity.Progress {
				item, found := s.store.Get(record.ItemID)
				if !found {
					continue
				}
				author := ""
				if item.Author != nil {
					author = *item.Author
				} else if len(item.Tags) > 0 && (item.Kind == api.KindAudiobook || item.Kind == api.KindEbook) {
					author = item.Tags[0]
				}
				person.Progress = append(person.Progress, sharedProgress{
					ItemID: item.ID, Title: item.Title, Author: author, Kind: item.Kind,
					Seconds: record.Seconds, Duration: record.Duration, UpdatedAt: record.UpdatedAt,
				})
			}
		}
		response.People = append(response.People, person)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handlePeopleSharing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	username := s.accountActivityUsername(r)
	if username == "" || !s.accountReady() {
		writeError(w, http.StatusUnauthorized, "Sign in to update sharing")
		return
	}
	var update sharingUpdate
	if err := jsonDecode(w, r, &update); err != nil || (update.ShareBookmarks == nil && update.ShareProgress == nil) {
		writeError(w, http.StatusBadRequest, "Choose at least one sharing setting")
		return
	}
	profile, ok, err := s.accounts.UpdateSharing(username, update.ShareBookmarks, update.ShareProgress)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save sharing settings")
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "Account is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, profile)
}
