package audiobookopt

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

func TestMP3ConversionFixture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg absent")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe absent")
	}
	dir := t.TempDir()
	store := library.New()
	libraryID := "books"
	for i, name := range []string{"01.mp3", "02.mp3", "03.mp3"} {
		path := filepath.Join(dir, "Author", "Book", name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := runCommand(context.Background(), ffmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.6", "-c:a", "libmp3lame", path); err != nil {
			t.Fatal(err)
		}
		st, _ := os.Stat(path)
		id := name
		store.Upsert(&library.Item{MediaItem: api.MediaItem{ID: id, Title: "Book", Kind: api.KindAudiobook, Format: api.FormatMP3, DurationSeconds: 0.6, LibraryID: &libraryID}, FilePath: path, SizeBytes: st.Size(), ModTime: st.ModTime()})
		_ = i
	}
	m, err := NewMP3(store, []config.Library{{ID: libraryID, Path: dir, Kind: "audiobook"}}, t.TempDir(), ffmpeg, ffprobe)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	s := m.Status("01.mp3")
	if !s.Eligible || s.PartCount != 3 {
		t.Fatalf("status %+v", s)
	}
	j, err := m.Enqueue("01.mp3", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Enqueue("02.mp3", false); err != ErrConflict {
		t.Fatalf("duplicate %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s = m.Status("01.mp3")
		if s.Job != nil && (s.Job.Status == "ready" || s.Job.Status == "failed") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.Job == nil || s.Job.Status != "ready" {
		t.Fatalf("job %+v", s.Job)
	}
	if s.Job.ChapterCount != 3 || !s.Job.FullDecode {
		t.Fatalf("receipt %+v", s.Job)
	}
	f, err := m.Open("02.mp3")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, name := range []string{"01.mp3", "02.mp3", "03.mp3"} {
		if _, err = os.Stat(filepath.Join(dir, "Author", "Book", name)); err != nil {
			t.Fatal("original missing", err)
		}
	}
	if _, err = os.Stat(filepath.Join(m.root, j.ID, "receipt.json")); err != nil {
		t.Fatal(err)
	}
}
func TestMP3MetadataEscape(t *testing.T) {
	if got := ffEscape("A= B;#\nC"); got != "A\\= B\\;\\# C" {
		t.Fatal(got)
	}
}

func TestMP3RestartMarksInterruptedAndRejectsDuplicateFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "audiobook-m4b")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "jobs.json"), []byte(`[{"id":"old","itemID":"a","status":"encoding","parts":[{"id":"a"}]}]`), 0600); err != nil {
		t.Fatal(err)
	}
	store := library.New()
	m, err := NewMP3(store, nil, dir, "ffmpeg", "ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	s := m.Status("a")
	if s.Job == nil || s.Job.Status != "interrupted" {
		t.Fatalf("restart %+v", s)
	}
	libraryID := "books"
	libs := []config.Library{{ID: libraryID, Path: dir, Kind: "audiobook"}}
	m.libraries = libs
	for _, id := range []string{"a", "b"} {
		store.Upsert(&library.Item{MediaItem: api.MediaItem{ID: id, Kind: api.KindAudiobook, LibraryID: &libraryID, DurationSeconds: 100}, FilePath: filepath.Join(dir, "Author", "Book", id+".mp3"), SizeBytes: 100})
	}
	if s := m.Status("a"); s.Eligible {
		t.Fatal("ambiguous duplicate copies must not be merged")
	}
}

func TestMP3ManyShortPartsHaveSampleBasedChapterOffsets(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg absent")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe absent")
	}
	dir := t.TempDir()
	store := library.New()
	libraryID := "books"
	const count = 18
	const partDuration = 0.13
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%03d.mp3", i)
		path := filepath.Join(dir, "Author", "Book", name)
		os.MkdirAll(filepath.Dir(path), 0700)
		frequency := 440
		if i%2 == 1 {
			frequency = 880
		}
		rate := "44100"
		if i%3 == 1 {
			rate = "22050"
		}
		if err := runCommand(context.Background(), ffmpeg, "-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:duration=%f", frequency, partDuration), "-ar", rate, "-c:a", "libmp3lame", path); err != nil {
			t.Fatal(err)
		}
		st, _ := os.Stat(path)
		store.Upsert(&library.Item{MediaItem: api.MediaItem{ID: name, Title: "Book", Kind: api.KindAudiobook, LibraryID: &libraryID, DurationSeconds: partDuration}, FilePath: path, SizeBytes: st.Size(), ModTime: st.ModTime()})
	}
	m, err := NewMP3(store, []config.Library{{ID: libraryID, Path: dir, Kind: "audiobook"}}, t.TempDir(), ffmpeg, ffprobe)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	job, err := m.Enqueue("000.mp3", false)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var s MP3Status
	for time.Now().Before(deadline) {
		s = m.Status("000.mp3")
		if s.Job != nil && (s.Job.Status == "ready" || s.Job.Status == "failed") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.Job == nil || s.Job.Status != "ready" {
		t.Fatalf("job %+v", s.Job)
	}
	output := filepath.Join(m.root, job.ID, "book.m4b")
	pr, err := m.probe.inspect(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if abs(parseFloat(pr.Format.Duration)-count*partDuration) > 0.05 {
		t.Fatalf("duration drift %s", pr.Format.Duration)
	}
	if audioStreams(pr)[0].Channels != 1 {
		t.Fatal("mono source unexpectedly expanded")
	}
	cmd := exec.Command(ffmpeg, "-v", "error", "-i", output, "-map", "0:a:0", "-f", "s16le", "-ac", "1", "-ar", "44100", "-")
	pcm, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range pr.Chapters {
		expected := float64(i) * partDuration
		if abs(parseFloat(c.Start)-expected) > 0.002 {
			t.Fatalf("part %d start %s want %.4f", i, c.Start, expected)
		}
		start := int((expected + 0.035) * 44100)
		end := int((expected + 0.095) * 44100)
		crossings := 0
		for sample := start + 1; sample < end; sample++ {
			a := int16(binary.LittleEndian.Uint16(pcm[2*(sample-1):]))
			b := int16(binary.LittleEndian.Uint16(pcm[2*sample:]))
			if a <= 0 && b > 0 {
				crossings++
			}
		}
		frequency := float64(crossings) / 0.06
		want := 440.0
		if i%2 == 1 {
			want = 880
		}
		if abs(frequency-want) > 35 {
			t.Fatalf("part %d audio frequency %.1f want %.1f: chapter/content misalignment", i, frequency, want)
		}
	}
}
