package enrich

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"tm-sonder/server/internal/library"
)

// Catalog is the slice of the library store the enrichment pass needs. It is
// deliberately narrow so this package does not depend on the HTTP layer.
type Catalog interface {
	InternalItems() []*library.Item
	Update(id string, fn func(*library.Item) bool) bool
}

// passWorkers bounds concurrent provider requests during an enrichment pass.
const passWorkers = 3

// RunPass fetches provider metadata for items missing a summary or still
// carrying only a generated thumbnail, applying results in place. Official
// posters override frame grabs; locally discovered artwork is never replaced.
// It honors ctx for cancellation and returns how many items changed.
func RunPass(ctx context.Context, logger *log.Logger, store Catalog, cacheRoot string) int {
	enricher := New(cacheRoot)

	type job struct{ item *library.Item }
	var candidates []*library.Item
	for _, it := range store.InternalItems() {
		needsSummary := it.Summary == ""
		needsOfficialPoster := it.PosterSource == "" || it.PosterSource == "thumbnail"
		if needsSummary || needsOfficialPoster {
			candidates = append(candidates, it)
		}
	}
	if logger != nil {
		logger.Printf("enrichment pass: %d candidate item(s)", len(candidates))
	}

	in := make(chan job)
	var wg sync.WaitGroup
	var updated atomic.Int64
	var done atomic.Int64

	for i := 0; i < passWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				it := j.item
				input := Input{
					Title:            it.Title,
					Kind:             string(it.Kind),
					Year:             it.Year,
					Studio:           it.Studio,
					Edition:          derefStr(it.Edition),
					MetadataIDSource: derefStr(it.MetadataIDSource),
					MetadataID:       derefStr(it.MetadataID),
				}
				if it.ShowTitle != nil {
					input.ShowTitle = *it.ShowTitle
				}
				if it.SeasonNumber != nil && it.EpisodeNumber != nil {
					input.Season, input.Episode = *it.SeasonNumber, *it.EpisodeNumber
				}

				result, err := enricher.Enrich(ctx, input)
				if err == nil && result != nil &&
					(result.Summary != "" || len(result.Tags) > 0 || result.PosterPath != "") {
					// Update under the store lock so a concurrent probe,
					// thumbnail, or progress write is not clobbered.
					var changed bool
					if store.Update(it.ID, func(cur *library.Item) bool {
						changed = applyResult(cur, result)
						return changed
					}) && changed {
						updated.Add(1)
					}
				}
				if n := done.Add(1); logger != nil && n%25 == 0 {
					logger.Printf("enrichment: %d/%d processed (%d updated)",
						n, len(candidates), updated.Load())
				}
			}
		}()
	}

feed:
	for _, it := range candidates {
		select {
		case in <- job{item: it}:
		case <-ctx.Done():
			break feed
		}
	}
	close(in)
	wg.Wait()
	return int(updated.Load())
}

// applyResult merges one provider result into a catalog item, returning whether
// anything changed. Locally discovered artwork is never replaced, and official
// posters override generated thumbnails. Art is installed Plex-style into the
// media folder when possible so other tools see it; the cache copy is the
// fallback.
func applyResult(cur *library.Item, result *Enrichment) bool {
	changed := false
	if result.Summary != "" && cur.Summary == "" {
		cur.Summary = result.Summary
		changed = true
	}
	if result.Author != "" && cur.Author == nil {
		a := result.Author
		cur.Author = &a
		changed = true
	}
	if result.Narrator != "" && cur.Narrator == nil {
		n := result.Narrator
		cur.Narrator = &n
		changed = true
	}
	for _, genre := range result.Genres {
		if !containsFold(cur.Genres, genre) {
			cur.Genres = append(cur.Genres, genre)
			changed = true
		}
	}
	for _, tag := range result.Tags {
		dup := false
		for _, existing := range cur.Tags {
			if strings.EqualFold(existing, tag) {
				dup = true
				break
			}
		}
		if !dup {
			cur.Tags = append(cur.Tags, tag)
			changed = true
		}
	}
	if result.PosterPath != "" && cur.PosterSource != "local" {
		installed := result.PosterPath
		posterDest, _ := library.PlexArtPaths(cur)
		if dest, err := library.CopyArtTo(result.PosterPath, posterDest); err == nil {
			installed = dest
		}
		if cur.PosterPath != installed || cur.PosterSource != result.Provider {
			cur.PosterPath = installed
			u := "/artwork/poster/" + cur.ID
			cur.PosterURL = &u
			cur.PosterSource = result.Provider
			changed = true
		}
	}
	if result.BackdropPath != "" && cur.BackdropPath == "" {
		_, fanartDest := library.PlexArtPaths(cur)
		installed := result.BackdropPath
		if dest, err := library.CopyArtTo(result.BackdropPath, fanartDest); err == nil {
			installed = dest
		}
		cur.BackdropPath = installed
		u := "/artwork/backdrop/" + cur.ID
		cur.BackdropURL = &u
		changed = true
	}
	return changed
}

// containsFold reports whether list already holds value, case-insensitively.
func containsFold(list []string, value string) bool {
	for _, existing := range list {
		if strings.EqualFold(existing, value) {
			return true
		}
	}
	return false
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
