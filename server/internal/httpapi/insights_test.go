package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

func insightFixtureItem(id, lib, path string, bytes int64) *library.Item {
	return &library.Item{MediaItem: api.MediaItem{ID: id, Title: id, LibraryID: &lib, Kind: api.KindAudiobook, Format: api.FormatMP3, DurationSeconds: 120}, FilePath: "/private/media/" + lib + "/" + path, SourceRelativePath: path, SizeBytes: bytes}
}

func TestLibraryInsightsGroupsAndFiltersCatalog(t *testing.T) {
	libs := []config.Library{{ID: "a", Name: "Audio", Kind: "audiobook", Path: "/private/media/a"}, {ID: "b", Name: "Other", Kind: "audiobook", Path: "/private/media/b"}}
	p1, p2, other := insightFixtureItem("p1", "a", "Author/Book/01.mp3", 100), insightFixtureItem("p2", "a", "Author/Book/02.mp3", 200), insightFixtureItem("other", "b", "Author/Book/01.mp3", 400)
	p1.Genres = []string{"Fantasy", "fantasy", " Fiction "}
	p2.Genres = []string{"Fantasy"}
	p1.Tags = []string{"not a genre"}
	placeholder := insightFixtureItem("placeholder", "a", "", 999)
	placeholder.FilePath = ""
	unconfigured := insightFixtureItem("unknown", "removed", "Book/01.mp3", 999)
	p3 := insightFixtureItem("p3", "a", "Author/Book/03.mp3", 50)
	items := []*library.Item{p2, p1, p3, other, placeholder, unconfigured}
	report := buildLibraryInsights(items, libs, "", "", api.ReadingState{})
	if report.Totals.Titles != 2 || report.Totals.Files != 4 || report.Totals.Bytes != 750 || report.Totals.MissingGenreTitles != 1 {
		t.Fatalf("totals: %+v", report.Totals)
	}
	if len(report.Genres) != 2 || report.Genres[0].Titles != 1 {
		t.Fatalf("genres: %+v", report.Genres)
	}
	if report.LargestTitles[0].Bytes != 400 || report.LargestTitles[1].Bytes != 350 || report.LargestTitles[1].ID != "p1" || report.LargestTitles[1].Files != 3 {
		t.Fatalf("largest titles: %+v", report.LargestTitles)
	}
	if report.LargestFiles[0].Name != "01.mp3" {
		t.Fatalf("missing physical filename: %+v", report.LargestFiles[0])
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "/private/") || strings.Contains(string(encoded), "not a genre") {
		t.Fatalf("leaked paths or tags: %s", encoded)
	}
	filtered := buildLibraryInsights(items, libs, "a", "audiobook", api.ReadingState{})
	if filtered.Totals.Titles != 1 || filtered.Totals.Bytes != 350 || len(filtered.Libraries) != 2 || filtered.Libraries[1].Files != 0 {
		t.Fatalf("filtered: %+v", filtered)
	}
	empty := buildLibraryInsights(items, libs, "a", "movie", api.ReadingState{})
	if empty.Totals.Files != 0 || len(empty.LargestTitles) != 0 || empty.LargestTitles == nil {
		t.Fatalf("empty: %+v", empty)
	}
}

func TestLibraryInsightsCountsRecordedSessionsOnly(t *testing.T) {
	libs := []config.Library{{ID: "a", Kind: "audiobook", Path: "/private/media/a"}}
	items := []*library.Item{insightFixtureItem("p1", "a", "Book/01.mp3", 100), insightFixtureItem("p2", "a", "Book/02.mp3", 200), insightFixtureItem("p3", "a", "Book/03.mp3", 50)}
	now := time.Now().UTC()
	record := func(id, session string, active, media float64) api.BookReadingRecord {
		return api.BookReadingRecord{ItemID: id, Reads: []api.BookReadRun{{Sessions: []api.ReadingSession{{ID: session, ActiveSeconds: active, MediaSeconds: media, UpdatedAt: now}}}}}
	}
	reading := api.ReadingState{Records: []api.BookReadingRecord{record("p1", "same", 10, 15), record("p2", "same", 20, 30), record("p2", "second", 40, 60), record("deleted", "hidden", 900, 900), {ItemID: "p1", LikedAt: &now}}}
	report := buildLibraryInsights(items, libs, "", "", reading)
	if report.Activity.Sessions != 2 || report.Activity.ActiveSeconds != 60 || report.Activity.MediaSeconds != 90 || len(report.Activity.MostAccessed) != 1 {
		t.Fatalf("activity: %+v", report.Activity)
	}
	if report.Activity.MostAccessed[0].ID != "p1" {
		t.Fatalf("representative: %+v", report.Activity.MostAccessed)
	}
}

func TestLibraryInsightsEndpointValidationAndAccountIsolation(t *testing.T) {
	f := newFixture(t, func(cfg *config.Config) {
		cfg.AllowLAN = true
		cfg.Libraries = []config.Library{{ID: "audio", Name: "Audio", Kind: "audiobook", Path: "/private/media/audio"}}
	})
	f.store.Upsert(insightFixtureItem("audio", "audio", "Book/01.mp3", 100))
	for _, query := range []string{"?libraryID=missing", "?kind=bogus", "?kind=plex"} {
		resp, _ := get(t, f.ts.URL+"/api/library/insights"+query)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: %d", query, resp.StatusCode)
		}
	}
	if err := f.s.accounts.Setup("owner", "long enough password"); err != nil {
		t.Fatal(err)
	}
	invite, _, _ := f.s.accounts.InviteInfo("owner")
	if err := f.s.accounts.Signup("other", "another long password", invite); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ensureAccountActivity("owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.accountActivity.recordReadSession("owner", "audio", api.ReadingSessionUpdate{SessionID: "private-session", ActiveSeconds: 50, MediaSeconds: 75}, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, username := range []string{"owner", "other"} {
		token, _, err := f.s.accounts.CreateSession(username)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/library/insights", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		result := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(result, req)
		if result.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", username, result.Code, result.Body.String())
		}
		var report libraryInsights
		if err := json.Unmarshal(result.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		want := 0
		if username == "owner" {
			want = 1
		}
		if report.Activity.Sessions != want {
			t.Fatalf("%s sessions %d want %d", username, report.Activity.Sessions, want)
		}
		if result.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("account response must not be shared-cached")
		}
	}
}
