package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Goldens lock the wire shapes documented in API.md: camelCase keys,
// explicit nulls for nil optionals (no omitempty on core DTOs), and no
// filesystem paths. Deviations here break existing iOS clients.

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHealthGolden(t *testing.T) {
	got := mustJSON(t, HealthResponse{
		Status: "ok", Name: "TM Sonder", App: "TM Sonder", ID: "tm-sonder",
		Service: "_tmsonder._tcp", Library: "/api/library",
	})
	want := `{"status":"ok","name":"TM Sonder","app":"TM Sonder","id":"tm-sonder","service":"_tmsonder._tcp","library":"/api/library","allowLAN":false,"requiresPairing":false}`
	if got != want {
		t.Errorf("health bytes drifted:\n got %s\nwant %s", got, want)
	}
}

func TestPlaybackTrackNullRules(t *testing.T) {
	got := mustJSON(t, PlaybackTrack{ID: "sidecar:0", Label: "Ep", Kind: TrackSidecar})
	want := `{"id":"sidecar:0","label":"Ep","languageCode":null,"kind":"sidecar","url":null}`
	if got != want {
		t.Errorf("track null rules:\n got %s\nwant %s", got, want)
	}
	withOpt := mustJSON(t, PlaybackTrack{
		ID: "embedded-audio:0", Label: "Japanese", Kind: TrackEmbedded,
		LanguageCode: strPtr("ja"), URL: strPtr("/subtitles/x/0"),
	})
	for _, frag := range []string{`"languageCode":"ja"`, `"url":"/subtitles/x/0"`} {
		if !strings.Contains(withOpt, frag) {
			t.Errorf("missing %s in %s", frag, withOpt)
		}
	}
}

func TestMediaItemKeyParity(t *testing.T) {
	m := MediaItem{Tags: []string{}, EmbeddedAudioTracks: []PlaybackTrack{}, EmbeddedSubtitleTracks: []PlaybackTrack{}}
	var raw map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, m)), &raw); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"id", "title", "subtitle", "kind", "studio", "year", "durationSeconds",
		"format", "libraryID", "tags", "summary", "progressSeconds",
		"showTitle", "seasonNumber", "episodeNumber", "metadataIDSource",
		"metadataID", "edition", "splitPart", "isPlaceholder", "posterURL",
		"backdropURL", "embeddedAudioTracks", "embeddedSubtitleTracks",
		"trackProbeUpdatedAt", "probedWidth", "probedHeight", "probedCodec",
		"probedBitrate", "bookValidation", "coverSource",
	}
	for _, k := range wantKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("mediaItem missing key %q", k)
		}
	}
	if len(raw) != len(wantKeys) {
		extra := map[string]bool{}
		for k := range raw {
			extra[k] = true
		}
		for _, k := range wantKeys {
			delete(extra, k)
		}
		for k := range extra {
			t.Errorf("mediaItem unexpected extra key %q", k)
		}
	}
	// Null-vs-value rules for nil optionals.
	for _, k := range []string{"libraryID", "showTitle", "posterURL", "probedWidth"} {
		if raw[k] != nil {
			t.Errorf("%s should be null when unset, got %v", k, raw[k])
		}
	}
	// Non-optional Swift arrays encode as [], never null.
	if raw["tags"] == nil || raw["embeddedAudioTracks"] == nil {
		t.Error("tags/tracks must serialize as [] not null")
	}
}

func TestMediaItemNeverEmitsFilePath(t *testing.T) {
	m := MediaItem{ID: "x", Title: "T"}
	s := mustJSON(t, m)
	if strings.Contains(s, "filePath") || strings.Contains(s, "FilePath") {
		t.Errorf("filesystem path leaked: %s", s)
	}
}

func TestProgressRecordOmitRules(t *testing.T) {
	// Locked rule: track-selection optionals are OMITTED when unset
	// (documented deviation from Swift's null emission).
	rec := ProgressRecord{
		ID: "p1", ItemID: "i1", Seconds: 5, Duration: 10,
		UpdatedAt: time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
	}
	got := mustJSON(t, rec)
	want := `{"id":"p1","itemID":"i1","seconds":5,"duration":10,"updatedAt":"2026-08-21T12:00:00Z"}`
	if got != want {
		t.Errorf("progress omit rules:\n got %s\nwant %s", got, want)
	}
	sub := false
	rec.SubtitlesEnabled = &sub
	if !strings.Contains(mustJSON(t, rec), `"subtitlesEnabled":false`) {
		t.Error("set optional should serialize")
	}
}

func TestPlaybackSessionOmitRules(t *testing.T) {
	id := "i1"
	sess := PlaybackSession{ItemID: &id, StreamURL: "/stream/i1"}
	got := mustJSON(t, sess)
	// itemID/streamURL always present; selection fields omitted until set.
	wantFrag := `"itemID":"i1","streamURL":"/stream/i1"`
	if !strings.HasPrefix(got, "{\""+wantFrag[2:]) && !strings.Contains(got, wantFrag) {
		t.Errorf("session prefix wrong: %s", got)
	}
	if strings.Contains(got, "audioTracks") == false {
		t.Errorf("audioTracks must always be present: %s", got)
	}
	var raw map[string]any
	json.Unmarshal([]byte(got), &raw)
	for _, k := range []string{"seconds", "duration", "percent", "audioTracks", "subtitleTracks"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("session missing %q", k)
		}
	}
	for _, k := range []string{"updatedAt", "audioTrackID", "subtitlesEnabled"} {
		if _, ok := raw[k]; ok {
			t.Errorf("session %q should be omitted when unset", k)
		}
	}
}

func TestDiscoveryAndThemeShapes(t *testing.T) {
	d := DiscoveryResponse{}
	var raw map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, d)), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"serverID", "version", "build", "localURL", "lanURL",
		"discoveryMethods", "tailscaleHint", "capabilities", "endpoints", "theme"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("discovery missing %q", k)
		}
	}
	if raw["lanURL"] != nil {
		t.Error("lanURL should be null when unset")
	}
	th := ThemeSnapshot{Preset: "earthy"}
	ts := mustJSON(t, th)
	for _, k := range []string{"preset", "background", "sidebar", "surface", "border", "accent", "text"} {
		if !strings.Contains(ts, `"`+k+`"`) {
			t.Errorf("theme missing %q: %s", k, ts)
		}
	}
}

func TestLibraryResponseShape(t *testing.T) {
	lr := LibraryResponse{}
	var raw map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, lr)), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"items", "progress", "mediaDirectories", "activity", "serverSettings", "theme"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("library response missing %q", k)
		}
	}
	// serverSettings/theme are nullable pointers on the wire.
	if raw["serverSettings"] != nil || raw["theme"] != nil {
		t.Error("serverSettings/theme should be null when unset")
	}
}

func strPtr(s string) *string { return &s }
