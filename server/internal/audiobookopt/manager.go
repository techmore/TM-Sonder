package audiobookopt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

var (
	ErrUnavailable  = errors.New("audiobook optimization is unavailable")
	ErrNotCandidate = errors.New("item is not an eligible audiobook candidate")
	ErrConflict     = errors.New("an optimization job already exists for this item")
	ErrNotFound     = errors.New("optimization job not found")
)

type Manager struct {
	mu           sync.Mutex
	store        *library.Store
	libraries    []config.Library
	root         string
	statePath    string
	ffmpeg       string
	ffprobe      string
	state        diskState
	wake         chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	activeCancel context.CancelFunc
	done         chan struct{}
	closed       bool
}

func New(store *library.Store, libraries []config.Library, dataDir, ffmpeg, ffprobe string) (*Manager, error) {
	if store == nil || dataDir == "" || ffmpeg == "" || ffprobe == "" {
		return nil, ErrUnavailable
	}
	root := filepath.Join(dataDir, "audiobook-optimization")
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o700); err != nil {
		return nil, fmt.Errorf("create audiobook optimization workspace: %w", err)
	}
	m := &Manager{
		store: store, libraries: append([]config.Library(nil), libraries...), root: root,
		statePath: filepath.Join(root, "queue.json"), ffmpeg: ffmpeg, ffprobe: ffprobe,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	go m.worker()
	return m, nil
}

func (m *Manager) SetLibraries(libraries []config.Library) {
	m.mu.Lock()
	m.libraries = append([]config.Library(nil), libraries...)
	m.mu.Unlock()
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.statePath)
	if errors.Is(err, os.ErrNotExist) {
		m.state = diskState{Jobs: []diskJob{}, UpdatedAt: time.Now().UTC()}
		return m.persistLocked()
	}
	if err != nil {
		return fmt.Errorf("read optimization queue: %w", err)
	}
	if err := json.Unmarshal(data, &m.state); err != nil {
		return fmt.Errorf("decode optimization queue: %w", err)
	}
	if m.state.Jobs == nil {
		m.state.Jobs = []diskJob{}
	}
	// A process restart cannot safely continue a partial copy or encode. Keep
	// the record visible and retryable; a retry starts from a clean workspace.
	var staleWork []string
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		switch j.Status {
		case StatusCopying, StatusEncoding, StatusValidating, StatusReturning:
			staleWork = append(staleWork, filepath.Join(m.root, "work", j.ID))
			j.Status = StatusInterrupted
			j.Phase = "Interrupted by server restart; retry to restart safely"
			j.Progress = 0
			j.UpdatedAt = time.Now().UTC()
		}
	}
	for _, path := range staleWork {
		_ = os.RemoveAll(path)
	}
	return m.persistLocked()
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	jobs := make([]Job, len(m.state.Jobs))
	for i, j := range m.state.Jobs {
		jobs[i] = j.Job
		// Keep the queue visually tied to the catalog item. The poster endpoint
		// reads the same item record and remains valid when the source is promoted.
		if item, ok := m.store.Get(j.ItemID); ok && item.PosterPath != "" {
			jobs[i].PosterURL = "/artwork/poster/" + j.ItemID
		}
		if j.Receipt != nil {
			receiptCopy := *j.Receipt
			jobs[i].Receipt = &receiptCopy
		}
		jobs[i].Error = strings.ReplaceAll(jobs[i].Error, m.root, "<local workspace>")
		jobs[i].Error = strings.ReplaceAll(jobs[i].Error, j.SourcePath, "<source audiobook>")
		jobs[i].Error = strings.ReplaceAll(jobs[i].Error, j.LibraryRoot, "<audiobook library>")
	}
	// Preserve the manager's order so queue prioritization is visible to clients.
	out := Snapshot{Paused: m.state.Paused, UpdatedAt: m.state.UpdatedAt, Jobs: jobs}
	for _, j := range m.state.Jobs {
		if j.Status == StatusCopying || j.Status == StatusEncoding || j.Status == StatusValidating || j.Status == StatusReturning {
			out.ActiveJobID = j.ID
			break
		}
	}
	return out
}

