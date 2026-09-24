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
	FilePath string `json:"filePath"`
	// StableKey identifies the media relative to its configured library rather
	// than by the host's absolute mount path. It lets imports and container
	// remaps retain list/progress references without changing public IDs.
	StableKey                string    `json:"stableKey,omitempty"`
	SourceRelativePath       string    `json:"sourceRelativePath,omitempty"`
	SidecarPaths             []string  `json:"sidecarPaths,omitempty"`
	PosterPath               string    `json:"posterPath,omitempty"`
	BackdropPath             string    `json:"backdropPath,omitempty"`
	PosterSource             string    `json:"posterSource,omitempty"` // local|thumbnail|wikipedia|audnexus|open-library
	SizeBytes                int64     `json:"sizeBytes,omitempty"`
	ProbedAudioChannels      []int     `json:"probedAudioChannels,omitempty"`
	ProbedAudioBitrates      []int     `json:"probedAudioBitrates,omitempty"`
	ProbedVideoStreams       int       `json:"probedVideoStreams,omitempty"`
	ProbedHasCover           bool      `json:"probedHasCover,omitempty"`
	ProbedCoverKnown         bool      `json:"probedCoverKnown,omitempty"`
	ProbedUnsupportedStreams int       `json:"probedUnsupportedStreams,omitempty"`
	ModTime                  time.Time `json:"modTime"`
	ParseVersion             int       `json:"parseVersion,omitempty"` // parser semantics stamp; older versions rebuild on rescan
}

func (i *Item) clone() *Item {
	c := *i
	c.MediaItem = cloneMediaItem(i.MediaItem)
	if i.SidecarPaths != nil {
		c.SidecarPaths = append([]string(nil), i.SidecarPaths...)
	}
	if i.ProbedAudioChannels != nil {
		c.ProbedAudioChannels = append([]int(nil), i.ProbedAudioChannels...)
	}
	if i.ProbedAudioBitrates != nil {
		c.ProbedAudioBitrates = append([]int(nil), i.ProbedAudioBitrates...)
	}
	return &c
}

