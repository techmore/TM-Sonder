package audiobookopt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunJobStagesValidatedOutputAndPreservesSource(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("requires unix statfs")
	}

	root := t.TempDir()
	libRoot := filepath.Join(root, "library")
	bookDir := filepath.Join(libRoot, "Test Author", "Test Book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bookDir, "Test Book.m4b")
	metadata := filepath.Join(bookDir, "chapters.txt")
	chapterData := ";FFMETADATA1\ntitle=Test Book\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=15000\ntitle=First Half\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=15000\nEND=30000\ntitle=Second Half\n"
	if err := os.WriteFile(metadata, []byte(chapterData), 0o600); err != nil {
		t.Fatal(err)
	}
	// A tiny AAC M4B with an embedded cover exercises the real encode, remux,
	// container probe, full decode, checksum, and NAS staging path.
	cmd := exec.Command(ffmpeg, "-hide_banner", "-v", "error", "-nostdin", "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=30",
		"-f", "lavfi", "-i", "color=c=red:s=96x96:d=1",
		"-f", "ffmetadata", "-i", metadata,
		"-map", "0:a:0", "-map", "1:v:0", "-c:a", "aac", "-b:a", "160k",
		"-c:v", "mjpeg", "-frames:v", "1", "-disposition:v:0", "attached_pic",
		"-map_metadata", "2", "-map_chapters", "2", "-f", "mp4", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %v: %s", err, out)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(before)
	st, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}

	m := &Manager{root: filepath.Join(root, "data", "audiobook-optimization"), ffmpeg: ffmpeg, ffprobe: ffprobe}
	job := diskJob{
		Job:        Job{ID: "integration-job", Title: "Test Book", SourceBytes: st.Size(), BitrateKbps: 32, Attempt: 1},
		SourcePath: source, LibraryRoot: libRoot, RelativePath: filepath.Join("Test Author", "Test Book", "Test Book.m4b"),
		SourceMtimeNS: st.ModTime().UnixNano(),
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := m.runJob(ctx, job); err != nil {
		t.Fatalf("run job: %v", err)
	}

	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	afterHash := sha256.Sum256(after)
	if beforeHash != afterHash {
		t.Fatal("source bytes changed")
	}
	staged := filepath.Join(libRoot, ".sonder-optimization-staging", "Test Author", "Test Book", "Test Book.opus-32k-r1.m4b")
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged output missing: %v", err)
	}
	receiptData, err := os.ReadFile(staged + ".receipt.json")
	if err != nil {
		t.Fatalf("receipt missing: %v", err)
	}
	var receipt Receipt
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.OutputBytes >= receipt.SourceBytes || receipt.SavedBytes <= 0 {
		t.Fatalf("expected measured reduction, got %+v", receipt)
	}
	if receipt.Validation.AudioCodec != "opus" || !receipt.Validation.FullDecode || !receipt.Validation.CoverPresent {
		t.Fatalf("validation receipt incomplete: %+v", receipt.Validation)
	}
	if receipt.Validation.ChapterCount != 2 || !receipt.Validation.ChaptersPreserved || !receipt.Validation.MetadataPreserved {
		t.Fatalf("chapter or metadata validation incomplete: %+v", receipt.Validation)
	}
	outputHash, err := fileSHA256(staged)
	if err != nil {
		t.Fatal(err)
	}
	if outputHash != receipt.OutputSHA256 {
		t.Fatal("staged file hash differs from receipt")
	}
	if _, err := os.Stat(filepath.Join(m.root, "work", job.ID)); !os.IsNotExist(err) {
		t.Fatal("local scratch was not cleaned after success")
	}
	if _, err := hex.DecodeString(receipt.SourceSHA256); err != nil {
		t.Fatalf("bad source checksum: %v", err)
	}
}

func TestRunJobRequiresAndUsesApprovedCatalogCover(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	root := t.TempDir()
	libRoot := filepath.Join(root, "library")
	bookDir := filepath.Join(libRoot, "Author", "Uncovered Book")
	if err := os.MkdirAll(bookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(bookDir, "Uncovered Book.m4b")
	cover := filepath.Join(bookDir, "cover.jpg")
	commands := [][]string{
		{ffmpeg, "-hide_banner", "-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "sine=frequency=330:duration=30", "-c:a", "aac", "-b:a", "160k", "-metadata", "title=Uncovered Book", "-f", "mp4", source},
		{ffmpeg, "-hide_banner", "-v", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "color=c=blue:s=96x96", "-frames:v", "1", cover},
	}
	for _, args := range commands {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("fixture command: %v: %s", err, out)
		}
	}
	st, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{root: filepath.Join(root, "data", "audiobook-optimization"), ffmpeg: ffmpeg, ffprobe: ffprobe}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		t.Fatal(err)
	}
	job := diskJob{Job: Job{ID: "approved-cover-job", Title: "Uncovered Book", SourceBytes: st.Size(), BitrateKbps: 32, Attempt: 1},
		SourcePath: source, LibraryRoot: libRoot, RelativePath: filepath.Join("Author", "Uncovered Book", "Uncovered Book.m4b"),
		SourceMtimeNS: st.ModTime().UnixNano(), UseCatalogCover: true, CatalogCoverPath: cover}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := m.runJob(ctx, job); err != nil {
		t.Fatalf("run job with approved cover: %v", err)
	}
	staged := filepath.Join(libRoot, ".sonder-optimization-staging", "Author", "Uncovered Book", "Uncovered Book.opus-32k-r1.m4b")
	receiptBytes, err := os.ReadFile(staged + ".receipt.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt Receipt
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.CoverSource != "user-approved catalog artwork" || !receipt.Validation.CoverPresent {
		t.Fatalf("cover approval missing from receipt: %+v", receipt)
	}
}
