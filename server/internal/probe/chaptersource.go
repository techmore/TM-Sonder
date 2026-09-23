package probe

import (
	"context"
	"os"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
)

// maxChapterEntries bounds the chapter cache (one entry per audiobook).
const maxChapterEntries = 512

// ChapterSource implements httpapi.ChapterProvider: it extracts audiobook
// chapters on demand, cached per file version (path + mtime) so repeat detail
// requests don't re-run ffprobe. The cache is bounded and returns copies.
type ChapterSource struct {
	mu      sync.Mutex
	getItem func(itemID string) (path string, mod time.Time, ok bool)
	ffprobe string
	cache   map[string]chapterCacheEntry
	order   []string
}

type chapterCacheEntry struct {
	mod      time.Time
	chapters []api.AudiobookChapter
}

// NewChapterSource builds a chapter source. getItem resolves an item ID to its
// file path and modification time (typically library.Store.Get).
func NewChapterSource(getItem func(itemID string) (string, time.Time, bool), ffprobe string) *ChapterSource {
	return &ChapterSource{
		getItem: getItem,
		ffprobe: ffprobe,
		cache:   make(map[string]chapterCacheEntry),
	}
}

func (c *ChapterSource) ChaptersFor(ctx context.Context, itemID string) ([]api.AudiobookChapter, error) {
	path, mod, ok := c.getItem(itemID)
	if !ok {
		return nil, os.ErrNotExist
	}
	c.mu.Lock()
	if e, ok := c.cache[itemID]; ok && e.mod.Equal(mod) {
		out := cloneChapters(e.chapters)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	chapters, err := Chapters(ctx, c.ffprobe, path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.storeLocked(itemID, chapterCacheEntry{mod: mod, chapters: chapters})
	c.mu.Unlock()
	return cloneChapters(chapters), nil
}

// storeLocked inserts or replaces an entry, evicting the oldest insertion when
// the cache is full. Caller must hold c.mu.
func (c *ChapterSource) storeLocked(itemID string, e chapterCacheEntry) {
	if _, exists := c.cache[itemID]; !exists {
		if len(c.order) >= maxChapterEntries {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.cache, oldest)
		}
		c.order = append(c.order, itemID)
	}
	c.cache[itemID] = e
}

func cloneChapters(in []api.AudiobookChapter) []api.AudiobookChapter {
	if in == nil {
		return nil
	}
	out := make([]api.AudiobookChapter, len(in))
	copy(out, in)
	for i := range out {
		if out[i].EndSeconds != nil {
			v := *out[i].EndSeconds
			out[i].EndSeconds = &v
		}
	}
	return out
}
