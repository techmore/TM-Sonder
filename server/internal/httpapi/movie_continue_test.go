package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Before the first account exists there is no activity store to consult. The
// library page requests this shelf on every load, and it used to answer 500
// until the owner finished account setup.
func TestMovieContinueBeforeAccountSetup(t *testing.T) {
	f := newFixture(t, nil)
	resp, err := http.Get(f.ts.URL + "/api/movies/continue")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/movies/continue = %d, want 200", resp.StatusCode)
	}
	var body struct {
		DismissedMovieIDs []string `json:"dismissedMovieIDs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DismissedMovieIDs == nil || len(body.DismissedMovieIDs) != 0 {
		t.Errorf("dismissedMovieIDs = %v, want an empty list", body.DismissedMovieIDs)
	}

	item := f.addItem(t, "m1", "Movie")
	req, _ := http.NewRequest(http.MethodPatch, f.ts.URL+"/api/movies/"+item.ID+"/continue", strings.NewReader(`{"dismissed":true}`))
	req.Header.Set("Content-Type", "application/json")
	patch, err := f.ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer patch.Body.Close()
	if patch.StatusCode != http.StatusUnauthorized {
		t.Errorf("PATCH without an account = %d, want 401", patch.StatusCode)
	}
}
