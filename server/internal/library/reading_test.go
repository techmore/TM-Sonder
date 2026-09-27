package library

import (
	"path/filepath"
	"testing"
	"time"

	"tm-sonder/server/internal/api"
)

func TestBookReadingStateAndCumulativeSessions(t *testing.T) {
	s := New()
	s.Upsert(
		&Item{MediaItem: api.MediaItem{ID: "audio-a", Kind: api.KindAudiobook}},
		&Item{MediaItem: api.MediaItem{ID: "audio-b", Kind: api.KindAudiobook}},
		&Item{MediaItem: api.MediaItem{ID: "ebook", Kind: api.KindEbook}},
	)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	queued, liked := true, true
	if _, ok := s.SetBookReadingFlags("audio-a", &queued, &liked, now); !ok {
		t.Fatal("book flags were rejected")
	}
	if _, ok := s.SetBookReadingFlags("audio-b", &queued, nil, now.Add(time.Minute)); !ok {
		t.Fatal("second queue item was rejected")
	}
	if got := s.ReorderReadingQueue([]string{"audio-b", "audio-a"}); len(got) != 2 || got[0] != "audio-b" {
		t.Fatalf("queue order = %v", got)
	}

	update := api.ReadingSessionUpdate{SessionID: "session-1", ActiveSeconds: 30, MediaSeconds: 45}
	first, ok := s.RecordReadSession("audio-a", update, now.Add(2*time.Minute))
	if !ok || len(first.Reads) != 1 || first.Reads[0].ActiveSeconds != 30 || first.Reads[0].MediaSeconds != 45 {
		t.Fatalf("first heartbeat = %+v, ok=%v", first, ok)
	}
	// Retried requests are cumulative and must not double-count.
	retried, ok := s.RecordReadSession("audio-a", update, now.Add(3*time.Minute))
	if !ok || retried.Reads[0].ActiveSeconds != 30 || retried.Reads[0].MediaSeconds != 45 {
		t.Fatalf("retry double-counted: %+v", retried)
	}
	update.ActiveSeconds, update.MediaSeconds = 60, 90
	completed, ok := s.RecordReadSession("audio-a", update, now.Add(4*time.Minute))
	if !ok || completed.Reads[0].ActiveSeconds != 60 || completed.Reads[0].MediaSeconds != 90 || completed.Reads[0].CompletedAt != nil {
		t.Fatalf("cumulative heartbeat = %+v", completed.Reads)
	}
	update.Completed = true
	completed, ok = s.RecordReadSession("audio-a", update, now.Add(5*time.Minute))
	if !ok || completed.Reads[0].CompletedAt == nil {
		t.Fatalf("completion was not recorded: %+v", completed.Reads)
	}
	update.SessionID, update.Completed = "session-2", false
	secondRead, ok := s.RecordReadSession("audio-a", update, now.Add(24*time.Hour))
	if !ok || len(secondRead.Reads) != 2 || secondRead.Reads[1].StartedAt != now.Add(24*time.Hour) {
		t.Fatalf("second read run was not created: %+v", secondRead.Reads)
	}
	if _, ok := s.RecordReadSession("ebook", api.ReadingSessionUpdate{SessionID: "not-audio"}, now); ok {
		t.Fatal("ebook should not accept audiobook playback history")
	}
}

func TestReadingStateSurvivesProgressSidecarAndSnapshotMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	s := New()
	s.Upsert(&Item{MediaItem: api.MediaItem{ID: "audio-a", Kind: api.KindAudiobook}})
	queued, liked := true, true
	now := time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC)
	s.SetBookReadingFlags("audio-a", &queued, &liked, now)
	s.RecordReadSession("audio-a", api.ReadingSessionUpdate{SessionID: "session", ActiveSeconds: 90, MediaSeconds: 120}, now.Add(time.Minute))
	if err := s.SaveProgress(path); err != nil {
		t.Fatal(err)
	}

	restored := New()
	restored.Upsert(&Item{MediaItem: api.MediaItem{ID: "audio-a", Kind: api.KindAudiobook}})
	if err := restored.LoadProgress(path); err != nil {
		t.Fatal(err)
	}
	state := restored.ReadingState()
	if len(state.Queue) != 1 || state.Queue[0] != "audio-a" || len(state.Records) != 1 || !state.Records[0].LikedAt.Equal(now) {
		t.Fatalf("reading flags not restored: %+v", state)
	}
	if len(state.Records[0].Reads) != 1 || state.Records[0].Reads[0].ActiveSeconds != 90 || state.Records[0].Reads[0].MediaSeconds != 120 {
		t.Fatalf("reading session not restored: %+v", state.Records[0].Reads)
	}

	legacy := Snapshot{SchemaVersion: 2, Items: []*Item{}, Progress: []api.ProgressRecord{}, Lists: []BookList{}}
	if err := migrateSnapshot(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.SchemaVersion != CurrentSnapshotVersion || legacy.Reading.Queue == nil || legacy.Reading.Records == nil {
		t.Fatalf("v2 migration did not initialize reading state: %+v", legacy)
	}
}
