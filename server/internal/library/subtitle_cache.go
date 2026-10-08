package library

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SubtitleCacheBase ties maintained subtitles to this exact source revision.
// Different cuts, replaced files and changed mounts do not reuse stale captions.
func SubtitleCacheBase(dir string, item *Item) string {
	if dir == "" || item.ID == "" || filepath.Base(item.ID) != item.ID || item.ID == "." || item.ID == ".." {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", item.FilePath, item.SizeBytes, item.ModTime.UnixNano())))
	base := strings.TrimSuffix(filepath.Base(item.FilePath), filepath.Ext(item.FilePath))
	if len(base) > 160 {
		base = strings.ToValidUTF8(base[:160], "_")
	}
	return filepath.Join(dir, item.ID, fmt.Sprintf("%s.%x", base, sum[:8]))
}

func CachedSidecarPaths(dir string, item *Item) []string {
	base := SubtitleCacheBase(dir, item)
	if base == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Dir(base))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), filepath.Base(base)+".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".vtt" && ext != ".srt" {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() > 0 {
			out = append(out, filepath.Join(filepath.Dir(base), e.Name()))
		}
	}
	sort.Strings(out)
	return out
}
