package audiobookopt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

type MP3Status struct {
	Eligible           bool    `json:"eligible"`
	Reason             string  `json:"reason,omitempty"`
	PartCount          int     `json:"partCount"`
	NeedsCoverApproval bool    `json:"needsCoverApproval"`
	TargetFormat       string  `json:"targetFormat"`
	TargetCodec        string  `json:"targetCodec"`
	Job                *MP3Job `json:"job,omitempty"`
}
type MP3Job struct {
	ID                      string    `json:"id"`
	ItemID                  string    `json:"itemID"`
	Status                  string    `json:"status"`
	Phase                   string    `json:"phase,omitempty"`
	Error                   string    `json:"error,omitempty"`
	CreatedAt               time.Time `json:"createdAt"`
	DurationSeconds         float64   `json:"durationSeconds,omitempty"`
	ChapterCount            int       `json:"chapterCount,omitempty"`
	CoverSource             string    `json:"coverSource,omitempty"`
	ChapterSource           string    `json:"chapterSource,omitempty"`
	ChapterSourceURL        string    `json:"chapterSourceURL,omitempty"`
	ChapterReviewImportedAt time.Time `json:"chapterReviewImportedAt,omitempty"`
	FullDecode              bool      `json:"fullDecode"`
}
type MP3ChapterReview struct {
	Chapters   []api.AudiobookChapter
	SourceURL  string
	ImportedAt time.Time
}
type mp3Record struct {
	MP3Job
	Parts            []*library.Item        `json:"parts"`
	CoverPath        string                 `json:"coverPath,omitempty"`
	ReviewedChapters []api.AudiobookChapter `json:"reviewedChapters,omitempty"`
}

