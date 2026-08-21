package library

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tm-sonder/server/internal/api"
)

// Snapshot is the on-disk catalog format. Generation is persisted so
// If-None-Match / 304 flows survive restarts.
type Snapshot struct {
	Generation  int64                `json:"generation"`
	Items       []*Item              `json:"items"`
	Progress    []api.ProgressRecord `json:"progress"`
	Directories []api.MediaDirectory `json:"mediaDirectories"`
	Activity    []api.ActivityEvent  `json:"activity"`
}

// snapshot builds a stable deep copy of current state.
func (s *Store) snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := &Snapshot{
		Generation:  s.gen,
		Items:       make([]*Item, 0, len(s.items)),
		Progress:    make([]api.ProgressRecord, 0, len(s.progress)),
		Directories: append([]api.MediaDirectory(nil), s.directories...),
		Activity:    make([]api.ActivityEvent, len(s.activity)),
	}
	for _, it := range s.items {
		snap.Items = append(snap.Items, it.clone())
	}
	for _, p := range s.progress {
		snap.Progress = append(snap.Progress, *p)
	}
	copy(snap.Activity, s.activity)
	return snap
}

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

	s.mu.Lock()
	defer s.mu.Unlock()
	items := make(map[string]*Item, len(snap.Items))
	for _, it := range snap.Items {
		if it.ID == "" {
			continue
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
	s.gen = max64(snap.Generation, 1)
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// Save atomically writes the current state to path via temp file + rename.
func (s *Store) Save(path string) error {
	snap := s.snapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sonder-snapshot-*")
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
	return os.Rename(tmpName, path)
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