// Enqueue accepts only items in the server's AAC-to-Opus recommendations. A
// sidecar cover is used only after an explicit UI acknowledgement.
func (m *Manager) Enqueue(ids []string, approvedCoverIDs map[string]bool, bitrateOverrides map[string]int) ([]Job, error) {
	if len(ids) == 0 || len(ids) > 50 {
		return nil, errors.New("select between 1 and 50 audiobooks")
	}
	recommended := map[string]bool{}
	for _, candidate := range m.store.OptimizationQueue().Jobs {
		if candidate.Family == "audiobook-opus" {
			recommended[candidate.ID] = true
		}
	}
	created := make([]Job, 0, len(ids))
	pending := make([]diskJob, 0, len(ids))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrUnavailable
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if !recommended[id] {
			return nil, fmt.Errorf("%s: %w", id, ErrNotCandidate)
		}
		item, ok := m.store.Get(id)
		if !ok || item.Kind != api.KindAudiobook || item.Format != api.FormatM4B || item.FilePath == "" {
			return nil, fmt.Errorf("%s: %w", id, ErrNotCandidate)
		}
		root, relative, err := m.libraryFor(item.FilePath)
		if err != nil {
			return nil, err
		}
		coverPath := ""
		useCover := approvedCoverIDs[id]
		if !item.ProbedCoverKnown {
			return nil, fmt.Errorf("%s needs a refreshed cover probe", item.Title)
		}
		if !item.ProbedHasCover && !useCover {
			return nil, fmt.Errorf("%s needs a verified cover or explicit catalog-art approval", item.Title)
		}
		if useCover {
			if item.PosterPath == "" || item.PosterSource == "thumbnail" {
				return nil, fmt.Errorf("%s has no eligible catalog artwork", item.Title)
			}
			coverPath = item.PosterPath
		}
		st, err := os.Stat(item.FilePath)
		if err != nil {
			return nil, fmt.Errorf("stat source: %w", err)
		}
		if st.Size() != item.SizeBytes || (!item.ModTime.IsZero() && !st.ModTime().Equal(item.ModTime)) {
			return nil, fmt.Errorf("%s changed since the catalog scan; rescan before queueing", item.Title)
		}
		bitrate := 32
		if len(item.ProbedAudioChannels) == 1 && item.ProbedAudioChannels[0] == 2 {
			bitrate = 48
		}
		if selected := bitrateOverrides[id]; selected != 0 {
			bitrate = selected
		}
		allowed := (len(item.ProbedAudioChannels) == 1 && item.ProbedAudioChannels[0] == 1 && (bitrate == 24 || bitrate == 32 || bitrate == 40)) ||
			(len(item.ProbedAudioChannels) == 1 && item.ProbedAudioChannels[0] == 2 && (bitrate == 48 || bitrate == 64))
		if !allowed {
			return nil, fmt.Errorf("%s has an unsupported target bitrate for its channel count", item.Title)
		}
		if len(item.ProbedAudioBitrates) != 1 || item.ProbedAudioBitrates[0] <= bitrate*2*1000 {
			return nil, fmt.Errorf("%s source bitrate is too close to the selected target", item.Title)
		}
		for _, existing := range m.state.Jobs {
			if existing.ItemID != id || existing.Status == StatusCanceled {
				continue
			}
			canRevise := existing.Status == StatusStaged && existing.Receipt != nil &&
				strings.HasPrefix(existing.Receipt.PlaybackReview, "needs-revision") && existing.BitrateKbps != bitrate
			if !canRevise {
				return nil, fmt.Errorf("%s: %w", item.Title, ErrConflict)
			}
		}
		for _, existing := range pending {
			if existing.ItemID == id {
				return nil, fmt.Errorf("%s: %w", item.Title, ErrConflict)
			}
		}
		jobID, err := newID()
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		j := diskJob{
			Job: Job{ID: jobID, ItemID: id, Title: item.Title, Status: StatusQueued, Attempt: 1,
				Phase: "Waiting in queue", SourceBytes: st.Size(), BitrateKbps: bitrate,
				CatalogCoverAvailable: item.PosterPath != "" && item.PosterSource != "thumbnail",
				EmbeddedCoverPresent:  item.ProbedHasCover,
				CreatedAt:             now, UpdatedAt: now},
			SourcePath: item.FilePath, LibraryRoot: root, RelativePath: relative,
			SourceMtimeNS:   st.ModTime().UnixNano(),
			UseCatalogCover: useCover, CatalogCoverPath: coverPath,
		}
		pending = append(pending, j)
		created = append(created, j.Job)
	}
	if len(created) == 0 {
		return nil, errors.New("no new jobs selected")
	}
	oldLen := len(m.state.Jobs)
	m.state.Jobs = append(m.state.Jobs, pending...)
	if err := m.persistLocked(); err != nil {
		m.state.Jobs = m.state.Jobs[:oldLen]
		return nil, err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return created, nil
}

func (m *Manager) libraryFor(path string) (string, string, error) {
	var best string
	for _, lib := range m.libraries {
		if lib.Kind != "audiobook" {
			continue
		}
		root, err := filepath.Abs(lib.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && (best == "" || len(root) > len(best)) {
			best = root
		}
	}
	if best == "" {
		return "", "", errors.New("audiobook source is outside configured audiobook roots")
	}
	rel, _ := filepath.Rel(best, path)
	return best, rel, nil
}

func (m *Manager) Pause() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrUnavailable
	}
	wasPaused := m.state.Paused
	m.state.Paused = true
	m.touchLocked()
	if err := m.persistLocked(); err != nil {
		m.state.Paused = wasPaused
		return err
	}
	return nil
}

