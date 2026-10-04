package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"tm-sonder/server/internal/api"
)

func TestChapterLookupIdentifier(t *testing.T) {
	for _, r := range []chapterLookupRequest{{ASIN: "../../etc/passwd", Region: "us"}, {ASIN: "B08G9PRS1K", Region: "../"}} {
		if _, err := validChapterLookup(r); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	r, err := validChapterLookup(chapterLookupRequest{ASIN: " b08g9prs1k "})
	if err != nil || r.Region != "us" || r.ASIN != "B08G9PRS1K" {
		t.Fatalf("normalization: %+v %v", r, err)
	}
}
func TestChapterLookupEditionRuntimeAndMilliseconds(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("region") != "uk" {
			t.Error("region omitted")
		}
		if strings.HasSuffix(r.URL.Path, "/chapters") {
			w.Write([]byte(`{"isAccurate":true,"runtimeLengthMs":100000,"chapters":[{"title":"Opening","startOffsetMs":0,"lengthMs":13250},{"title":"Chapter 1","startOffsetMs":13250,"lengthMs":86750}]}`))
		} else {
			w.Write([]byte(`{"title":"Exact edition","authors":[{"name":"Author"}],"narrators":[{"name":"Narrator"}]}`))
		}
	}))
	defer service.Close()
	got, err := lookupAudnexus(context.Background(), service.Client(), service.URL, chapterLookupRequest{ASIN: "B08G9PRS1K", Region: "uk"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.Chapters[1].StartSeconds != 13.25 || got.MatchConfidence != "runtime-compatible-needs-edition-review" || got.Narrators[0] != "Narrator" {
		t.Fatalf("%+v", got)
	}
	mismatch, err := lookupAudnexus(context.Background(), service.Client(), service.URL, chapterLookupRequest{ASIN: "B08G9PRS1K", Region: "uk"}, 500)
	if err != nil || mismatch.MatchConfidence != "runtime-mismatch" {
		t.Fatalf("%+v %v", mismatch, err)
	}
}
func TestChapterLookupMalformedAndOversized(t *testing.T) {
	for _, body := range []string{`{"chapters":[{"title":"Late","startOffsetMs":2000,"lengthMs":1000},{"title":"Earlier","startOffsetMs":1000,"lengthMs":1000}],"runtimeLengthMs":5000}`, strings.Repeat("x", chapterMapLimit+1)} {
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/chapters") {
				w.Write([]byte(body))
			} else {
				w.Write([]byte(`{"title":"Book"}`))
			}
		}))
		_, err := lookupAudnexus(context.Background(), service.Client(), service.URL, chapterLookupRequest{ASIN: "B08G9PRS1K"}, 5)
		service.Close()
		if err == nil {
			t.Error("accepted invalid response")
		}
	}
}
func TestChapterMarkerValidation(t *testing.T) {
	for _, markers := range [][]chapterMapMarker{{{Title: "A", StartSeconds: -1}}, {{Title: "A", StartSeconds: 101}}, {{Title: "A", EndSeconds: 110}}, {{Title: "A", StartSeconds: 10}, {Title: "B", StartSeconds: 10}}, {{Title: "A", EndSeconds: 60}, {Title: "B", StartSeconds: 50}}} {
		if _, err := validateChapterMarkers(markers, 100); err == nil {
			t.Fatalf("accepted %+v", markers)
		}
	}
	got, err := validateChapterMarkers([]chapterMapMarker{{Title: " A "}, {Title: "B", StartSeconds: 50}}, 100)
	if err != nil || got[0].EndSeconds != 50 || got[1].EndSeconds != 100 {
		t.Fatalf("%+v %v", got, err)
	}
}
func TestChapterMapReviewPersistenceAndReset(t *testing.T) {
	f := newFixture(t, nil)
	it := f.addItem(t, "chapter-book", "Book")
	it.Kind = api.KindAudiobook
	f.store.Upsert(it)
	send := func(method, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, f.ts.URL+"/api/audiobooks/chapter-book/chapter-map", strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	body := `{"chapters":[{"title":"First","startSeconds":0},{"title":"Second","startSeconds":50}],"sourceURL":"https://example.org/edition","editionConfirmed":false}`
	resp := send("POST", body)
	readAll(t, resp)
	if resp.StatusCode != 422 {
		t.Fatalf("unreviewed %d", resp.StatusCode)
	}
	resp = send("POST", strings.Replace(body, `false`, `true`, 1))
	response := readAll(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("import %d %s", resp.StatusCode, response)
	}
	_, text := get(t, f.ts.URL+"/api/audiobooks/chapter-book/chapters")
	var timeline audiobookChapterTimeline
	if err := json.Unmarshal([]byte(text), &timeline); err != nil {
		t.Fatal(err)
	}
	if timeline.Source != "reviewed-import" || timeline.ChapterCount != 2 || timeline.Chapters[1].PartID != it.ID {
		t.Fatalf("%+v", timeline)
	}
	parts, _ := f.s.chapterBookParts(it.ID)
	path := f.s.chapterMapPath(parts)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	it.DurationSeconds = 200
	f.store.Upsert(it)
	_, text = get(t, f.ts.URL+"/api/audiobooks/chapter-book/chapters")
	if strings.Contains(text, `reviewed-import`) {
		t.Fatalf("stale import: %s", text)
	}
	resp = send("DELETE", "")
	readAll(t, resp)
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("not reset: %v", err)
	}
}
func TestImportedChapterPartCoordinates(t *testing.T) {
	parts := []catalogPart{{ID: "one", Index: 1, DurationSeconds: 40}, {ID: "two", Index: 2, DurationSeconds: 60}}
	timeline := importedChapterTimeline("two", parts, chapterMapSidecar{SourceURL: "https://example.org", Chapters: []chapterMapMarker{{Title: "First", StartSeconds: 0, EndSeconds: 40}, {Title: "Second", StartSeconds: 40, EndSeconds: 100}}})
	if timeline.Chapters[1].PartID != "two" || timeline.Chapters[1].PartIndex != 2 || timeline.Chapters[1].StartSeconds != 40 {
		t.Fatalf("%+v", timeline)
	}
}
func TestChapterEditorTrustBoundary(t *testing.T) {
	f := newFixture(t, nil)
	r := httptest.NewRequest("POST", "https://attacker.example/api/audiobooks/id/chapter-map", nil)
	r.RemoteAddr = "127.0.0.1:123"
	if f.s.chapterEditorAllowed(r) {
		t.Fatal("trusted rebound host")
	}
	r.Host = "127.0.0.1"
	r.RemoteAddr = "192.168.1.2:123"
	if f.s.chapterEditorAllowed(r) {
		t.Fatal("trusted remote peer")
	}
}

func TestChapterMapInvalidatedWhenSourceFileChanges(t *testing.T) {
	f := newFixture(t, nil)
	it := f.addItem(t, "changed-file", "Book")
	it.Kind = api.KindAudiobook
	f.store.Upsert(it)
	parts, err := f.s.chapterBookParts(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := f.s.chapterBookFingerprint(parts)
	if err := os.WriteFile(it.FilePath, []byte("replacement recording with same catalog runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	if before == f.s.chapterBookFingerprint(parts) {
		t.Fatal("fingerprint failed to detect changed recording")
	}
}
func TestChapterMapRejectsRuntimeMismatch(t *testing.T) {
	f := newFixture(t, nil)
	it := f.addItem(t, "runtime-book", "Book")
	it.Kind = api.KindAudiobook
	f.store.Upsert(it)
	body := `{"chapters":[{"title":"First","startSeconds":0}],"sourceURL":"https://example.org/edition","editionConfirmed":true,"runtimeSeconds":500}`
	resp, err := http.Post(f.ts.URL+"/api/audiobooks/runtime-book/chapter-map", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	readAll(t, resp)
	if resp.StatusCode != 422 {
		t.Fatalf("runtime mismatch accepted: %d", resp.StatusCode)
	}
}
