package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tm-sonder/server/internal/audiobookopt"
)

func TestAudiobookOptimizationQueueControls(t *testing.T) {
	f := newFixture(t, nil)
	optimizer, err := audiobookopt.New(f.store, nil, f.s.cfg().DataDir, "ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	f.s.SetAudiobookOptimizer(optimizer)
	t.Cleanup(func() { _ = optimizer.Close(context.Background()) })

	request := func(method, path string) audiobookopt.Snapshot {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Host = "127.0.0.1"
		w := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: status %d: %s", method, path, w.Code, w.Body.String())
		}
		var snapshot audiobookopt.Snapshot
		if err := json.Unmarshal(w.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	if got := request(http.MethodPost, "/api/optimization/audiobooks/queue/pause"); !got.Paused {
		t.Fatal("pause route did not pause the queue")
	}
	if got := request(http.MethodGet, "/api/optimization/audiobooks/jobs"); !got.Paused {
		t.Fatal("jobs route did not report the persisted paused state")
	}
	if got := request(http.MethodPost, "/api/optimization/audiobooks/queue/resume"); got.Paused {
		t.Fatal("resume route left the queue paused")
	}
}
