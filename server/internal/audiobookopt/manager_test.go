package audiobookopt

import (
	"context"
	"testing"
	"time"

	"tm-sonder/server/internal/library"
)

func TestQueuePausePersistsAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	first, err := New(library.New(), nil, dataDir, "ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	second, err := New(library.New(), nil, dataDir, "ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close(context.Background())
	if !second.Snapshot().Paused {
		t.Fatal("paused state did not survive restart")
	}
	if err := second.Resume(); err != nil {
		t.Fatal(err)
	}
	if second.Snapshot().Paused {
		t.Fatal("resume did not clear paused state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := second.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
