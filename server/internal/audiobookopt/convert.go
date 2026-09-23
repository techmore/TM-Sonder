package audiobookopt

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type mediaStream struct {
	Index       int    `json:"index"`
	Type        string `json:"codec_type"`
	Codec       string `json:"codec_name"`
	CodecTag    string `json:"codec_tag_string"`
	Channels    int    `json:"channels"`
	Duration    string `json:"duration"`
	Bitrate     string `json:"bit_rate"`
	Disposition struct {
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
}

type mediaProbe struct {
	Streams  []mediaStream `json:"streams"`
	Chapters []struct {
		Start string            `json:"start_time"`
		End   string            `json:"end_time"`
		Tags  map[string]string `json:"tags"`
	} `json:"chapters"`
	Format struct {
		Name     string            `json:"format_name"`
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func (m *Manager) runJob(ctx context.Context, job diskJob) error {
	work := filepath.Join(m.root, "work", job.ID)
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		return err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(work)
		}
	}()

	before, err := os.Stat(job.SourcePath)
	if err != nil {
		return fmt.Errorf("source unavailable: %w", err)
	}
	if before.Size() != job.SourceBytes || before.ModTime().UnixNano() != job.SourceMtimeNS {
		return errors.New("source changed since it was queued; refresh the library and queue it again")
	}
	returnRoot := filepath.Join(job.LibraryRoot, ".sonder-optimization-staging")
	rel := filepath.FromSlash(job.RelativePath)
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)) + fmt.Sprintf(".opus-%dk-r%d.m4b", job.BitrateKbps, job.Attempt)
	stagedRel := filepath.Join(".sonder-optimization-staging", filepath.Dir(rel), base)
	destination := filepath.Join(job.LibraryRoot, stagedRel)
	if err := ensureInside(returnRoot, destination); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("staging destination already exists; refusing to overwrite")
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(destination + ".receipt.json"); err == nil {
		return errors.New("staging receipt already exists; refusing to overwrite")
	} else if !os.IsNotExist(err) {
		return err
	}
	localSource := filepath.Join(work, "source.m4b")
	sourceHash, err := copyAndHash(ctx, job.SourcePath, localSource, func(done, total int64) {
		pct := 5.0
		if total > 0 {
			pct += float64(done) * 15 / float64(total)
		}
		m.update(job.ID, StatusCopying, "Copying source to local workspace", pct, done, nil)
	})
	if err != nil {
		return fmt.Errorf("copy source locally: %w", err)
	}
	after, err := os.Stat(job.SourcePath)
	if err != nil || after.Size() != before.Size() || after.ModTime().UnixNano() != before.ModTime().UnixNano() {
		return errors.New("source changed while it was copied; no output was produced")
	}
	inputProbe, err := m.inspect(ctx, localSource)
	if err != nil {
		return fmt.Errorf("probe local source: %w", err)
	}
	audio := audioStreams(inputProbe)
	if len(audio) != 1 || strings.ToLower(audio[0].Codec) != "aac" {
		return errors.New("source is no longer a single AAC audio stream")
	}
	if audio[0].Channels < 1 || audio[0].Channels > 2 {
		return errors.New("only mono and stereo audiobooks are supported")
	}
	if hasMovingVideo(inputProbe) {
		return errors.New("source contains non-cover video and needs manual review")
	}
	for _, stream := range inputProbe.Streams {
		chapterTextTrack := stream.Type == "data" && stream.CodecTag == "text" && len(inputProbe.Chapters) > 0
		if stream.Type == "subtitle" || (stream.Type != "audio" && stream.Type != "video" && !chapterTextTrack) {
			return errors.New("source has subtitle or other non-audio streams that are not preserved")
		}
	}
	coverIndex := attachedPictureIndex(inputProbe)
	coverInput := localSource
	coverMap := "0:" + fmt.Sprint(coverIndex)
	if coverIndex < 0 {
		if !job.UseCatalogCover || job.CatalogCoverPath == "" {
			return errors.New("no embedded cover; explicitly approve catalog artwork before queueing")
		}
		if st, err := os.Stat(job.CatalogCoverPath); err != nil || st.IsDir() {
			return errors.New("approved catalog cover is unavailable")
		}
		coverInput = filepath.Join(work, "approved-cover-source")
		if _, err := copyAndHash(ctx, job.CatalogCoverPath, coverInput, func(int64, int64) {}); err != nil {
			return fmt.Errorf("copy approved cover to local workspace: %w", err)
		}
		coverMap = "0:v:0"
	}
	duration := parseFloat(inputProbe.Format.Duration)
	if duration <= 0 {
		return errors.New("source duration is missing")
	}
	if err := m.ensureLocalSpace(job.SourceBytes); err != nil {
		return err
	}

	audioOnly := filepath.Join(work, "audio.mp4")
	output := filepath.Join(work, "verified.m4b")
	update := func(outSeconds float64) {
		p := 20 + outSeconds/duration*55
		if p > 75 {
			p = 75
		}
		if p < 20 {
			p = 20
		}
		m.update(job.ID, StatusEncoding, "Encoding Opus locally", p, int64(outSeconds*float64(job.BitrateKbps)*125), nil)
	}
	m.update(job.ID, StatusEncoding, "Encoding Opus locally", 20, 0, nil)
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y", "-i", localSource,
		"-map", "0:a:0", "-map_metadata", "0", "-map_chapters", "0", "-vn",
		"-c:a", "libopus", "-b:a", fmt.Sprintf("%dk", job.BitrateKbps), "-vbr", "on",
		"-application", "audio", "-compression_level", "10", "-f", "mp4",
		"-progress", "pipe:1", "-nostats", audioOnly}
	if err := runProgress(ctx, m.ffmpeg, args, update); err != nil {
		return fmt.Errorf("encode audio: %w", err)
	}

	cover := filepath.Join(work, "cover.jpg")
	if coverIndex >= 0 {
		if err := runCommand(ctx, m.ffmpeg, "-hide_banner", "-v", "error", "-nostdin", "-y", "-i", localSource, "-map", coverMap, "-frames:v", "1", cover); err != nil {
			return fmt.Errorf("extract embedded cover: %w", err)
		}
	} else if err := runCommand(ctx, m.ffmpeg, "-hide_banner", "-v", "error", "-nostdin", "-y", "-i", coverInput, "-map", coverMap, "-frames:v", "1", cover); err != nil {
		return fmt.Errorf("prepare approved cover: %w", err)
	}
	remuxArgs := []string{"-hide_banner", "-v", "error", "-nostdin", "-y", "-i", audioOnly, "-i", cover,
		"-map", "0:a:0", "-map", "1:v:0", "-map_metadata", "0", "-map_chapters", "0",
		"-c", "copy", "-disposition:v:0", "attached_pic", "-f", "mp4", "-movflags", "+faststart", output}
	if err := runCommand(ctx, m.ffmpeg, remuxArgs...); err != nil {
		return fmt.Errorf("attach cover and chapters: %w", err)
	}

	m.update(job.ID, StatusValidating, "Checking container, chapters, cover, metadata, and full decode", 80, job.SourceBytes, nil)
	outProbe, err := m.inspect(ctx, output)
	if err != nil {
		return fmt.Errorf("probe output: %w", err)
	}
	validation, err := validate(inputProbe, outProbe, audio[0].Channels)
	if err != nil {
		return err
	}
	if err := runCommand(ctx, m.ffmpeg, "-hide_banner", "-v", "error", "-xerror", "-nostdin", "-i", output, "-map", "0:a:0", "-f", "null", "-"); err != nil {
		return fmt.Errorf("full output audio decode failed: %w", err)
	}
	validation.FullDecode = true
	staged, err := os.Stat(output)
	if err != nil {
		return err
	}
	outputHash, err := fileSHA256(output)
	if err != nil {
		return err
	}
	// The source was fingerprinted while copied; stat it again without a second
	// full NAS read. The receipt hashes the exact bytes that were encoded.
	finalSource, err := os.Stat(job.SourcePath)
	if err != nil || finalSource.Size() != before.Size() || finalSource.ModTime().UnixNano() != before.ModTime().UnixNano() {
		return errors.New("source changed during conversion")
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create NAS review folder: %w", err)
	}
	if _, err := os.Lstat(destination); err == nil {
		return errors.New("staging destination already exists; refusing to overwrite")
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Lstat(destination + ".receipt.json"); err == nil {
		return errors.New("staging receipt already exists; refusing to overwrite")
	} else if !os.IsNotExist(err) {
		return err
	}
	remoteFree, err := freeBytes(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("check NAS review staging space: %w", err)
	}
	if remoteFree < staged.Size()+1024*1024 {
		return fmt.Errorf("insufficient space in NAS review staging: need at least %d bytes, have %d", staged.Size()+1024*1024, remoteFree)
	}
	m.update(job.ID, StatusReturning, "Copying verified output to hidden NAS review staging", 90, 0, nil)
	copyTmp, err := os.CreateTemp(filepath.Dir(destination), ".sonder-copy-*.partial")
	if err != nil {
		return fmt.Errorf("create NAS temporary output: %w", err)
	}
	if err := copyTmp.Chmod(0o644); err != nil {
		copyTmp.Close()
		return err
	}
	tmpName := copyTmp.Name()
	defer os.Remove(tmpName)
	transferredHash, err := copyToWriter(ctx, output, copyTmp, func(done, total int64) {
		pct := 90.0
		if total > 0 {
			pct += float64(done) * 9 / float64(total)
		}
		m.update(job.ID, StatusReturning, "Copying verified output to hidden NAS review staging", pct, done, nil)
	})
	if err != nil {
		copyTmp.Close()
		return fmt.Errorf("copy verified output back: %w", err)
	}
	if err := copyTmp.Sync(); err != nil {
		copyTmp.Close()
		return err
	}
	if err := copyTmp.Close(); err != nil {
		return err
	}
	if transferredHash != outputHash {
		return errors.New("NAS copy checksum mismatch")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(tmpName, destination); err != nil {
		return fmt.Errorf("publish verified result to review staging: %w", err)
	}
	if err := os.Remove(tmpName); err != nil {
		_ = os.Remove(destination)
		return err
	}
	outputBytes := staged.Size()
	receipt := &Receipt{
		CreatedAt: time.Now().UTC(), SourceBytes: before.Size(), OutputBytes: outputBytes,
		SavedBytes: before.Size() - outputBytes, SavedPercent: float64(before.Size()-outputBytes) * 100 / float64(before.Size()),
		SourceSHA256: sourceHash, OutputSHA256: outputHash, BitrateKbps: job.BitrateKbps,
		CoverSource: "embedded source cover",
		Validation:  validation, PlaybackReview: "pending human listening and target-device playback",
		StagedRelativePath: filepath.ToSlash(stagedRel),
	}
	if coverIndex < 0 {
		receipt.CoverSource = "user-approved catalog artwork"
	}
	if receipt.SavedBytes <= 0 {
		_ = os.Remove(destination)
		return errors.New("verified output is not smaller than its source")
	}
	if err := writeReceipt(destination+".receipt.json", receipt); err != nil {
		_ = os.Remove(destination)
		return fmt.Errorf("write review receipt: %w", err)
	}
	m.update(job.ID, StatusReturning, "Output and checksum receipt copied to NAS staging", 100, outputBytes, receipt)
	_ = os.RemoveAll(work)
	succeeded = true
	return nil
}

func (m *Manager) inspect(ctx context.Context, path string) (mediaProbe, error) {
	var out mediaProbe
	cmd := exec.CommandContext(ctx, m.ffprobe, "-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters", path)
	data, err := cmd.Output()
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func validate(src, dst mediaProbe, sourceChannels int) (Validation, error) {
	audio := audioStreams(dst)
	sourceAudio := audioStreams(src)
	if len(audio) != 1 || strings.ToLower(audio[0].Codec) != "opus" {
		return Validation{}, errors.New("output is not exactly one Opus audio stream")
	}
	if !strings.Contains(dst.Format.Name, "mp4") {
		return Validation{}, errors.New("output container is not MP4-family")
	}
	if audio[0].Channels != sourceChannels {
		return Validation{}, errors.New("output channel count differs from source")
	}
	if len(sourceAudio) != 1 {
		return Validation{}, errors.New("source audio stream count changed")
	}
	sourceDuration, outputDuration := parseFloat(src.Format.Duration), parseFloat(dst.Format.Duration)
	if sourceDuration <= 0 || outputDuration <= 0 || abs(sourceDuration-outputDuration) > 0.25 {
		return Validation{}, errors.New("output duration differs from source by more than 250 ms")
	}
	sourceAudioDuration, outputAudioDuration := parseFloat(sourceAudio[0].Duration), parseFloat(audio[0].Duration)
	if sourceAudioDuration > 0 && outputAudioDuration > 0 && abs(sourceAudioDuration-outputAudioDuration) > 0.25 {
		return Validation{}, errors.New("output audio stream duration differs from source by more than 250 ms")
	}
	if !hasAttachedPicture(dst) {
		return Validation{}, errors.New("output is missing embedded cover art")
	}
	if len(src.Chapters) != len(dst.Chapters) {
		return Validation{}, errors.New("chapter count changed")
	}
	for i := range src.Chapters {
		a, b := src.Chapters[i], dst.Chapters[i]
		if a.Tags["title"] != b.Tags["title"] || abs(parseFloat(a.Start)-parseFloat(b.Start)) > 0.1 || abs(parseFloat(a.End)-parseFloat(b.End)) > 0.1 {
			return Validation{}, fmt.Errorf("chapter %d changed", i+1)
		}
	}
	if !metadataEqual(src.Format.Tags, dst.Format.Tags) {
		return Validation{}, errors.New("common metadata fields changed")
	}
	return Validation{Container: dst.Format.Name, AudioCodec: strings.ToLower(audio[0].Codec), Channels: audio[0].Channels,
		DurationSeconds: outputDuration, ChapterCount: len(dst.Chapters), CoverPresent: true,
		ChaptersPreserved: true, MetadataPreserved: true}, nil
}

func metadataEqual(source, output map[string]string) bool {
	wanted := []string{"title", "artist", "album", "album_artist", "composer", "date", "genre", "comment", "track", "disc"}
	for _, key := range wanted {
		src := findTag(source, key)
		if src == "" {
			continue
		}
		if findTag(output, key) != src {
			return false
		}
	}
	return true
}

func findTag(tags map[string]string, key string) string {
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func audioStreams(p mediaProbe) []mediaStream {
	out := p.Streams[:0:0]
	for _, s := range p.Streams {
		if s.Type == "audio" {
			out = append(out, s)
		}
	}
	return out
}

func hasMovingVideo(p mediaProbe) bool {
	for _, s := range p.Streams {
		if s.Type == "video" && s.Disposition.AttachedPic == 0 {
			return true
		}
	}
	return false
}
func hasAttachedPicture(p mediaProbe) bool { return attachedPictureIndex(p) >= 0 }
func attachedPictureIndex(p mediaProbe) int {
	for _, s := range p.Streams {
		if s.Type == "video" && s.Disposition.AttachedPic != 0 {
			return s.Index
		}
	}
	return -1
}

func copyAndHash(ctx context.Context, source, destination string, progress func(int64, int64)) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	st, err := input.Stat()
	if err != nil {
		return "", err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer output.Close()
	h := sha256.New()
	buf := make([]byte, 1024*1024)
	var done int64
	last := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := input.Read(buf)
		if n > 0 {
			if _, err := h.Write(buf[:n]); err != nil {
				return "", err
			}
			if _, err := output.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if time.Since(last) > time.Second || done == st.Size() {
				progress(done, st.Size())
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err := output.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyToWriter(ctx context.Context, source string, output io.Writer, progress func(int64, int64)) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	st, err := input.Stat()
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writer := io.MultiWriter(output, h)
	buf := make([]byte, 1024*1024)
	var done int64
	last := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := input.Read(buf)
		if n > 0 {
			if _, err := writer.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if time.Since(last) > time.Second || done == st.Size() {
				progress(done, st.Size())
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runCommand(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 2500 {
			msg = msg[len(msg)-2500:]
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func runProgress(ctx context.Context, bin string, args []string, onProgress func(float64)) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var outTime float64
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_us=") {
			outTime = parseFloat(strings.TrimPrefix(line, "out_time_us=")) / 1_000_000
			onProgress(outTime)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return scanErr
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 2500 {
			msg = msg[len(msg)-2500:]
		}
		return fmt.Errorf("%w: %s", waitErr, msg)
	}
	return nil
}

func (m *Manager) ensureLocalSpace(sourceBytes int64) error {
	needed := sourceBytes*2 + 128*1024*1024
	free, err := freeBytes(m.root)
	if err != nil {
		return fmt.Errorf("check local workspace space: %w", err)
	}
	if free < needed {
		return fmt.Errorf("insufficient local workspace space: need %d bytes, have %d", needed, free)
	}
	return nil
}

func writeReceipt(path string, receipt *Receipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".receipt-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Link(tmp, path); err != nil {
		return err
	}
	return os.Remove(tmp)
}

func replaceReceipt(path string, receipt *Receipt) error {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".receipt-update-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ensureInside(root, dest string) error {
	r, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	d, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(r, d)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("staging destination escaped its hidden review directory")
	}
	return nil
}

func parseFloat(s string) float64 { var n float64; _, _ = fmt.Sscanf(s, "%f", &n); return n }
func abs(n float64) float64 {
	if n < 0 {
		return -n
	}
	return n
}
