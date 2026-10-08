package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"
)

// Share one bounded cache across routes: many episodes reference the same
// show poster. Never put original media files into this cache.
const artworkCacheLimit = 64 << 20
const artworkEntryLimit = 8 << 20
const artworkCacheTTL = 10 * time.Minute

type cachedArtwork struct {
	data        []byte
	modified    time.Time
	accessed    time.Time
	loaded      time.Time
	contentType string
}

var coverCache = struct {
	sync.Mutex
	entries map[string]cachedArtwork
	bytes   int
}{entries: make(map[string]cachedArtwork)}

func getCachedArtwork(path string) (cachedArtwork, bool) {
	coverCache.Lock()
	defer coverCache.Unlock()
	entry, ok := coverCache.entries[path]
	if !ok {
		return cachedArtwork{}, false
	}
	if time.Since(entry.loaded) > artworkCacheTTL {
		delete(coverCache.entries, path)
		coverCache.bytes -= len(entry.data)
		return cachedArtwork{}, false
	}
	entry.accessed = time.Now()
	coverCache.entries[path] = entry
	return entry, true
}

func rememberArtwork(path string, data []byte, modified time.Time, contentType string) {
	if len(data) == 0 || len(data) > artworkEntryLimit {
		return
	}
	coverCache.Lock()
	defer coverCache.Unlock()
	if previous, ok := coverCache.entries[path]; ok {
		coverCache.bytes -= len(previous.data)
	}
	delete(coverCache.entries, path)
	for coverCache.bytes+len(data) > artworkCacheLimit {
		oldestPath := ""
		var oldest time.Time
		for key, entry := range coverCache.entries {
			if oldestPath == "" || entry.accessed.Before(oldest) {
				oldestPath, oldest = key, entry.accessed
			}
		}
		if oldestPath == "" {
			break
		}
		coverCache.bytes -= len(coverCache.entries[oldestPath].data)
		delete(coverCache.entries, oldestPath)
	}
	now := time.Now()
	coverCache.entries[path] = cachedArtwork{data: data, modified: modified, accessed: now, loaded: now, contentType: contentType}
	coverCache.bytes += len(data)
}

func serveCachedArtwork(w http.ResponseWriter, r *http.Request, name string, entry cachedArtwork) {
	w.Header().Set("Content-Type", entry.contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, name, entry.modified, bytes.NewReader(entry.data))
}

func readArtworkForCache(reader io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(reader, artworkEntryLimit+1))
}
