// Package mediacache provides a bounded, read-through cache for media files.
//
// The cache deliberately copies complete files in the background. A first
// request continues reading from the NAS immediately; subsequent requests use
// the local copy once its atomic promotion has completed. This keeps startup,
// seeking, and failed NAS reads from blocking on a multi-gigabyte copy.
package mediacache

import (
	"container/heap"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"tm-sonder/server/internal/api"
)

// Config controls the local media cache.
type Config struct {
	Enabled      bool
	Dir          string
	MaxBytes     int64
	MinFreeBytes int64
}

// Media identifies one immutable source version. Size and ModTime are part of
// the cache key, so a changed NAS file never serves an old local copy.
type Media struct {
	ID         string
	SourcePath string
	Kind       api.MediaKind
	Year       int
	Size       int64
	ModTime    time.Time
}

// Snapshot is safe to expose through a status endpoint.
type Snapshot struct {
	Enabled      bool              `json:"enabled"`
	Directory    string            `json:"directory"`
	MaxBytes     int64             `json:"maxBytes"`
	MinFreeBytes int64             `json:"minFreeBytes"`
	FreeBytes    int64             `json:"freeBytes"`
	CachedBytes  int64             `json:"cachedBytes"`
	CachedFiles  int               `json:"cachedFiles"`
	ActiveFiles  int               `json:"activeFiles"`
	Copying      int               `json:"copying"`
	Queued       int               `json:"queued"`
	Policy       map[string]string `json:"policy"`
}

type entry struct {
	key      string
	path     string
	size     int64
	kind     api.MediaKind
	year     int
	active   int
	lastUsed time.Time
}

type metadata struct {
	ID   string        `json:"id"`
	Kind api.MediaKind `json:"kind"`
	Year int           `json:"year"`
}

type pending struct {
	media    Media
	key      string
	priority int
	order    int64
	index    int
}

type pendingQueue []*pending

func (q pendingQueue) Len() int { return len(q) }

func (q pendingQueue) Less(i, j int) bool {
	if q[i].priority != q[j].priority {
		return q[i].priority > q[j].priority
	}
	return q[i].order < q[j].order
}

func (q pendingQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *pendingQueue) Push(x any) {
	p := x.(*pending)
	p.index = len(*q)
	*q = append(*q, p)
}

func (q *pendingQueue) Pop() any {
	old := *q
	n := len(old)
	p := old[n-1]
	old[n-1] = nil
	p.index = -1
	*q = old[:n-1]
	return p
}

// Manager is safe for concurrent HTTP requests.
type Manager struct {
	dir      string
	maxBytes int64
	minFree  int64
	enabled  bool

	mu       sync.Mutex
	entries  map[string]*entry
	pending  map[string]*pending
	copying  map[string]bool
	queue    pendingQueue
	bytes    int64
	order    int64
	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}
	closeOne sync.Once
}

var errCacheStopping = errors.New("media cache stopping")

