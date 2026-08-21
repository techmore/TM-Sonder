// Package library holds the in-memory media catalog, progress records,
// activity log, and their JSON snapshot persistence.
package library

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
)

const ActivityCap = 200

// Item is the server-side catalog entry. It embeds the wire DTO and adds
// filesystem fields that are never serialized to API clients.
type Item struct {
	api.MediaItem
	FilePath     string    `json:"filePath"`
	SidecarPaths []string  `json:"sidecarPaths,omitempty"`
	PosterPath   string    `json:"posterPath,omitempty"`
	BackdropPath string    `json:"backdropPath,omitempty"`
	SizeBytes    int64     `json:"sizeBytes,omitempty"`
	ModTime      time.Time `json:"modTime"`
}

func (i *Item) clone() *Item {
	c := *i
	c.MediaItem = cloneMediaItem(i.MediaItem)
	if i.SidecarPaths != nil {
		c.SidecarPaths = append([]string(nil), i.SidecarPaths...)
	}
	return &c
}

func cloneMediaItem(m api.MediaItem) api.MediaItem {
	if m.Tags != nil {
		m.Tags = append([]string(nil), m.Tags...)
	}
	if m.EmbeddedAudioTracks != nil {
		m.EmbeddedAudioTracks = append([]api.PlaybackTrack(nil), m.EmbeddedAudioTracks...)
	}
	if m.EmbeddedSubtitleTracks != nil {
		m.EmbeddedSubtitleTracks = append([]api.PlaybackTrack(nil), m.EmbeddedSubtitleTracks...)
	}
	return m
}

// Store is a concurrency-safe catalog guarded by an RWMutex.
type Store struct {
	mu          sync.RWMutex
	items       map[string]*Item
	progress    map[string]*api.ProgressRecord
	directories []api.MediaDirectory
	activity    []api.ActivityEvent
	gen         int64

	saveMu sync.Mutex
	timer  *time.Timer
	onSave func(error)
}

func New() *Store {
	return &Store{
		items:    make(map[string]*Item),
		progress: make(map[string]*api.ProgressRecord),
	}
}

// ETag returns the current library entity tag, quoted per RFC 7232.
func (s *Store) ETag() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf(`"sonder-library-%d"`, s.gen)
}

func (s *Store) Generation() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gen
}

// Upsert inserts or replaces items, bumping the generation once.
func (s *Store) Upsert(items ...*Item) {
	if len(items) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range items {
		s.items[it.ID] = it.clone()
	}
	s.gen++
}

// Remove deletes items by ID; removed reports how many were deleted.
func (s *Store) Remove(ids ...string) int {
	if len(ids) == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, id := range ids {
		if _, ok := s.items[id]; ok {
			delete(s.items, id)
			n++
		}
	}
	if n > 0 {
		s.gen++
	}
	return n
}

// RetainOnly drops every item whose ID is not in keep; used by incremental
// scans for removal detection. Reports how many were dropped.
func (s *Store) RetainOnly(keep map[string]bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for id := range s.items {
		if !keep[id] {
			delete(s.items, id)
			dropped++
		}
	}
	if dropped > 0 {
		s.gen++
	}
	return dropped
}

// Get returns a copy of one item including filesystem-only fields.
func (s *Store) Get(id string) (*Item, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	it, ok := s.items[id]
	if !ok {
		return nil, false
	}
	return it.clone(), true
}

// Items returns wire-shaped copies sorted by title then ID.
func (s *Store) Items() []api.MediaItem {
	internal := s.InternalItems()
	out := make([]api.MediaItem, len(internal))
	for i, it := range internal {
		out[i] = it.MediaItem
	}
	return out
}

// InternalItems returns full copies (including file paths), title-sorted.
func (s *Store) InternalItems() []*Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it.clone())
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Title != out[b].Title {
			return out[a].Title < out[b].Title
		}
		return out[a].ID < out[b].ID
	})
	return out
}

// Count returns the number of catalog items.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// SetProgress records playback position last-write-wins per itemID (older or
// equal timestamps never overwrite newer ones) and merges the seconds into
// the item's wire progress. Returns false if the write was stale-rejected.
func (s *Store) SetProgress(rec api.ProgressRecord) bool {
	if rec.ItemID == "" {
		return false
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	if rec.ID == "" {
		rec.ID = api.NewID()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if prev, ok := s.progress[rec.ItemID]; ok && !rec.UpdatedAt.After(prev.UpdatedAt) {
		return false
	}
	r := rec
	s.progress[rec.ItemID] = &r

	if it, ok := s.items[rec.ItemID]; ok {
		it.ProgressSeconds = r.Seconds
		if r.Duration > 0 && it.DurationSeconds == 0 {
			it.DurationSeconds = r.Duration
		}
	}
	s.gen++
	return true
}

// Progress returns all progress records, itemID-sorted for stable output.
func (s *Store) Progress() []api.ProgressRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]api.ProgressRecord, 0, len(s.progress))
	for _, p := range s.progress {
		out = append(out, *p)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ItemID < out[b].ItemID })
	return out
}

// ProgressFor returns a copy of one item's progress record.
func (s *Store) ProgressFor(itemID string) (api.ProgressRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.progress[itemID]
	if !ok {
		return api.ProgressRecord{}, false
	}
	return *p, true
}

// SetDirectories replaces the media directory table shown in /api/library.
func (s *Store) SetDirectories(dirs []api.MediaDirectory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.directories = dirs
	s.gen++
}

// Directories returns a copy of the media directory table.
func (s *Store) Directories() []api.MediaDirectory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]api.MediaDirectory(nil), s.directories...)
}

// RecordActivity appends an event to the bounded activity log.
func (s *Store) RecordActivity(title, detail, icon string) {
	ev := api.ActivityEvent{
		ID:     api.NewID(),
		Title:  title,
		Detail: detail,
		Icon:   icon,
		Date:   time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activity = append(s.activity, ev)
	if len(s.activity) > ActivityCap {
		s.activity = s.activity[len(s.activity)-ActivityCap:]
	}
	s.gen++
}

// Activity returns a copy of the newest events, oldest first.
func (s *Store) Activity() []api.ActivityEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]api.ActivityEvent, len(s.activity))
	copy(out, s.activity)
	return out
}
