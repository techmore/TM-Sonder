package probe

import (
	"context"
	"os"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
)

// ChapterSource implements httpapi.ChapterProvider: it extracts audiobook
// chapters on demand, cached per file version (path + mtime) so repeat detail
// requests don't re-run ffprobe.
type ChapterSource struct {
	mu      sync.Mutex
	getItem func(itemID string) (path string, mod time.Time, ok bool)
	ffprobe string
	cache   map[string]chapterCacheEntry
}

type chapterCacheEntry struct {
	mod      time.Time
	chapters []api.AudiobookChapter
}

// NewChapterSource builds a chapter source. getItem resolves an item ID to its
// file path and modification time (typically library.Store.Get).
func NewChapterSource(getItem func(itemID string) (string, time.Time, bool), ffprobe string) *ChapterSource {
	return &ChapterSource{getItem: getItem, ffprobe: ffprobe}
}

func (c *ChapterSource) ChaptersFor(itemID string) ([]api.AudiobookChapter, error) {
	path, mod, ok := c.getItem(itemID)
	if !ok {
		return nil, os.ErrNotExist
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]chapterCacheEntry)
	}
	if e, ok := c.cache[itemID]; ok && e.mod.Equal(mod) {
		c.mu.Unlock()
		return e.chapters, nil
	}
	c.mu.Unlock()

	chapters, err := Chapters(context.Background(), c.ffprobe, path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[itemID] = chapterCacheEntry{mod: mod, chapters: chapters}
	c.mu.Unlock()
	return chapters, nil
}
