package probe

import (
	"context"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
)

// maxCacheEntries bounds the probe cache so a long-lived server that churns
// through many files cannot grow it without limit.
const maxCacheEntries = 4096

type cacheEntry struct {
	size   int64
	mod    time.Time
	result *Result
}

// Cache memoizes probe results keyed by path, invalidated by size+mtime.
// It is safe for concurrent use and evicts least-recently-inserted entries
// once maxCacheEntries is reached.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	order   []string // insertion order for FIFO eviction
	ffprobe string
}

func NewCache(ffprobePath string) *Cache {
	return &Cache{entries: make(map[string]cacheEntry), ffprobe: ffprobePath}
}

// ProbeResult returns a copy of the cached result when the file is unchanged,
// else it re-probes and refreshes the cache. Callers own the returned value.
func (c *Cache) ProbeResult(ctx context.Context, path string, size int64, mod time.Time) (*Result, error) {
	c.mu.RLock()
	if e, ok := c.entries[path]; ok && e.size == size && e.mod.Equal(mod) {
		c.mu.RUnlock()
		return e.result.clone(), nil
	}
	c.mu.RUnlock()

	res, err := Probe(ctx, c.ffprobe, path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.storeLocked(path, cacheEntry{size: size, mod: mod, result: res})
	c.mu.Unlock()
	return res.clone(), nil
}

// storeLocked inserts or replaces an entry, evicting the oldest insertion when
// the cache is full. Caller must hold c.mu.
func (c *Cache) storeLocked(path string, e cacheEntry) {
	if _, exists := c.entries[path]; !exists {
		if len(c.order) >= maxCacheEntries {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, path)
	}
	c.entries[path] = e
}

// Len reports the number of cached entries (introspection/tests).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// clone deep-copies a Result so cached state can never be mutated by callers.
func (r *Result) clone() *Result {
	if r == nil {
		return nil
	}
	c := *r
	c.Width = cloneIntPtr(r.Width)
	c.Height = cloneIntPtr(r.Height)
	c.Codec = cloneStringPtr(r.Codec)
	c.Bitrate = cloneIntPtr(r.Bitrate)
	c.AudioTracks = cloneTracks(r.AudioTracks)
	c.SubtitleTracks = cloneTracks(r.SubtitleTracks)
	if r.AudioCodecs != nil {
		c.AudioCodecs = append([]string(nil), r.AudioCodecs...)
	}
	if r.AudioChannels != nil {
		c.AudioChannels = append([]int(nil), r.AudioChannels...)
	}
	if r.AudioBitrates != nil {
		c.AudioBitrates = append([]int(nil), r.AudioBitrates...)
	}
	return &c
}

func cloneTracks(in []api.PlaybackTrack) []api.PlaybackTrack {
	if in == nil {
		return nil
	}
	out := make([]api.PlaybackTrack, len(in))
	copy(out, in)
	for i := range out {
		out[i].LanguageCode = cloneStringPtr(out[i].LanguageCode)
		out[i].URL = cloneStringPtr(out[i].URL)
	}
	return out
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