func (m *Manager) Resume() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrUnavailable
	}
	wasPaused := m.state.Paused
	m.state.Paused = false
	m.touchLocked()
	if err := m.persistLocked(); err != nil {
		m.state.Paused = wasPaused
		return err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		switch j.Status {
		case StatusQueued, StatusInterrupted, StatusFailed:
			j.Status, j.Phase, j.Progress = StatusCanceled, "Canceled", 0
		case StatusCopying, StatusEncoding, StatusValidating, StatusReturning:
			if m.activeCancel == nil {
				j.Status, j.Phase, j.Progress = StatusCanceled, "Canceled before worker start", 0
				break
			}
			m.activeCancel()
			j.Status, j.Phase = StatusCanceled, "Cancel requested; cleaning temporary output"
		default:
			return errors.New("job is already complete and cannot be canceled")
		}
		j.UpdatedAt = time.Now().UTC()
		m.touchLocked()
		return m.persistLocked()
	}
	return ErrNotFound
}

func (m *Manager) Retry(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		if j.Status != StatusFailed && j.Status != StatusInterrupted && j.Status != StatusCanceled {
			return errors.New("only failed, interrupted, or canceled jobs can be retried")
		}
		j.Attempt++
		j.Status, j.Phase, j.Progress, j.Error, j.StartedAt = StatusQueued, "Waiting in queue", 0, "", time.Time{}
		j.Receipt = nil
		j.UpdatedAt = time.Now().UTC()
		m.touchLocked()
		if err := m.persistLocked(); err != nil {
			return err
		}
		select {
		case m.wake <- struct{}{}:
		default:
		}
		return nil
	}
	return ErrNotFound
}

// Prioritize moves a queued job to the front of the sequential worker queue.
func (m *Manager) Prioritize(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	index := -1
	for i := range m.state.Jobs {
		if m.state.Jobs[i].ID == id {
			if m.state.Jobs[i].Status != StatusQueued {
				return errors.New("only queued jobs can be prioritized")
			}
			index = i
			break
		}
	}
	if index < 0 {
		return ErrNotFound
	}
	firstQueued := -1
	for i := range m.state.Jobs {
		if m.state.Jobs[i].Status == StatusQueued {
			firstQueued = i
			break
		}
	}
	if firstQueued < 0 || index == firstQueued {
		return nil
	}
	job := m.state.Jobs[index]
	copy(m.state.Jobs[firstQueued+1:index+1], m.state.Jobs[firstQueued:index])
	m.state.Jobs[firstQueued] = job
	m.state.Jobs[firstQueued].UpdatedAt = time.Now().UTC()
	if err := m.persistLocked(); err != nil {
		return err
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) MarkPlaybackReview(id, decision, note string) error {
	if decision != "accepted" && decision != "needs-revision" {
		return errors.New("review decision must be accepted or needs-revision")
	}
	if len(note) > 500 {
		return errors.New("review note must be at most 500 characters")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		if j.Status != StatusStaged || j.Receipt == nil {
			return errors.New("only verified staged outputs can be playback-reviewed")
		}
		previous := *j.Receipt
		j.Receipt.PlaybackReview = decision
		if strings.TrimSpace(note) != "" {
			j.Receipt.PlaybackReview += ": " + strings.TrimSpace(note)
		}
		j.UpdatedAt = time.Now().UTC()
		receiptPath := filepath.Join(j.LibraryRoot, filepath.FromSlash(j.Receipt.StagedRelativePath)) + ".receipt.json"
		if err := ensureInside(filepath.Join(j.LibraryRoot, ".sonder-optimization-staging"), receiptPath); err != nil {
			*j.Receipt = previous
			return err
		}
		if err := replaceReceipt(receiptPath, j.Receipt); err != nil {
			*j.Receipt = previous
			return err
		}
		if err := m.persistLocked(); err != nil {
			*j.Receipt = previous
			_ = replaceReceipt(receiptPath, j.Receipt)
			return err
		}
		return nil
	}
	return ErrNotFound
}

// Promote replaces the original only after the staged output was accepted.
func (m *Manager) Promote(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		if j.Status != StatusStaged || j.Receipt == nil {
			return errors.New("only verified staged outputs can replace originals")
		}
		staged := filepath.Join(j.LibraryRoot, filepath.FromSlash(j.Receipt.StagedRelativePath))
		source := filepath.Join(j.LibraryRoot, filepath.FromSlash(j.RelativePath))
		if err := ensureInside(filepath.Join(j.LibraryRoot, ".sonder-optimization-staging"), staged); err != nil {
			return err
		}
		if _, err := os.Stat(staged); err != nil {
			return fmt.Errorf("staged output unavailable: %w", err)
		}
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("original source unavailable: %w", err)
		}
		backup := source + ".sonder-original-backup"
		_ = os.Remove(backup)
		if err := os.Rename(source, backup); err != nil {
			return fmt.Errorf("move original aside: %w", err)
		}
		if err := os.Rename(staged, source); err != nil {
			_ = os.Rename(backup, source)
			return fmt.Errorf("install optimized output: %w", err)
		}
		_ = os.Remove(backup)
		j.Status, j.Phase, j.Progress = StatusAccepted, "Optimized file installed; original removed", 100
		j.Receipt.PlaybackReview = "auto-promoted after full validation"
		j.UpdatedAt = time.Now().UTC()
		return m.persistLocked()
	}
	return ErrNotFound
}