// New creates or opens a cache. Existing complete entries are retained across
// service restarts; partial copies are discarded.
func New(cfg Config) (*Manager, error) {
	m := &Manager{
		dir:      cfg.Dir,
		maxBytes: cfg.MaxBytes,
		minFree:  cfg.MinFreeBytes,
		enabled:  cfg.Enabled && cfg.Dir != "" && cfg.MaxBytes > 0,
		entries:  make(map[string]*entry),
		pending:  make(map[string]*pending),
		copying:  make(map[string]bool),
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	if !m.enabled {
		close(m.done)
		return m, nil
	}
	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return nil, fmt.Errorf("media cache directory: %w", err)
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	go m.worker()
	return m, nil
}

// Close stops background copies after the current copy finishes. A partial
// copy remains hidden and is removed on the next open.
func (m *Manager) Close() {
	m.closeOne.Do(func() {
		if !m.enabled {
			return
		}
		close(m.stop)
		<-m.done
	})
}

// Acquire returns the local cache path when a complete copy is available. On
// a miss it queues a background copy and returns the NAS source path. The
// release function must be called when the returned path is no longer in use;
// active entries are protected from eviction.
func (m *Manager) Acquire(media Media) (path string, release func(), hit bool) {
	if !m.cacheable(media) {
		return media.SourcePath, func() {}, false
	}
	key := cacheKey(media)
	m.mu.Lock()
	if e, ok := m.entries[key]; ok {
		if st, err := os.Stat(e.path); err == nil && st.Size() == media.Size {
			e.active++
			e.lastUsed = time.Now()
			path := e.path
			m.mu.Unlock()
			_ = os.Chtimes(path, time.Now(), time.Now())
			return path, func() { m.release(key) }, true
		}
		m.removeEntryLocked(key)
	}
	m.enqueueLocked(media, key)
	m.mu.Unlock()
	m.signal()
	return media.SourcePath, func() {}, false
}

// Prefetch queues a copy without changing the serving path. It is useful to a
// caller that wants to warm one or more known-priority items while preserving
// the same cache policy.
func (m *Manager) Prefetch(media Media) {
	if !m.cacheable(media) {
		return
	}
	key := cacheKey(media)
	m.mu.Lock()
	m.enqueueLocked(media, key)
	m.mu.Unlock()
	m.signal()
}

// Status returns current cache accounting and the policy exposed to the UI.
func (m *Manager) Status() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := 0
	for _, e := range m.entries {
		if e.active > 0 {
			active++
		}
	}
	return Snapshot{
		Enabled:      m.enabled,
		Directory:    m.dir,
		MaxBytes:     m.maxBytes,
		MinFreeBytes: m.minFree,
		FreeBytes:    availableBytes(m.dir),
		CachedBytes:  m.bytes,
		CachedFiles:  len(m.entries),
		ActiveFiles:  active,
		Copying:      len(m.copying),
		Queued:       len(m.pending),
		Policy: map[string]string{
			"ebook":       "highest priority",
			"audiobook":   "high priority",
			"movie":       "medium priority; newer release years retained first",
			"documentary": "medium-low priority; newer release years retained first",
			"tvShow":      "low priority",
		},
	}
}

func (m *Manager) cacheable(media Media) bool {
	return m.enabled && media.SourcePath != "" && media.Size > 0 && media.Size <= m.maxBytes
}

func (m *Manager) enqueueLocked(media Media, key string) {
	if _, ok := m.entries[key]; ok || m.copying[key] {
		return
	}
	if _, ok := m.pending[key]; ok {
		return
	}
	m.order++
	p := &pending{media: media, key: key, priority: priority(media.Kind, media.Year), order: m.order}
	m.pending[key] = p
	heap.Push(&m.queue, p)
}

func (m *Manager) release(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[key]; ok && e.active > 0 {
		e.active--
	}
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) worker() {
	defer close(m.done)
	for {
		select {
		case <-m.stop:
			return
		case <-m.wake:
			for {
				select {
				case <-m.stop:
					return
				default:
				}
				p := m.next()
				if p == nil {
					break
				}
				m.copy(p.media, p.key)
				m.mu.Lock()
				delete(m.copying, p.key)
				m.mu.Unlock()
			}
		}
	}
}

func (m *Manager) next() *pending {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queue.Len() == 0 {
		return nil
	}
	p := heap.Pop(&m.queue).(*pending)
	delete(m.pending, p.key)
	m.copying[p.key] = true
	return p
}

