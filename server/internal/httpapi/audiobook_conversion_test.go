package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"tm-sonder/server/internal/audiobookopt"
)

func TestMP3ConversionRoutesAndOwnerBoundary(t *testing.T) {
	f := newFixture(t, nil)
	m, err := audiobookopt.NewMP3(f.store, nil, f.s.cfg().DataDir, "ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	f.s.SetMP3Converter(m)
	request := func(method, path, body, remote, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = remote
		r.Host = host
		w := httptest.NewRecorder()
		f.s.Handler().ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/api/audiobooks/missing/conversion", "", "127.0.0.1:1234", "127.0.0.1")
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var state audiobookopt.MP3Status
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || state.Eligible || state.TargetCodec != "aac" {
		t.Fatalf("state %+v: %v", state, err)
	}
	if w := request("POST", "/api/audiobooks/missing/conversion", "{", "127.0.0.1:1234", "127.0.0.1"); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: %d", w.Code)
	}
	if w := request("POST", "/api/audiobooks/missing/conversion", "{}", "198.51.100.10:1234", "sonder.example"); w.Code != http.StatusUnauthorized && w.Code != http.StatusForbidden {
		t.Fatalf("unauthorized conversion: %d", w.Code)
	}
	if w := request("GET", "/api/audiobooks/missing/conversion/download", "", "127.0.0.1:1234", "127.0.0.1"); w.Code != http.StatusNotFound {
		t.Fatalf("missing output: %d", w.Code)
	}
}