// MP3Manager keeps source files untouched and processes one conversion at a time.
type MP3Manager struct {
	mu        sync.Mutex
	store     *library.Store
	libraries []config.Library
	root      string
	probe     *Manager
	jobs      []mp3Record
	wake      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewMP3(store *library.Store, libs []config.Library, dataDir, ffmpeg, ffprobe string) (*MP3Manager, error) {
	if store == nil || dataDir == "" || ffmpeg == "" || ffprobe == "" {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &MP3Manager{store: store, libraries: libs, root: filepath.Join(dataDir, "audiobook-m4b"), probe: &Manager{ffmpeg: ffmpeg, ffprobe: ffprobe}, wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel, done: make(chan struct{}), jobs: []mp3Record{}}
	if err := os.MkdirAll(m.root, 0700); err != nil {
		cancel()
		return nil, err
	}
	if b, err := os.ReadFile(filepath.Join(m.root, "jobs.json")); err == nil {
		if err = json.Unmarshal(b, &m.jobs); err != nil {
			cancel()
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		cancel()
		return nil, err
	}
	for i := range m.jobs {
		if m.jobs[i].Status == "encoding" {
			m.jobs[i].Status = "interrupted"
			m.jobs[i].Error = "Server restarted; start a new conversion"
			_ = os.RemoveAll(filepath.Join(m.root, m.jobs[i].ID))
		}
	}
	if err := m.persist(); err != nil {
		cancel()
		return nil, err
	}
	go m.worker()
	return m, nil
}
func (m *MP3Manager) parts(id string) ([]*library.Item, error) {
	item, ok := m.store.Get(id)
	if !ok || item.Kind != api.KindAudiobook {
		return nil, ErrNotCandidate
	}
	groups, conflicts := library.BookGroups(m.store.InternalItems(), m.libraries)
	for _, conflict := range conflicts {
		for _, excluded := range conflict.ExcludedIDs {
			if excluded == id {
				return nil, errors.New("Book has conflicting or duplicate files; resolve its parts before conversion")
			}
		}
	}
	group, grouped := groups[id]
	parts := []*library.Item{item}
	if grouped {
		parts = nil
		for _, it := range m.store.InternalItemsOfKind(api.KindAudiobook) {
			if g, ok := groups[it.ID]; ok && g.ID == group.ID {
				parts = append(parts, it)
			}
		}
		sort.Slice(parts, func(i, j int) bool { return groups[parts[i].ID].Index < groups[parts[j].ID].Index })
	}
	for _, it := range parts {
		if !strings.EqualFold(filepath.Ext(it.FilePath), ".mp3") {
			return nil, errors.New("Only books whose ordered parts are all MP3 can be converted")
		}
		if _, _, err := (&Manager{libraries: m.libraries}).libraryFor(it.FilePath); err != nil {
			return nil, err
		}
	}
	return parts, nil
}
func (m *MP3Manager) Status(id string) MP3Status {
	parts, err := m.parts(id)
	s := MP3Status{TargetFormat: "m4b", TargetCodec: "aac", PartCount: len(parts), Eligible: err == nil}
	if err != nil {
		s.Reason = err.Error()
	} else {
		s.NeedsCoverApproval = !parts[0].ProbedHasCover && parts[0].PosterPath != "" && parts[0].PosterSource != "thumbnail"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.jobs) - 1; i >= 0; i-- {
		for _, p := range m.jobs[i].Parts {
			if p.ID == id {
				j := m.jobs[i].MP3Job
				s.Job = &j
				return s
			}
		}
	}
	return s
}
func (m *MP3Manager) Enqueue(id string, approveCover bool, reviewed ...MP3ChapterReview) (MP3Job, error) {
	parts, err := m.parts(id)
	if err != nil {
		return MP3Job{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		for _, p := range j.Parts {
			for _, n := range parts {
				if p.ID == n.ID && (j.Status == "queued" || j.Status == "encoding" || j.Status == "ready") {
					return MP3Job{}, ErrConflict
				}
			}
		}
	}
	if len(m.jobs) >= 200 {
		return MP3Job{}, errors.New("Conversion history is full")
	}
	for _, p := range parts {
		st, err := os.Stat(p.FilePath)
		if err != nil {
			return MP3Job{}, errors.New("Source is unavailable")
		}
		if st.Size() != p.SizeBytes || (!p.ModTime.IsZero() && !st.ModTime().Equal(p.ModTime)) {
			return MP3Job{}, errors.New("Source changed; rescan before converting")
		}
	}
	jobID, err := newID()
	if err != nil {
		return MP3Job{}, err
	}
	r := mp3Record{MP3Job: MP3Job{ID: jobID, ItemID: id, Status: "queued", CreatedAt: time.Now().UTC()}, Parts: parts}
	if len(reviewed) > 0 {
		r.ReviewedChapters = append([]api.AudiobookChapter(nil), reviewed[0].Chapters...)
		r.ChapterSourceURL = reviewed[0].SourceURL
		r.ChapterReviewImportedAt = reviewed[0].ImportedAt
	}
	if approveCover && parts[0].PosterPath != "" && parts[0].PosterSource != "thumbnail" {
		r.CoverPath = parts[0].PosterPath
	}
	m.jobs = append(m.jobs, r)
	if err := m.persist(); err != nil {
		m.jobs = m.jobs[:len(m.jobs)-1]
		return MP3Job{}, err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return r.MP3Job, nil
}
func (m *MP3Manager) persist() error {
	b, err := json.MarshalIndent(m.jobs, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(m.root, ".jobs-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(m.root, "jobs.json"))
}
func (m *MP3Manager) Close(ctx context.Context) error {
	m.cancel()
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *MP3Manager) worker() {
	defer close(m.done)
	for {
		if m.ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		index := -1
		var r mp3Record
		for i := range m.jobs {
			if m.jobs[i].Status == "queued" {
				index = i
				m.jobs[i].Status = "encoding"
				m.jobs[i].Phase = "Inspecting source files"
				r = m.jobs[i]
				_ = m.persist()
				break
			}
		}
		m.mu.Unlock()
		if index < 0 {
			select {
			case <-m.ctx.Done():
				return
			case <-m.wake:
				continue
			}
		}
		err := m.convert(m.ctx, &r)
		m.mu.Lock()
		if err != nil {
			r.Status = "failed"
			r.Phase = "Conversion failed; originals retained"
			r.Error = "Conversion failed; originals retained"
			if m.ctx.Err() != nil {
				r.Status = "interrupted"
			}
		} else {
			r.Status = "ready"
			r.Phase = "Verified; ready to download"
		}
		m.jobs[index] = r
		_ = m.persist()
		m.mu.Unlock()
	}
}
func (m *MP3Manager) Open(id string) (*os.File, error) {
	s := m.Status(id)
	if s.Job == nil || s.Job.Status != "ready" {
		return nil, ErrNotFound
	}
	return os.Open(filepath.Join(m.root, s.Job.ID, "book.m4b"))
}
func ffEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "=", "\\=", ";", "\\;", "#", "\\#", "\n", " ", "\r", " ")
	return r.Replace(s)
}
func (m *MP3Manager) convert(ctx context.Context, r *mp3Record) error {
	work := filepath.Join(m.root, r.ID)
	if err := os.Mkdir(work, 0700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(work)
		}
	}()
	var duration float64
	channels := 1
	sourceProbes := make([]mediaProbe, len(r.Parts))
	var estimatedDuration float64
	for i, p := range r.Parts {
		pr, err := m.probe.inspect(ctx, p.FilePath)
		if err != nil {
			return err
		}
		a := audioStreams(pr)
		if len(a) != 1 || a[0].Codec != "mp3" || a[0].Channels < 1 || a[0].Channels > 2 || hasMovingVideo(pr) {
			return errors.New("unsupported source")
		}
		if a[0].Channels > channels {
			channels = a[0].Channels
		}
		sourceProbes[i] = pr
		estimatedDuration += parseFloat(pr.Format.Duration)
	}
	required := int64(estimatedDuration*(44100*float64(channels)*2+16000)) + 128*1024*1024
	if free, err := freeBytes(work); err != nil || free < required {
		return errors.New("insufficient space for decoded staging and final M4B")
	}
	type boundary struct{ sourceStart, sourceDuration, decodedStart, decodedDuration float64 }
	boundaries := []boundary{}
	var sourceDuration float64
	var sourceTags map[string]string
	chapters := []struct {
		title      string
		start, end float64
	}{}
	var list strings.Builder
	cover := ""
	r.ChapterSource = "embedded chapters and source-file boundaries"
	for i, p := range r.Parts {
		st, err := os.Stat(p.FilePath)
		if err != nil || st.Size() != p.SizeBytes || (!p.ModTime.IsZero() && !st.ModTime().Equal(p.ModTime)) {
			return errors.New("source changed")
		}
		pr := sourceProbes[i]
		if i == 0 {
			sourceTags = pr.Format.Tags
		}
		a := audioStreams(pr)
		if len(a) != 1 || a[0].Codec != "mp3" || hasMovingVideo(pr) {
			return errors.New("unsupported source")
		}
		d := parseFloat(pr.Format.Duration)
		if d <= 0 {
			return errors.New("missing duration")
		}
		if i == 0 && attachedPictureIndex(pr) >= 0 {
			cover = filepath.Join(work, "cover.jpg")
			if err := runCommand(ctx, m.probe.ffmpeg, "-v", "error", "-nostdin", "-i", p.FilePath, "-map", fmt.Sprintf("0:%d", attachedPictureIndex(pr)), "-frames:v", "1", cover); err != nil {
				return err
			}
			r.CoverSource = "embedded source artwork"
		}
		// Normalize lossless PCM once, then encode the whole book once to avoid per-track AAC priming.
		m.phase(r.ID, fmt.Sprintf("Decoding source %d of %d", i+1, len(r.Parts)))
		audio := filepath.Join(work, fmt.Sprintf("part-%06d.wav", i))
		if err := runCommand(ctx, m.probe.ffmpeg, "-v", "error", "-xerror", "-nostdin", "-i", p.FilePath, "-map", "0:a:0", "-vn", "-c:a", "pcm_s16le", "-ar", "44100", "-ac", fmt.Sprint(channels), audio); err != nil {
			return err
		}
		decoded, err := m.probe.inspect(ctx, audio)
		if err != nil {
			return err
		}
		actual := parseFloat(decoded.Format.Duration)
		if actual <= 0 || abs(actual-d) > 0.25 {
			return errors.New("decoded source duration differs unexpectedly")
		}
		boundaries = append(boundaries, boundary{sourceDuration, d, duration, actual})
		sourceDuration += d
		if len(pr.Chapters) > 0 {
			for _, c := range pr.Chapters {
				start, end := parseFloat(c.Start), parseFloat(c.End)
				if start < 0 || end <= start || end > actual+0.25 {
					return errors.New("invalid embedded chapters")
				}
				chapters = append(chapters, struct {
					title      string
					start, end float64
				}{c.Tags["title"], duration + start, duration + min(end, actual)})
			}
		} else {
			title := strings.TrimSuffix(filepath.Base(p.FilePath), filepath.Ext(p.FilePath))
			chapters = append(chapters, struct {
				title      string
				start, end float64
			}{title, duration, duration + actual})
		}
		// Record observed source evidence before encoding; no external timing is guessed.
		evidence := map[string]any{"sourcePartID": p.ID, "durationSeconds": d, "decodedDurationSeconds": actual, "sourceTags": pr.Format.Tags, "sourceAudioStreams": audioStreams(pr), "targetChannels": channels, "chapterCount": len(pr.Chapters), "chapters": pr.Chapters, "embeddedCover": attachedPictureIndex(pr) >= 0, "decision": "preserve-source", "externalEditionMatch": "not-run", "whisperAlignment": "not-run", "reviewedChapterSourceURL": r.ChapterSourceURL, "reviewedChapterImportedAt": r.ChapterReviewImportedAt}
		preflight, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, fmt.Sprintf("preflight-%06d.json", i)), preflight, 0600); err != nil {
			return err
		}
		fmt.Fprintf(&list, "file '%s'\n", filepath.Base(audio))
		duration += actual

	}
	if len(r.ReviewedChapters) > 0 {
		chapters = nil
		mapTime := func(t float64) float64 {
			for _, b := range boundaries {
				if t <= b.sourceStart+b.sourceDuration+0.001 {
					return b.decodedStart + min(max(t-b.sourceStart, 0), b.decodedDuration)
				}
			}
			return duration
		}
		var previous float64
		for i, c := range r.ReviewedChapters {
			if c.EndSeconds == nil || c.StartSeconds < 0 || *c.EndSeconds <= c.StartSeconds || *c.EndSeconds > sourceDuration+0.25 || (i > 0 && c.StartSeconds < previous) {
				return errors.New("reviewed chapter map does not match source runtime")
			}
			chapters = append(chapters, struct {
				title      string
				start, end float64
			}{c.Title, mapTime(c.StartSeconds), mapTime(*c.EndSeconds)})
			previous = *c.EndSeconds
		}
		r.ChapterSource = "reviewed edition-matched chapter map"
	}
	if cover == "" && r.CoverPath != "" {
		cover = filepath.Join(work, "cover.jpg")
		if err := runCommand(ctx, m.probe.ffmpeg, "-v", "error", "-nostdin", "-i", r.CoverPath, "-frames:v", "1", cover); err != nil {
			return err
		}
		r.CoverSource = "approved catalog artwork"
	}
	if cover == "" {
		r.CoverSource = "none; source has no embedded cover"
	}
	author := ""
	if r.Parts[0].Author != nil {
		author = *r.Parts[0].Author
	}
	meta := fmt.Sprintf(";FFMETADATA1\ntitle=%s\nartist=%s\nalbum=%s\n", ffEscape(r.Parts[0].Title), ffEscape(author), ffEscape(r.Parts[0].Title))
	for key, value := range sourceTags {
		if key == "title" || key == "artist" || key == "album" || key == "encoder" || key == "major_brand" || key == "minor_version" || key == "compatible_brands" {
			continue
		}
		meta += ffEscape(key) + "=" + ffEscape(value) + "\n"
	}
	for _, c := range chapters {
		meta += fmt.Sprintf("[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n", int64(c.start*1000), int64(c.end*1000), ffEscape(c.title))
	}
	if err := os.WriteFile(filepath.Join(work, "metadata.txt"), []byte(meta), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(work, "parts.txt"), []byte(list.String()), 0600); err != nil {
		return err
	}
	args := []string{"-v", "error", "-nostdin", "-f", "concat", "-safe", "1", "-i", filepath.Join(work, "parts.txt"), "-f", "ffmetadata", "-i", filepath.Join(work, "metadata.txt")}
	if cover != "" {
		args = append(args, "-i", cover)
	}
	args = append(args, "-map", "0:a:0", "-map_metadata", "1", "-map_chapters", "1", "-c:a", "aac", "-b:a", "128k", "-ar", "44100", "-ac", fmt.Sprint(channels))
	if cover != "" {
		args = append(args, "-map", "2:v:0", "-c:v", "copy", "-disposition:v:0", "attached_pic")
	}
	out := filepath.Join(work, "book.m4b")
	args = append(args, "-movflags", "+faststart+use_metadata_tags", "-f", "mp4", out)
	if free, err := freeBytes(work); err != nil || free < int64(duration*16000)+128*1024*1024 {
		return errors.New("insufficient output space")
	}
	m.phase(r.ID, "Encoding the complete book to AAC")
	if err := runCommand(ctx, m.probe.ffmpeg, args...); err != nil {
		return err
	}
	m.phase(r.ID, "Validating chapters, artwork and full audio decode")
	dst, err := m.probe.inspect(ctx, out)
	if err != nil {
		return err
	}
	a := audioStreams(dst)
	if len(a) != 1 || a[0].Codec != "aac" || len(dst.Chapters) != len(chapters) {
		return errors.New("output validation failed")
	}
	od := parseFloat(dst.Format.Duration)
	if abs(od-duration) > 0.05 {
		return errors.New("duration validation failed")
	}
	for i, c := range dst.Chapters {
		if c.Tags["title"] != chapters[i].title || abs(parseFloat(c.Start)-chapters[i].start) > 0.05 || abs(parseFloat(c.End)-chapters[i].end) > 0.05 {
			return errors.New("chapter validation failed")
		}
	}
	if cover != "" && attachedPictureIndex(dst) < 0 {
		return errors.New("cover validation failed")
	}
	if dst.Format.Tags["title"] != r.Parts[0].Title || dst.Format.Tags["artist"] != author {
		return errors.New("metadata validation failed")
	}
	for key, value := range sourceTags {
		if key == "title" || key == "artist" || key == "album" || key == "encoder" || key == "major_brand" || key == "minor_version" || key == "compatible_brands" {
			continue
		}
		if dst.Format.Tags[key] != value {
			return errors.New("source tag validation failed")
		}
	}
	if err := runCommand(ctx, m.probe.ffmpeg, "-v", "error", "-xerror", "-nostdin", "-i", out, "-map", "0:a:0", "-f", "null", "-"); err != nil {
		return err
	}
	for _, p := range r.Parts {
		st, err := os.Stat(p.FilePath)
		if err != nil || st.Size() != p.SizeBytes || (!p.ModTime.IsZero() && !st.ModTime().Equal(p.ModTime)) {
			return errors.New("source changed during conversion")
		}
	}
	r.DurationSeconds = od
	r.ChapterCount = len(chapters)
	r.FullDecode = true
	receipt, _ := json.MarshalIndent(r.MP3Job, "", "  ")
	if err := os.WriteFile(filepath.Join(work, "receipt.json"), receipt, 0600); err != nil {
		return err
	}
	entries, _ := os.ReadDir(work)
	for _, e := range entries {
		if e.Name() != "book.m4b" && e.Name() != "receipt.json" && !strings.HasPrefix(e.Name(), "preflight-") {
			_ = os.Remove(filepath.Join(work, e.Name()))
		}
	}
	success = true
	return nil
}

func (m *MP3Manager) phase(id, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.jobs {
		if m.jobs[i].ID == id {
			m.jobs[i].Phase = value
			_ = m.persist()
			return
		}
	}
}