func (m *Manager) OpenStagedOutput(id string) (*os.File, string, time.Time, error) {
	m.mu.Lock()
	var root, relative string
	for _, j := range m.state.Jobs {
		if j.ID == id && j.Status == StatusStaged && j.Receipt != nil {
			root, relative = j.LibraryRoot, filepath.FromSlash(j.Receipt.StagedRelativePath)
			break
		}
	}
	m.mu.Unlock()
	if root == "" || relative == "" {
		return nil, "", time.Time{}, ErrNotFound
	}
	reviewRoot := filepath.Join(root, ".sonder-optimization-staging")
	path := filepath.Join(root, relative)
	if err := ensureInside(reviewRoot, path); err != nil {
		return nil, "", time.Time{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, "", time.Time{}, err
	}
	if st.IsDir() {
		f.Close()
		return nil, "", time.Time{}, errors.New("staged output is not a file")
	}
	return f, filepath.Base(path), st.ModTime(), nil
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
	}
	m.mu.Unlock()
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) worker() {
	defer close(m.done)
	for {
		job, ok := m.nextJob()
		if !ok {
			select {
			case <-m.ctx.Done():
				return
			case <-m.wake:
				continue
			}
		}
		jobCtx, cancel := context.WithCancel(m.ctx)
		m.mu.Lock()
		canceledBeforeStart := true
		for i := range m.state.Jobs {
			if m.state.Jobs[i].ID == job.ID && m.state.Jobs[i].Status != StatusCanceled {
				canceledBeforeStart = false
				m.activeCancel = cancel
				break
			}
		}
		m.mu.Unlock()
		if canceledBeforeStart {
			cancel()
			continue
		}
		err := m.runJob(jobCtx, job)
		cancel()
		m.mu.Lock()
		m.activeCancel = nil
		for i := range m.state.Jobs {
			if m.state.Jobs[i].ID != job.ID {
				continue
			}
			j := &m.state.Jobs[i]
			if err != nil {
				if errors.Is(err, context.Canceled) {
					j.Status, j.Phase = StatusCanceled, "Canceled; temporary files removed"
				} else {
					j.Status, j.Phase, j.Error = StatusFailed, "Failed", err.Error()
				}
				j.Progress = 0
			} else {
				j.Status, j.Phase, j.Progress, j.CurrentBytes = StatusStaged, "Verified and copied to NAS review staging", 100, j.SourceBytes
			}
			j.UpdatedAt = time.Now().UTC()
			break
		}
		m.touchLocked()
		_ = m.persistLocked()
		paused := m.state.Paused
		m.mu.Unlock()
		if err == nil {
			// Validated outputs are installed immediately to avoid retaining two full libraries.
			_ = m.Promote(job.ID)
		}
		if paused {
			continue
		}
	}
}

func (m *Manager) nextJob() (diskJob, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.state.Paused {
		return diskJob{}, false
	}
	for i := range m.state.Jobs {
		if m.state.Jobs[i].Status == StatusQueued {
			j := &m.state.Jobs[i]
			j.Status, j.Phase, j.Progress, j.CurrentBytes, j.StartedAt = StatusCopying, "Copying source to local workspace", 0, 0, time.Now().UTC()
			j.UpdatedAt = time.Now().UTC()
			m.touchLocked()
			_ = m.persistLocked()
			return *j, true
		}
	}
	return diskJob{}, false
}

func (m *Manager) update(id string, status Status, phase string, progress float64, current int64, receipt *Receipt) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		if j.Status == StatusCanceled {
			return
		}
		j.Status, j.Phase, j.Progress, j.CurrentBytes = status, phase, progress, current
		if receipt != nil {
			j.Receipt = receipt
		}
		j.UpdatedAt = time.Now().UTC()
		m.touchLocked()
		_ = m.persistLocked()
		return
	}
}

func (m *Manager) touchLocked() { m.state.UpdatedAt = time.Now().UTC() }

func (m *Manager) persistLocked() error {
	m.touchLocked()
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.statePath), ".queue-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
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
	return os.Rename(tmp, m.statePath)
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func freeBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
