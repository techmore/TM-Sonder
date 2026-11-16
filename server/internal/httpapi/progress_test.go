package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
)

func postProgressUpdate(t *testing.T, f *fixture, itemID string, update api.PlaybackStateUpdate, cookie *http.Cookie) (int, api.PlaybackSession) {
	t.Helper()
	payload, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/progress/"+itemID, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(rec, req)
	var session api.PlaybackSession
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode %d response: %v (%s)", rec.Code, err, rec.Body.String())
	}
	return rec.Code, session
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestProgressStaleBaseReturnsConflictInsteadOfOverwriting(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "book", "Book")

	first := time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC)
	later := first.Add(2 * time.Hour)
	staleWrite := later.Add(time.Hour)

	// The device that listened last saves its position.
	if code, session := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 100, Duration: 600, UpdatedAt: ptrTime(first),
	}, nil); code != http.StatusOK || session.Seconds != 100 {
		t.Fatalf("initial save: %d %+v", code, session)
	}
	if code, session := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 160, Duration: 600, UpdatedAt: ptrTime(later), BaseUpdatedAt: ptrTime(first),
	}, nil); code != http.StatusOK || session.Seconds != 160 {
		t.Fatalf("newer save: %d %+v", code, session)
	}

	// A tab left open since the first save now writes its older position with
	// a later local clock. Its base proves it never saw the newer progress.
	code, session := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 110, Duration: 600, UpdatedAt: ptrTime(staleWrite), BaseUpdatedAt: ptrTime(first),
	}, nil)
	if code != http.StatusConflict {
		t.Fatalf("stale write status = %d, want 409", code)
	}
	if session.Seconds != 160 {
		t.Fatalf("conflict response seconds = %v, want stored 160", session.Seconds)
	}
	if rec, ok := f.store.ProgressFor("book"); !ok || rec.Seconds != 160 {
		t.Fatalf("stale write replaced stored progress: %+v ok=%v", rec, ok)
	}
}

func TestProgressCurrentBaseAllowsDeliberateRewind(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "movie", "Movie")

	first := time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)
	if code, _ := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 400, Duration: 600, UpdatedAt: ptrTime(first),
	}, nil); code != http.StatusOK {
		t.Fatalf("initial save status = %d", code)
	}

	// The same client saw the stored record and deliberately seeks backward.
	code, session := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 90, Duration: 600, UpdatedAt: ptrTime(later), BaseUpdatedAt: ptrTime(first),
	}, nil)
	if code != http.StatusOK || session.Seconds != 90 {
		t.Fatalf("rewind: %d %+v", code, session)
	}
	if rec, _ := f.store.ProgressFor("movie"); rec.Seconds != 90 {
		t.Fatalf("rewind not stored: %+v", rec)
	}
}

func TestProgressOutOfOrderTimestampIsReportedNotDroppedSilently(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "movie", "Movie")

	newer := time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)
	older := newer.Add(-time.Hour)
	if code, _ := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 300, Duration: 600, UpdatedAt: ptrTime(newer),
	}, nil); code != http.StatusOK {
		t.Fatalf("initial save status = %d", code)
	}

	code, session := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 120, Duration: 600, UpdatedAt: ptrTime(older),
	}, nil)
	if code != http.StatusConflict {
		t.Fatalf("out-of-order status = %d, want 409", code)
	}
	if session.Seconds != 300 {
		t.Fatalf("out-of-order response seconds = %v, want 300", session.Seconds)
	}
	if rec, _ := f.store.ProgressFor("movie"); rec.Seconds != 300 {
		t.Fatalf("out-of-order write replaced stored progress: %+v", rec)
	}
}

func TestProgressFutureTimestampIsClamped(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "movie", "Movie")

	now := time.Now().UTC()
	future := now.Add(48 * time.Hour)
	if code, _ := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 45, Duration: 600, UpdatedAt: ptrTime(future),
	}, nil); code != http.StatusOK {
		t.Fatalf("future save status = %d", code)
	}
	rec, _ := f.store.ProgressFor("movie")
	if rec.UpdatedAt.After(now.Add(progressClockSkew + time.Minute)) {
		t.Fatalf("future timestamp was trusted: %v", rec.UpdatedAt)
	}
	// A later ordinary write must not be blocked by the bogus timestamp.
	if code, session := postProgressUpdate(t, f, "movie", api.PlaybackStateUpdate{
		Seconds: 80, Duration: 600, UpdatedAt: ptrTime(now.Add(time.Minute)),
	}, nil); code != http.StatusOK || session.Seconds != 80 {
		t.Fatalf("write after clamp: %d %+v", code, session)
	}
}

func TestAccountProgressStaleBaseIsConfinedAndRejected(t *testing.T) {
	f := newFixture(t, nil)
	f.addItem(t, "book", "Book")
	if err := f.s.accounts.Setup("owner", "a long enough password"); err != nil {
		t.Fatal(err)
	}
	token, _, err := f.s.accounts.CreateSession("owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.ensureAccountActivity("owner"); err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: token}

	first := time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC)
	later := first.Add(2 * time.Hour)
	if code, _ := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 100, Duration: 600, UpdatedAt: ptrTime(first),
	}, cookie); code != http.StatusOK {
		t.Fatalf("initial save status = %d", code)
	}
	if code, _ := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 160, Duration: 600, UpdatedAt: ptrTime(later), BaseUpdatedAt: ptrTime(first),
	}, cookie); code != http.StatusOK {
		t.Fatalf("newer save status = %d", code)
	}

	code, session := postProgressUpdate(t, f, "book", api.PlaybackStateUpdate{
		Seconds: 110, Duration: 600, UpdatedAt: ptrTime(later.Add(time.Hour)), BaseUpdatedAt: ptrTime(first),
	}, cookie)
	if code != http.StatusConflict || session.Seconds != 160 {
		t.Fatalf("stale account write: %d %+v", code, session)
	}
	if rec, ok := f.s.accountActivity.progressFor("owner", "book"); !ok || rec.Seconds != 160 {
		t.Fatalf("stale account write replaced progress: %+v ok=%v", rec, ok)
	}
	// The legacy shared store must remain untouched by account-scoped writes.
	if rec, ok := f.store.ProgressFor("book"); ok {
		t.Fatalf("account write leaked into shared store: %+v", rec)
	}
}
