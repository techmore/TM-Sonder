package probe

import (
	"context"
	"sync"
	"time"
)

type cacheEntry struct {
	size   int64
	mod    time.Time
	result *Result
}

// Cache memoizes probe results keyed by path, invalidated by size+mtime.
// It is safe for concurrent use.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ffprobe string
}

func NewCache(ffprobePath string) *Cache {
	return &Cache{entries: make(map[string]cacheEntry), ffprobe: ffprobePath}
}

// ProbeResult returns the cached result when the file is unchanged, else it
// re-probes and refreshes the cache.
func (c *Cache) ProbeResult(ctx context.Context, path string, size int64, mod time.Time) (*Result, error) {
	c.mu.RLock()
	if e, ok := c.entries[path]; ok && e.size == size && e.mod.Equal(mod) {
		c.mu.RUnlock()
		return e.result, nil
	}
	c.mu.RUnlock()

	res, err := Probe(ctx, c.ffprobe, path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.entries[path] = cacheEntry{size: size, mod: mod, result: res}
	c.mu.Unlock()
	return res, nil
}

// probeJob is one pending unit of work handed to RunPool.
type probeJob struct {
	Path string
	Size int64
	Mod  time.Time
}

type probeOutcome struct {
	job    probeJob
	result *Result
	err    error
}

// RunPool probes jobs with `workers` concurrent goroutines, invoking onDone
// per completed outcome. Workers defaults to 2. Blocks until all work drains.
func (c *Cache) RunPool(ctx context.Context, workers int, jobs []probeJob, onDone func(job probeJob, res *Result, err error)) {
	if workers < 1 {
		workers = 2
	}
	in := make(chan probeJob)
	out := make(chan probeOutcome)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				res, err := c.ProbeResult(ctx, j.Path, j.Size, j.Mod)
				out <- probeOutcome{job: j, result: res, err: err}
			}
		}()
	}

	go func() {
		defer close(out)
		for _, j := range jobs {
			select {
			case in <- j:
			case <-ctx.Done():
				close(in)
				return
			}
		}
		close(in)
	}()

	for i := 0; i < len(jobs); i++ {
		select {
		case o := <-out:
			onDone(o.job, o.result, o.err)
		case <-ctx.Done():
			return
		}
	}
	wg.Wait()
}
