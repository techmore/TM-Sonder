package library

import (
	"sort"
	"time"

	"tm-sonder/server/internal/api"
)

// ReadingState returns a stable copy of the ordered queue and per-book state.
func (s *Store) ReadingState() api.ReadingState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := api.ReadingState{
		Queue:   append([]string(nil), s.readingQueue...),
		Records: make([]api.BookReadingRecord, 0, len(s.reading)),
	}
	for _, record := range s.reading {
		state.Records = append(state.Records, cloneReadingRecord(*record))
	}
	sort.Slice(state.Records, func(i, j int) bool {
		return state.Records[i].ItemID < state.Records[j].ItemID
	})
	return state
}

// SetBookReadingFlags updates the queue and like state without changing the
// catalog generation; reading state is served from its own small endpoint.
func (s *Store) SetBookReadingFlags(itemID string, queued, liked *bool, now time.Time) (api.BookReadingRecord, bool) {
	if itemID == "" {
		return api.BookReadingRecord{}, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[itemID]; !ok {
		return api.BookReadingRecord{}, false
	}
	if s.reading == nil {
		s.reading = make(map[string]*api.BookReadingRecord)
	}
	record := s.reading[itemID]
	if record == nil {
		record = &api.BookReadingRecord{ItemID: itemID, Reads: []api.BookReadRun{}}
	}
	changed := false
	if queued != nil {
		index := stringIndex(s.readingQueue, itemID)
		if *queued && index < 0 {
			s.readingQueue = append(s.readingQueue, itemID)
			t := now
			record.QueuedAt = &t
			changed = true
		} else if !*queued && index >= 0 {
			s.readingQueue = append(s.readingQueue[:index], s.readingQueue[index+1:]...)
			record.QueuedAt = nil
			changed = true
		}
	}
	if liked != nil {
		currentlyLiked := record.LikedAt != nil
		if *liked != currentlyLiked {
			if *liked {
				t := now
				record.LikedAt = &t
			} else {
				record.LikedAt = nil
			}
			changed = true
		}
	}
	if changed {
		record.UpdatedAt = now
		s.reading[itemID] = record
	}
	return cloneReadingRecord(*record), true
}

// ReorderReadingQueue reorders known queued IDs; omitted IDs retain their
// relative order, and unqueued or unknown IDs are ignored.
func (s *Store) ReorderReadingQueue(itemIDs []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed := make(map[string]bool, len(s.readingQueue))
	for _, id := range s.readingQueue {
		allowed[id] = true
	}
	ordered := make([]string, 0, len(s.readingQueue))
	seen := make(map[string]bool, len(s.readingQueue))
	for _, id := range itemIDs {
		if allowed[id] && !seen[id] {
			ordered = append(ordered, id)
			seen[id] = true
		}
	}
	for _, id := range s.readingQueue {
		if !seen[id] {
			ordered = append(ordered, id)
		}
	}
	s.readingQueue = ordered
	return append([]string(nil), ordered...)
}

// RecordReadSession upserts a cumulative client heartbeat. Repeated or
// out-of-order requests cannot double-count time. A completed read is retained
// as history, and the next play creates a new run.
func (s *Store) RecordReadSession(itemID string, update api.ReadingSessionUpdate, now time.Time) (api.BookReadingRecord, bool) {
	if itemID == "" || update.SessionID == "" || update.ActiveSeconds < 0 || update.MediaSeconds < 0 {
		return api.BookReadingRecord{}, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if it, ok := s.items[itemID]; !ok || it.Kind != api.KindAudiobook {
		return api.BookReadingRecord{}, false
	}
	if s.reading == nil {
		s.reading = make(map[string]*api.BookReadingRecord)
	}
	record := s.reading[itemID]
	if record == nil {
		record = &api.BookReadingRecord{ItemID: itemID, Reads: []api.BookReadRun{}}
	}
	if record.Reads == nil {
		record.Reads = []api.BookReadRun{}
	}
	readIndex, sessionIndex := -1, -1
	for ri := range record.Reads {
		for si := range record.Reads[ri].Sessions {
			if record.Reads[ri].Sessions[si].ID == update.SessionID {
				readIndex, sessionIndex = ri, si
				break
			}
		}
		if readIndex >= 0 {
			break
		}
	}
	if readIndex < 0 {
		if last := len(record.Reads) - 1; last >= 0 && record.Reads[last].CompletedAt == nil {
			readIndex = last
		} else {
			record.Reads = append(record.Reads, api.BookReadRun{
				ID: api.NewID(), StartedAt: now, Sessions: []api.ReadingSession{},
			})
			readIndex = len(record.Reads) - 1
		}
		record.Reads[readIndex].Sessions = append(record.Reads[readIndex].Sessions, api.ReadingSession{
			ID: update.SessionID, StartedAt: now, UpdatedAt: now,
		})
		sessionIndex = len(record.Reads[readIndex].Sessions) - 1
	}

	run := &record.Reads[readIndex]
	session := &run.Sessions[sessionIndex]
	if update.ActiveSeconds > session.ActiveSeconds {
		run.ActiveSeconds += update.ActiveSeconds - session.ActiveSeconds
		session.ActiveSeconds = update.ActiveSeconds
	}
	if update.MediaSeconds > session.MediaSeconds {
		run.MediaSeconds += update.MediaSeconds - session.MediaSeconds
		session.MediaSeconds = update.MediaSeconds
	}
	if now.After(session.UpdatedAt) {
		session.UpdatedAt = now
	}
	if update.Completed && run.CompletedAt == nil {
		completedAt := now
		run.CompletedAt = &completedAt
	}
	record.UpdatedAt = now
	s.reading[itemID] = record
	return cloneReadingRecord(*record), true
}

func cloneReadingRecord(in api.BookReadingRecord) api.BookReadingRecord {
	out := in
	out.QueuedAt = cloneTimePtr(in.QueuedAt)
	out.LikedAt = cloneTimePtr(in.LikedAt)
	if in.Reads != nil {
		out.Reads = make([]api.BookReadRun, len(in.Reads))
		for i := range in.Reads {
			out.Reads[i] = in.Reads[i]
			out.Reads[i].CompletedAt = cloneTimePtr(in.Reads[i].CompletedAt)
			out.Reads[i].Sessions = append([]api.ReadingSession(nil), in.Reads[i].Sessions...)
		}
	}
	return out
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func stringIndex(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
