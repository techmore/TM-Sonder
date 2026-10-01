package httpapi

import (
	"html/template"
	"net/http"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/auth"
)

var sharedQueueTemplate = template.Must(template.New("shared-queue").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><meta name="robots" content="noindex,nofollow"><title>Reading queue · TM Sonder</title><style>body{margin:0;background:#f1eee6;color:#27281f;font:16px/1.55 system-ui,sans-serif}.wrap{max-width:760px;margin:48px auto;padding:24px}header{border-bottom:1px solid #c8c3b6;padding-bottom:16px}h1{font:600 clamp(2rem,7vw,3rem)/1.05 Georgia,serif;margin:.2em 0}.brand{letter-spacing:.14em;text-transform:uppercase;font-size:.75rem;color:#6e705f}ol{padding-left:1.5rem}li{padding:22px 0;border-bottom:1px solid #d9d4c8}h2{font:600 1.45rem/1.2 Georgia,serif;margin:.1em 0 .35em}p{margin:.5em 0;color:#55564d}.kind{font-size:.85rem;color:#77786e}.empty{padding:32px 0}@media(max-width:600px){.wrap{margin:16px auto;padding:20px}}</style></head><body><main class="wrap"><header><div class="brand">TM Sonder · Shared reading queue</div><h1>Books to read</h1><p>Bookmarks shared by a Sonder reader.</p></header>{{if .Books}}<ol>{{range .Books}}<li><h2>{{.Title}}</h2><div class="kind">{{if .Author}}{{.Author}} · {{end}}{{if eq .Kind "audiobook"}}Audiobook{{else}}Ebook{{end}}</div>{{if .Summary}}<h3>About this book</h3><p>{{.Summary}}</p>{{else}}<p class="kind">No book overview is available yet.</p>{{end}}</li>{{end}}</ol>{{else}}<p class="empty">This reading queue is empty.</p>{{end}}</main></body></html>`))

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

type publicQueueUpdate struct {
	Enabled *bool `json:"enabled"`
}
type publicQueueResponse struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
}
type publicQueueBook struct {
	Title    string        `json:"title"`
	Author   string        `json:"author,omitempty"`
	Kind     api.MediaKind `json:"kind"`
	Summary  string        `json:"summary,omitempty"`
	QueuedAt *time.Time    `json:"queuedAt,omitempty"`
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

func (s *Server) handlePublicQueueSettings(w http.ResponseWriter, r *http.Request) {
	username := s.accountActivityUsername(r)
	if username == "" || !s.accountReady() {
		writeError(w, http.StatusUnauthorized, "Sign in to manage your public reading queue")
		return
	}
	switch r.Method {
	case http.MethodGet:
		token, ok := s.accounts.PublicQueueToken(username)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Account is unavailable")
			return
		}
		response := publicQueueResponse{Enabled: token != ""}
		if token != "" {
			response.URL = "/shared/queue/" + token
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPatch:
		var update publicQueueUpdate
		if err := jsonDecode(w, r, &update); err != nil || update.Enabled == nil {
			writeError(w, http.StatusBadRequest, "Provide enabled state")
			return
		}
		token, ok, err := s.accounts.SetPublicQueue(username, *update.Enabled)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not update public queue sharing")
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "Account is unavailable")
			return
		}
		response := publicQueueResponse{Enabled: token != ""}
		if token != "" {
			response.URL = "/shared/queue/" + token
		}
		writeJSON(w, http.StatusOK, response)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handlePublicQueue(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil || s.accountActivity == nil {
		http.NotFound(w, r)
		return
	}
	token := r.PathValue("token")
	username, ok := s.accounts.PublicQueueOwner(token)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.ensureAccountActivity(username); err != nil {
		writeError(w, http.StatusInternalServerError, "Reading queue unavailable")
		return
	}
	activity := s.accountActivity.snapshot(username)
	books := make([]publicQueueBook, 0, len(activity.Reading.Queue))
	for _, id := range activity.Reading.Queue {
		item, found := s.store.Get(id)
		if !found || (item.Kind != api.KindAudiobook && item.Kind != api.KindEbook) {
			continue
		}
		author := ""
		if item.Author != nil {
			author = *item.Author
		}
		var queuedAt *time.Time
		for _, record := range activity.Reading.Records {
			if record.ItemID == id {
				queuedAt = record.QueuedAt
				break
			}
		}
		books = append(books, publicQueueBook{Title: item.Title, Author: author, Kind: item.Kind, Summary: item.Summary, QueuedAt: queuedAt})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sharedQueueTemplate.Execute(w, struct{ Books []publicQueueBook }{Books: books}); err != nil {
		s.logger.Printf("render public reading queue: %v", err)
	}
}
