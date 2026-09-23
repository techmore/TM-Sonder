package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tm-sonder/server/internal/api"
)

// CurrentSnapshotVersion is bumped when the on-disk catalog shape changes.
// Older snapshots are migrated in memory before they are installed.
const CurrentSnapshotVersion = 2

// Snapshot is the on-disk catalog format. Generation is persisted so
// If-None-Match / 304 flows survive restarts.
type Snapshot struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Generation    int64                `json:"generation"`
	Items         []*Item              `json:"items"`
	Progress      []api.ProgressRecord `json:"progress"`
	Directories   []api.MediaDirectory `json:"mediaDirectories"`
	Activity      []api.ActivityEvent  `json:"activity"`
	Lists         []BookList           `json:"lists"`
}

// snapshot builds a stable deep copy of current state.
func (s *Store) snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := &Snapshot{
		SchemaVersion: CurrentSnapshotVersion,
		Generation:    s.gen,
		Items:         make([]*Item, 0, len(s.items)),
		Progress:      make([]api.ProgressRecord, 0, len(s.progress)),
		Directories:   append([]api.MediaDirectory(nil), s.directories...),
		Activity:      make([]api.ActivityEvent, len(s.activity)),
		Lists:         make([]BookList, len(s.lists)),
	}
	for _, it := range s.items {
		snap.Items = append(snap.Items, it.clone())
	}
	for _, p := range s.progress {
		snap.Progress = append(snap.Progress, *p)
	}
	copy(snap.Activity, s.activity)
	for i, list := range s.lists {
		snap.Lists[i] = cloneBookList(list)
	}
	return snap
}

// Snapshot returns a stable deep copy suitable for backup/export.
func (s *Store) Snapshot() Snapshot { return *s.snapshot() }