func cloneMediaItem(m api.MediaItem) api.MediaItem {
	if m.Tags != nil {
		m.Tags = append([]string(nil), m.Tags...)
	}
	if m.Genres != nil {
		m.Genres = append([]string(nil), m.Genres...)
	}
	if m.EmbeddedAudioTracks != nil {
		m.EmbeddedAudioTracks = append([]api.PlaybackTrack(nil), m.EmbeddedAudioTracks...)
	}
	if m.ProbedAudioCodecs != nil {
		m.ProbedAudioCodecs = append([]string(nil), m.ProbedAudioCodecs...)
	}
	if m.EmbeddedSubtitleTracks != nil {
		m.EmbeddedSubtitleTracks = append([]api.PlaybackTrack(nil), m.EmbeddedSubtitleTracks...)
	}
	// Deep-copy pointed scalars so callers can never write through a pointer
	// into store-owned state outside the lock.
	m.ShowTitle = cloneStringPtr(m.ShowTitle)
	m.ShowGroupID = cloneStringPtr(m.ShowGroupID)
	m.ShowGroupTitle = cloneStringPtr(m.ShowGroupTitle)
	m.LibraryID = cloneStringPtr(m.LibraryID)
	m.MetadataIDSource = cloneStringPtr(m.MetadataIDSource)
	m.MetadataID = cloneStringPtr(m.MetadataID)
	m.Edition = cloneStringPtr(m.Edition)
	m.SplitPart = cloneStringPtr(m.SplitPart)
	m.Author = cloneStringPtr(m.Author)
	m.Narrator = cloneStringPtr(m.Narrator)
	m.PosterURL = cloneStringPtr(m.PosterURL)
	m.BackdropURL = cloneStringPtr(m.BackdropURL)
	m.SeasonNumber = cloneIntPtr(m.SeasonNumber)
	m.EpisodeNumber = cloneIntPtr(m.EpisodeNumber)
	m.ProbedWidth = cloneIntPtr(m.ProbedWidth)
	m.ProbedHeight = cloneIntPtr(m.ProbedHeight)
	m.ProbedCodec = cloneStringPtr(m.ProbedCodec)
	m.ProbedBitrate = cloneIntPtr(m.ProbedBitrate)
	m.BookValidation = cloneStringPtr(m.BookValidation)
	m.CoverSource = cloneStringPtr(m.CoverSource)
	if m.TrackProbeUpdatedAt != nil {
		t := *m.TrackProbeUpdatedAt
		m.TrackProbeUpdatedAt = &t
	}
	for i := range m.EmbeddedAudioTracks {
		m.EmbeddedAudioTracks[i].LanguageCode = cloneStringPtr(m.EmbeddedAudioTracks[i].LanguageCode)
		m.EmbeddedAudioTracks[i].URL = cloneStringPtr(m.EmbeddedAudioTracks[i].URL)
	}
	for i := range m.EmbeddedSubtitleTracks {
		m.EmbeddedSubtitleTracks[i].LanguageCode = cloneStringPtr(m.EmbeddedSubtitleTracks[i].LanguageCode)
		m.EmbeddedSubtitleTracks[i].URL = cloneStringPtr(m.EmbeddedSubtitleTracks[i].URL)
	}
	return m
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Store is a concurrency-safe catalog guarded by an RWMutex.
type Store struct {
	mu          sync.RWMutex
	items       map[string]*Item
	progress    map[string]*api.ProgressRecord
	lists       []BookList
	directories []api.MediaDirectory
	activity    []api.ActivityEvent
	gen         int64

	saveMu        sync.Mutex
	persistMu     sync.Mutex
	timer         *time.Timer
	progressMu    sync.Mutex
	progressTimer *time.Timer
	onSave        func(error)
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

// Update atomically mutates one catalog item while holding the store lock.
// fn receives a private copy of the current item and returns whether it
// changed anything; unchanged updates do not bump the generation. Reports
// whether the item existed.
//
// This is the safe alternative to Get→mutate→Upsert: because the mutation
// happens under the lock against current state, concurrent writers (probe,
// thumbnail, enrichment, progress) cannot clobber each other's fields.
func (s *Store) Update(id string, fn func(*Item) bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.items[id]
	if !ok {
		return false
	}
	next := cur.clone()
	if !fn(next) {
		return true
	}
	s.items[id] = next
	s.gen++
	return true
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
// scans for removal detection. Items belonging to a library in
// preserveLibraries are always kept, so a library whose scan failed (an
// unmounted NAS share, a permission error) is never wiped from the catalog.
// Reports how many were dropped.
func (s *Store) RetainOnly(keep map[string]bool, preserveLibraries map[string]bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	dropped := 0
	for id, it := range s.items {
		if keep[id] {
			continue
		}
		if it.LibraryID != nil && preserveLibraries[*it.LibraryID] {
			continue
		}
		delete(s.items, id)
		dropped++
	}
	if dropped > 0 {
		s.gen++
	}
	return dropped
}

// CountByLibrary returns item counts keyed by library ID without cloning or
// sorting the catalog. Items with no library assignment are not counted.
func (s *Store) CountByLibrary() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	for _, it := range s.items {
		if it.LibraryID != nil {
			counts[*it.LibraryID]++
		}
	}
	return counts
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

// FindByStableKey returns the catalog item associated with a library-relative
// media key. It is intentionally a linear lookup for now; the catalog is
// loaded in memory and this path only runs when an absolute-path ID misses.
func (s *Store) FindByStableKey(key string) (*Item, bool) {
	if key == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.StableKey == key {
			return item.clone(), true
		}
	}
	return nil, false
}

// Items returns wire-shaped copies sorted by title then ID.
func (s *Store) Items() []api.MediaItem {
	internal := s.InternalItems()
	out := make([]api.MediaItem, len(internal))
	for i, it := range internal {
		out[i] = it.MediaItem
		// Every movie gets a stable poster endpoint. The endpoint serves the
		// discovered artwork when available, falls back to a relocated
		// generated frame, and finally uses a title placeholder. This keeps
		// movie cards visually complete while still allowing official/local
		// artwork to take precedence.
		if out[i].Kind == api.KindMovie && out[i].PosterURL == nil {
			u := "/artwork/poster/" + out[i].ID
			out[i].PosterURL = &u
		}
	}
	return out
}

// InternalItems returns full copies (including file paths), title-sorted.
func (s *Store) InternalItems() []*Item {
	s.mu.RLock()
	out := make([]*Item, 0, len(s.items))
	for _, it := range s.items {
		out = append(out, it.clone())
	}
	s.mu.RUnlock()
	// Sorting does not need the store lock. Keeping it outside the critical
	// section prevents a full-catalog response from blocking poster, stream,
	// and status lookups while 22k titles are ordered.
	sort.Slice(out, func(a, b int) bool {
		if out[a].Title != out[b].Title {
			return out[a].Title < out[b].Title
		}
		return out[a].ID < out[b].ID
	})
	return out
}

// InternalItemsOfKind returns full copies of items matching kind, title-sorted.
// It avoids cloning the whole catalog for kind-specific routes.
func (s *Store) InternalItemsOfKind(kind api.MediaKind) []*Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Item, 0)
	for _, it := range s.items {
		if it.Kind == kind {
			out = append(out, it.clone())
		}
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