func (m *Manager) copy(media Media, key string) {
	if !m.sourceStillMatches(media) {
		return
	}
	if !m.hasSpace(media.Size) {
		return
	}
	m.mu.Lock()
	if !m.ensureRoomLocked(media.Size, key) {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	src, err := os.Open(media.SourcePath)
	if err != nil {
		return
	}
	defer src.Close()
	tmp, err := os.OpenFile(filepath.Join(m.dir, "."+key+".partial"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	copyErr := copyBytes(m.stop, tmp, src, media.Size)
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil || !m.sourceStillMatches(media) {
		return
	}

	path := filepath.Join(m.dir, key+".media")
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		return
	}
	if err := writeMetadata(filepath.Join(m.dir, key+".meta"), metadata{ID: media.ID, Kind: media.Kind, Year: media.Year}); err != nil {
		_ = os.Remove(path)
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)

	m.mu.Lock()
	if _, exists := m.entries[key]; exists {
		m.removeEntryLocked(key)
	}
	m.entries[key] = &entry{key: key, path: path, size: media.Size, kind: media.Kind, year: media.Year, lastUsed: now}
	m.bytes += media.Size
	m.ensureRoomLocked(0, key)
	m.mu.Unlock()
}

func (m *Manager) hasSpace(needed int64) bool {
	if m.minFree <= 0 {
		return true
	}
	free := availableBytes(m.dir)
	return free == 0 || free >= m.minFree+needed
}

func availableBytes(path string) int64 {
	if path == "" {
		return 0
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}

func copyBytes(stop <-chan struct{}, dst io.Writer, src io.Reader, size int64) error {
	buf := make([]byte, 1<<20)
	remaining := size
	for remaining > 0 {
		select {
		case <-stop:
			return errCacheStopping
		default:
		}
		want := int64(len(buf))
		if remaining < want {
			want = remaining
		}
		n, err := io.ReadFull(src, buf[:want])
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
			remaining -= int64(n)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) sourceStillMatches(media Media) bool {
	st, err := os.Stat(media.SourcePath)
	return err == nil && st.Size() == media.Size && st.ModTime().Equal(media.ModTime)
}

func (m *Manager) ensureRoomLocked(needed int64, keep string) bool {
	if needed > m.maxBytes || m.bytes+needed > m.maxBytes {
		candidates := make([]*entry, 0, len(m.entries))
		for _, e := range m.entries {
			if e.key != keep && e.active == 0 {
				candidates = append(candidates, e)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			pi, pj := priority(candidates[i].kind, candidates[i].year), priority(candidates[j].kind, candidates[j].year)
			if pi != pj {
				return pi < pj
			}
			return candidates[i].lastUsed.Before(candidates[j].lastUsed)
		})
		for _, e := range candidates {
			if m.bytes+needed <= m.maxBytes {
				break
			}
			m.removeEntryLocked(e.key)
		}
	}
	return needed <= m.maxBytes && m.bytes+needed <= m.maxBytes
}

func (m *Manager) removeEntryLocked(key string) {
	e, ok := m.entries[key]
	if !ok {
		return
	}
	if e.active > 0 {
		return
	}
	_ = os.Remove(e.path)
	_ = os.Remove(filepath.Join(m.dir, key+".meta"))
	m.bytes -= e.size
	if m.bytes < 0 {
		m.bytes = 0
	}
	delete(m.entries, key)
}

func (m *Manager) load() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return fmt.Errorf("media cache scan: %w", err)
	}
	for _, item := range entries {
		name := item.Name()
		if strings.HasSuffix(name, ".partial") {
			_ = os.Remove(filepath.Join(m.dir, name))
			continue
		}
		if !strings.HasSuffix(name, ".media") {
			continue
		}
		key := strings.TrimSuffix(name, ".media")
		path := filepath.Join(m.dir, name)
		st, statErr := os.Stat(path)
		if statErr != nil || st.Size() <= 0 || st.Size() > m.maxBytes {
			_ = os.Remove(path)
			_ = os.Remove(filepath.Join(m.dir, key+".meta"))
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(m.dir, key+".meta"))
		var meta metadata
		if readErr != nil || json.Unmarshal(data, &meta) != nil {
			_ = os.Remove(path)
			_ = os.Remove(filepath.Join(m.dir, key+".meta"))
			continue
		}
		m.entries[key] = &entry{key: key, path: path, size: st.Size(), kind: meta.Kind, year: meta.Year, lastUsed: st.ModTime()}
		m.bytes += st.Size()
	}
	m.ensureRoomLocked(0, "")
	return nil
}

func writeMetadata(path string, meta metadata) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".media-meta-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func cacheKey(media Media) string {
	h := sha256.New()
	identity := media.ID
	if identity == "" {
		identity = media.SourcePath
	}
	// Do not include the absolute source path. Stable catalog IDs let a cache
	// survive a NAS remount or an import path mapping when the file version is
	// unchanged.
	_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d", identity, media.Size, media.ModTime.UnixNano())
	return hex.EncodeToString(h.Sum(nil))
}

// priority is intentionally type-first: ebooks and audiobooks remain useful
// offline for much longer than a transient movie download. Release year then
// keeps newer movies ahead of older movies when their types compete for space.
func priority(kind api.MediaKind, year int) int {
	base := 100
	switch kind {
	case api.KindEbook:
		base = 500
	case api.KindAudiobook:
		base = 480
	case api.KindMovie:
		base = 300
	case api.KindDocumentary:
		base = 250
	case api.KindTVShow:
		base = 200
	}
	if year < 2000 {
		year = 2000
	}
	if year > 2100 {
		year = 2100
	}
	return base + year - 2000
}
