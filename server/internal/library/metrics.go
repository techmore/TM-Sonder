package library

import (
	"sync"
	"time"
)

const (
	readGet = iota
	readItems
	readInternalItems
	readProgress
	readOperationCount
)

var readOperationNames = [readOperationCount]string{"get", "items", "internalItems", "progress"}

// ReadOperationMetric measures a complete in-memory catalog read, including
// waiting for the store lock, copying, and sorting when applicable. It does not
// measure filesystem persistence, SQL queries, HTTP transport, or playback.
type ReadOperationMetric struct {
	Operation string  `json:"operation"`
	Count     uint64  `json:"count"`
	AverageMs float64 `json:"averageMs"`
	MaxMs     float64 `json:"maxMs"`
	LastMs    float64 `json:"lastMs"`
}

// ReadMetricsReport is process-local: every new Store starts fresh counters.
type ReadMetricsReport struct {
	StartedAt  time.Time             `json:"startedAt"`
	Operations []ReadOperationMetric `json:"operations"`
}

type readTiming struct {
	count uint64
	total time.Duration
	max   time.Duration
	last  time.Duration
}

// Fixed-size counters never retain item IDs or per-request samples. The metrics
// lock is independent of Store.mu and is only taken after a read releases it.
type catalogReadMetrics struct {
	mu        sync.Mutex
	startedAt time.Time
	timings   [readOperationCount]readTiming
}

func (m *catalogReadMetrics) record(operation int, elapsed time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &m.timings[operation]
	t.count++
	t.total += elapsed
	t.last = elapsed
	if elapsed > t.max {
		t.max = elapsed
	}
}

// ReadMetrics does not count itself as a catalog read. Callers receive their
// own report and can safely modify it without altering subsequent snapshots.
func (s *Store) ReadMetrics() ReadMetricsReport {
	m := &s.readMetrics
	m.mu.Lock()
	defer m.mu.Unlock()
	report := ReadMetricsReport{
		StartedAt:  m.startedAt,
		Operations: make([]ReadOperationMetric, readOperationCount),
	}
	for i, t := range m.timings {
		metric := ReadOperationMetric{Operation: readOperationNames[i], Count: t.count}
		if t.count > 0 {
			metric.AverageMs = float64(t.total) / float64(t.count) / float64(time.Millisecond)
			metric.MaxMs = float64(t.max) / float64(time.Millisecond)
			metric.LastMs = float64(t.last) / float64(time.Millisecond)
		}
		report.Operations[i] = metric
	}
	return report
}