// Load replaces in-memory state with the contents of path.
func (s *Store) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("library: read snapshot %s: %w", path, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("library: parse snapshot %s: %w", path, err)
	}
	if err := migrateSnapshot(&snap); err != nil {
		return fmt.Errorf("library: migrate snapshot %s: %w", path, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	items := make(map[string]*Item, len(snap.Items))
	for _, it := range snap.Items {
		if it.ID == "" {
			continue
		}
		// Repair the known episode-title-as-show bug without waiting for
		// network storage traversal. No media files or progress are changed.
		if it.Kind == api.KindTVShow && it.ShowTitle != nil && rePackedEpisode.MatchString(*it.ShowTitle) {
			parsed := ParseFilename(it.FilePath, "tvShow")
			if parsed.ShowTitle != "" && !rePackedEpisode.MatchString(parsed.ShowTitle) {
				it.ShowTitle = &parsed.ShowTitle
				it.Title, it.Subtitle = parsed.Title, parsed.Subtitle
				it.SeasonNumber, it.EpisodeNumber = parsed.Season, parsed.Episode
			}
		}
		items[it.ID] = it
	}
	progress := make(map[string]*api.ProgressRecord, len(snap.Progress))
	for i := range snap.Progress {
		p := snap.Progress[i]
		if p.ItemID == "" {
			continue
		}
		progress[p.ItemID] = &p
	}
	s.items = items
	s.progress = progress
	s.directories = snap.Directories
	s.activity = snap.Activity
	s.lists = nil
	for _, list := range snap.Lists {
		s.lists = append(s.lists, cloneBookList(list))
	}
	s.gen = max64(snap.Generation, 1)
	return nil
}

func migrateSnapshot(snap *Snapshot) error {
	// Snapshots before schemaVersion existed are version 1. They already have
	// the fields needed by the current runtime; v2 adds portable identity
	// fields to items and normalizes list containers.
	if snap.SchemaVersion == 0 {
		snap.SchemaVersion = 1
	}
	if snap.SchemaVersion > CurrentSnapshotVersion {
		return fmt.Errorf("schema version %d is newer than supported version %d", snap.SchemaVersion, CurrentSnapshotVersion)
	}
	for snap.SchemaVersion < CurrentSnapshotVersion {
		switch snap.SchemaVersion {
		case 1:
			for i := range snap.Lists {
				if snap.Lists[i].ItemIDs == nil {
					snap.Lists[i].ItemIDs = []string{}
				}
				if snap.Lists[i].ItemTags == nil {
					snap.Lists[i].ItemTags = map[string][]string{}
				}
			}
			snap.SchemaVersion = 2
		default:
			return fmt.Errorf("no migration for schema version %d", snap.SchemaVersion)
		}
	}
	if snap.Items == nil {
		snap.Items = []*Item{}
	}
	if snap.Progress == nil {
		snap.Progress = []api.ProgressRecord{}
	}
	if snap.Lists == nil {
		snap.Lists = []BookList{}
	}
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// writeAtomic writes data to path via a temp file in the same directory plus
// rename, fsyncing the file and (best effort) the parent directory.
func writeAtomic(path string, data []byte, tmpPrefix string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tmpPrefix)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	// Ignore errors: some network filesystems do not support directory sync.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Save atomically writes the current state to path via temp file + rename.
func (s *Store) Save(path string) error {
	// Serialize writers: the debounce timer and an explicit Flush can fire
	// concurrently and must not interleave temp files or renames.
	s.persistMu.Lock()
	defer s.persistMu.Unlock()

	snap := s.snapshot()
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return writeAtomic(path, data, ".sonder-snapshot-*")
}

// ProgressSnapshot is the small, frequently-written progress sidecar. It exists
// so playback heartbeats do not rewrite the whole multi-megabyte catalog.
type ProgressSnapshot struct {
	Progress []api.ProgressRecord `json:"progress"`
}

// SaveProgress writes only the progress records, atomically.
func (s *Store) SaveProgress(path string) error {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()

	s.mu.RLock()
	snap := ProgressSnapshot{Progress: make([]api.ProgressRecord, 0, len(s.progress))}
	for _, p := range s.progress {
		snap.Progress = append(snap.Progress, *p)
	}
	s.mu.RUnlock()

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return writeAtomic(path, data, ".sonder-progress-*")
}

// LoadProgress overlays a progress sidecar onto already-loaded catalog state,
// so a restart resumes with the latest playback positions.
func (s *Store) LoadProgress(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var snap ProgressSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("library: parse progress sidecar %s: %w", path, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.progress == nil {
		s.progress = make(map[string]*api.ProgressRecord, len(snap.Progress))
	}
	for i := range snap.Progress {
		p := snap.Progress[i]
		if p.ItemID == "" {
			continue
		}
		rec := p
		s.progress[rec.ItemID] = &rec
		if it, ok := s.items[rec.ItemID]; ok {
			it.ProgressSeconds = rec.Seconds
			if rec.Duration > 0 && it.DurationSeconds == 0 {
				it.DurationSeconds = rec.Duration
			}
		}
	}
	return nil
}

const DefaultSaveDelay = 500 * time.Millisecond

// SaveDebounced schedules a save after delay, coalescing rapid mutations into
// a single disk write. Only one pending save exists at a time.
func (s *Store) SaveDebounced(path string, delay time.Duration) {
	s.SaveDebouncedWithCallback(path, delay, nil)
}

// SaveDebouncedWithCallback is SaveDebounced with a completion hook invoked
// once the write finishes (useful for tests and logging).
func (s *Store) SaveDebouncedWithCallback(path string, delay time.Duration, cb func(error)) {
	if delay <= 0 {
		delay = DefaultSaveDelay
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, func() {
		err := s.Save(path)
		s.saveMu.Lock()
		s.timer = nil
		s.saveMu.Unlock()
		if cb != nil {
			cb(err)
		}
	})
}

// SaveProgressDebounced coalesces rapid progress writes into one small file
// write, independent of the full-catalog debounce.
func (s *Store) SaveProgressDebounced(path string, delay time.Duration) {
	if delay <= 0 {
		delay = DefaultSaveDelay
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if s.progressTimer != nil {
		s.progressTimer.Stop()
	}
	s.progressTimer = time.AfterFunc(delay, func() {
		_ = s.SaveProgress(path)
		s.progressMu.Lock()
		s.progressTimer = nil
		s.progressMu.Unlock()
	})
}

// Flush cancels any pending debounced save and writes synchronously. Safe to
// call during shutdown even if no save is pending.
func (s *Store) Flush(path string) error {
	s.saveMu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.saveMu.Unlock()
	return s.Save(path)
}
