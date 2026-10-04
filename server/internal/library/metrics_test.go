package library

import (
	"sync"
	"testing"
	"time"
)

func TestReadMetricsStartEmptyAndSnapshotsAreIndependent(t *testing.T) {
	before := time.Now()
	s := New()
	report := s.ReadMetrics()
	if report.StartedAt.Before(before) || report.StartedAt.After(time.Now()) {
		t.Fatalf("invalid startup time: %v", report.StartedAt)
	}
	if len(report.Operations) != readOperationCount {
		t.Fatalf("operations = %d", len(report.Operations))
	}
	for i, metric := range report.Operations {
		if metric.Operation != readOperationNames[i] || metric.Count != 0 || metric.AverageMs != 0 || metric.MaxMs != 0 || metric.LastMs != 0 {
			t.Fatalf("unexpected empty metric: %+v", metric)
		}
	}
	report.Operations[0].Count = 999
	if s.ReadMetrics().Operations[0].Count != 0 {
		t.Fatal("snapshot altered counters, or reading metrics recorded a read")
	}
	s.Get("missing")
	if New().ReadMetrics().Operations[0].Count != 0 {
		t.Fatal("a new store inherited previous counters")
	}
}

func TestReadMetricsCalculateDurations(t *testing.T) {
	s := New()
	s.readMetrics.record(readGet, 2*time.Millisecond)
	s.readMetrics.record(readGet, 6*time.Millisecond)
	s.readMetrics.record(readGet, time.Millisecond)
	m := s.ReadMetrics().Operations[readGet]
	if m.Count != 3 || m.AverageMs != 3 || m.MaxMs != 6 || m.LastMs != 1 {
		t.Fatalf("unexpected durations: %+v", m)
	}
}

func TestReadMetricsConcurrentReadsAndSnapshots(t *testing.T) {
	s := New()
	s.Upsert(testItem("a", "Alpha"))
	const workers, reads = 8, 40
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < reads; i++ {
				s.Get("a")
				s.Get("missing")
				s.Items()
				s.InternalItems()
				s.Progress()
				s.ReadMetrics()
			}
		}()
	}
	wg.Wait()
	for i, m := range s.ReadMetrics().Operations {
		expected := uint64(workers * reads)
		if i == readGet {
			expected *= 2
		}
		if m.Count != expected {
			t.Errorf("%s count = %d, want %d (Items must not also count InternalItems)", m.Operation, m.Count, expected)
		}
		if m.AverageMs < 0 || m.MaxMs < m.AverageMs || m.LastMs < 0 || m.MaxMs < m.LastMs {
			t.Errorf("invalid timing: %+v", m)
		}
	}
}
