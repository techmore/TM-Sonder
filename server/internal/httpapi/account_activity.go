package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
)

// accountActivityStore keeps bookmarks and playback progress private to an
// account. The catalog store remains shared, while this small sidecar is
// migrated from the old shared activity for the original owner on first use.
type accountActivityStore struct {
	mu         sync.RWMutex
	path       string
	accounts   map[string]accountActivity
	generation map[string]uint64
}

type accountActivity struct {
	Progress map[string]api.ProgressRecord `json:"progress"`
	Reading  api.ReadingState              `json:"reading"`
}

type accountActivityFile struct {
	Version  int                        `json:"version"`
	Accounts map[string]accountActivity `json:"accounts"`
}

func openAccountActivity(path string) (*accountActivityStore, error) {
	store := &accountActivityStore{
		path: path, accounts: make(map[string]accountActivity),
		generation: make(map[string]uint64),
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read account activity: %w", err)
	}
	var file accountActivityFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse account activity: %w", err)
	}
	if file.Version != 1 || file.Accounts == nil {
		return nil, fmt.Errorf("unsupported or invalid account activity file")
	}
	for username, activity := range file.Accounts {
		activity = cloneAccountActivity(activity)
		store.accounts[username] = activity
		store.generation[username] = 1
	}
	return store, nil
}

func (s *Server) ensureAccountActivity(username string) error {
	if s.activityErr != nil {
		return s.activityErr
	}
	if s.accountActivity == nil || s.accounts == nil || username == "" {
		return errors.New("account activity unavailable")
	}
	if s.accountActivity.has(username) {
		return nil
	}
	return s.accountActivity.ensure(username, s.accounts.Username(), s.store.Progress(), s.store.ReadingState())
}

func (s *Server) accountActivityUsername(r *http.Request) string {
	username, ok := s.sessionUsername(r)
	if !ok {
		return ""
	}
	return username
}

func (s *Server) accountActivityForRequest(r *http.Request) (string, accountActivity, bool, error) {
	username, scoped, err := s.ensureAccountActivityForRequest(r)
	if err != nil || !scoped {
		return username, accountActivity{}, scoped, err
	}
	return username, s.accountActivity.snapshot(username), true, nil
}

func (s *Server) ensureAccountActivityForRequest(r *http.Request) (string, bool, error) {
	username := s.accountActivityUsername(r)
	if username == "" {
		return "", false, nil
	}
	if err := s.ensureAccountActivity(username); err != nil {
		return username, true, err
	}
	return username, true, nil
}

func (s *accountActivityStore) ensure(username, owner string, legacyProgress []api.ProgressRecord, legacyReading api.ReadingState) error {
	if s == nil || username == "" {
		return errors.New("account activity unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[username]; ok {
		return nil
	}
	activity := emptyAccountActivity()
	if username == owner {
		for _, record := range legacyProgress {
			if record.ItemID != "" {
				activity.Progress[record.ItemID] = record
			}
		}
		activity.Reading = cloneReadingState(legacyReading)
	}
	updated := cloneActivityAccounts(s.accounts)
	updated[username] = activity
	if err := s.writeLocked(updated); err != nil {
		return err
	}
	s.accounts = updated
	s.generation[username] = 1
	return nil
}

func emptyAccountActivity() accountActivity {
	return accountActivity{
		Progress: make(map[string]api.ProgressRecord),
		Reading:  api.ReadingState{Queue: []string{}, Records: []api.BookReadingRecord{}},
	}
}

func cloneAccountActivity(in accountActivity) accountActivity {
	out := emptyAccountActivity()
	for id, record := range in.Progress {
		record.AudioTrackID = cloneStringPtr(record.AudioTrackID)
		record.SubtitleTrackID = cloneStringPtr(record.SubtitleTrackID)
		if record.SubtitlesEnabled != nil {
			value := *record.SubtitlesEnabled
			record.SubtitlesEnabled = &value
		}
		out.Progress[id] = record
	}
	out.Reading = cloneReadingState(in.Reading)
	return out
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneActivityAccounts(in map[string]accountActivity) map[string]accountActivity {
	out := make(map[string]accountActivity, len(in)+1)
	for username, activity := range in {
		out[username] = cloneAccountActivity(activity)
	}
	return out
}

func cloneReadingState(in api.ReadingState) api.ReadingState {
	out := api.ReadingState{
		Queue:   append([]string{}, in.Queue...),
		Records: make([]api.BookReadingRecord, 0, len(in.Records)),
	}
	for _, record := range in.Records {
		out.Records = append(out.Records, cloneAccountReadingRecord(record))
	}
	return out
}

func cloneAccountReadingRecord(in api.BookReadingRecord) api.BookReadingRecord {
	out := in
	out.QueuedAt = cloneAccountTimePtr(in.QueuedAt)
	out.LikedAt = cloneAccountTimePtr(in.LikedAt)
	if in.Reads != nil {
		out.Reads = make([]api.BookReadRun, len(in.Reads))
		for i, read := range in.Reads {
			out.Reads[i] = read
			out.Reads[i].CompletedAt = cloneAccountTimePtr(read.CompletedAt)
			out.Reads[i].Sessions = append([]api.ReadingSession(nil), read.Sessions...)
		}
	}
	return out
}

func cloneAccountTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *accountActivityStore) writeLocked(accounts map[string]accountActivity) error {
	data, err := json.MarshalIndent(accountActivityFile{Version: 1, Accounts: accounts}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".sonder-account-activity-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

func (s *accountActivityStore) snapshot(username string) accountActivity {
	if s == nil {
		return emptyAccountActivity()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAccountActivity(s.accounts[username])
}

func (s *accountActivityStore) progressSnapshotWithGeneration(username string) (map[string]api.ProgressRecord, uint64) {
	progress := make(map[string]api.ProgressRecord)
	if s == nil {
		return progress, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	activity := s.accounts[username]
	for itemID, record := range activity.Progress {
		record.AudioTrackID = cloneStringPtr(record.AudioTrackID)
		record.SubtitleTrackID = cloneStringPtr(record.SubtitleTrackID)
		if record.SubtitlesEnabled != nil {
			value := *record.SubtitlesEnabled
			record.SubtitlesEnabled = &value
		}
		progress[itemID] = record
	}
	return progress, s.generation[username]
}

func (s *accountActivityStore) has(username string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.accounts[username]
	return ok
}

func (s *accountActivityStore) progressFor(username, itemID string) (api.ProgressRecord, bool) {
	if s == nil {
		return api.ProgressRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	activity, exists := s.accounts[username]
	if !exists {
		return api.ProgressRecord{}, false
	}
	record, ok := activity.Progress[itemID]
	if ok {
		record.AudioTrackID = cloneStringPtr(record.AudioTrackID)
		record.SubtitleTrackID = cloneStringPtr(record.SubtitleTrackID)
		if record.SubtitlesEnabled != nil {
			value := *record.SubtitlesEnabled
			record.SubtitlesEnabled = &value
		}
	}
	return record, ok
}

func (s *accountActivityStore) setProgress(username string, record api.ProgressRecord) (bool, error) {
	if record.ItemID == "" {
		return false, nil
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	if record.ID == "" {
		record.ID = api.NewID()
	}
	changed := false
	err := s.update(username, func(activity *accountActivity) error {
		if previous, ok := activity.Progress[record.ItemID]; ok && !record.UpdatedAt.After(previous.UpdatedAt) {
			return nil
		}
		activity.Progress[record.ItemID] = record
		changed = true
		return nil
	})
	return changed, err
}

func (s *accountActivityStore) update(username string, mutate func(*accountActivity) error) error {
	if s == nil || username == "" {
		return errors.New("account activity unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.accounts[username]
	if !ok {
		return errors.New("account activity not initialized")
	}
	updated := cloneActivityAccounts(s.accounts)
	activity := updated[username]
	if err := mutate(&activity); err != nil {
		return err
	}
	updated[username] = activity
	if err := s.writeLocked(updated); err != nil {
		return err
	}
	s.accounts = updated
	s.generation[username]++
	return nil
}

func (s *accountActivityStore) setBookReadingFlags(username, itemID string, queued, liked *bool, now time.Time) (api.BookReadingRecord, error) {
	var result api.BookReadingRecord
	err := s.update(username, func(activity *accountActivity) error {
		if itemID == "" || (queued == nil && liked == nil) {
			return errors.New("invalid reading update")
		}
		record := findReadingRecord(&activity.Reading, itemID)
		changed := false
		if queued != nil {
			index := activityStringIndex(activity.Reading.Queue, itemID)
			if *queued && index < 0 {
				activity.Reading.Queue = append(activity.Reading.Queue, itemID)
				record.QueuedAt = cloneAccountTimePtr(&now)
				changed = true
			} else if !*queued && index >= 0 {
				activity.Reading.Queue = append(activity.Reading.Queue[:index], activity.Reading.Queue[index+1:]...)
				record.QueuedAt = nil
				changed = true
			}
		}
		if liked != nil {
			isLiked := record.LikedAt != nil
			if *liked != isLiked {
				if *liked {
					record.LikedAt = cloneAccountTimePtr(&now)
				} else {
					record.LikedAt = nil
				}
				changed = true
			}
		}
		if changed {
			record.UpdatedAt = now
			putReadingRecord(&activity.Reading, record)
		}
		result = cloneAccountReadingRecord(record)
		return nil
	})
	return result, err
}

func activityStringIndex(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

func findReadingRecord(state *api.ReadingState, itemID string) api.BookReadingRecord {
	for _, record := range state.Records {
		if record.ItemID == itemID {
			return cloneAccountReadingRecord(record)
		}
	}
	return api.BookReadingRecord{ItemID: itemID, Reads: []api.BookReadRun{}}
}

func putReadingRecord(state *api.ReadingState, record api.BookReadingRecord) {
	for index := range state.Records {
		if state.Records[index].ItemID == record.ItemID {
			state.Records[index] = cloneAccountReadingRecord(record)
			return
		}
	}
	state.Records = append(state.Records, cloneAccountReadingRecord(record))
	sort.Slice(state.Records, func(i, j int) bool { return state.Records[i].ItemID < state.Records[j].ItemID })
}

func (s *accountActivityStore) reorderReadingQueue(username string, itemIDs []string) ([]string, error) {
	var queue []string
	err := s.update(username, func(activity *accountActivity) error {
		allowed := make(map[string]bool, len(activity.Reading.Queue))
		for _, id := range activity.Reading.Queue {
			allowed[id] = true
		}
		ordered := make([]string, 0, len(activity.Reading.Queue))
		seen := make(map[string]bool, len(activity.Reading.Queue))
		for _, id := range itemIDs {
			if allowed[id] && !seen[id] {
				ordered = append(ordered, id)
				seen[id] = true
			}
		}
		for _, id := range activity.Reading.Queue {
			if !seen[id] {
				ordered = append(ordered, id)
			}
		}
		activity.Reading.Queue = ordered
		queue = append([]string(nil), ordered...)
		return nil
	})
	return queue, err
}

func (s *accountActivityStore) recordReadSession(username, itemID string, update api.ReadingSessionUpdate, now time.Time) (api.BookReadingRecord, error) {
	var result api.BookReadingRecord
	err := s.update(username, func(activity *accountActivity) error {
		record := findReadingRecord(&activity.Reading, itemID)
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
			readIndex = len(record.Reads) - 1
			if readIndex < 0 || record.Reads[readIndex].CompletedAt != nil {
				record.Reads = append(record.Reads, api.BookReadRun{ID: api.NewID(), StartedAt: now, Sessions: []api.ReadingSession{}})
				readIndex = len(record.Reads) - 1
			}
			record.Reads[readIndex].Sessions = append(record.Reads[readIndex].Sessions, api.ReadingSession{ID: update.SessionID, StartedAt: now, UpdatedAt: now})
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
			run.CompletedAt = cloneAccountTimePtr(&now)
		}
		record.UpdatedAt = now
		putReadingRecord(&activity.Reading, record)
		result = cloneAccountReadingRecord(record)
		return nil
	})
	return result, err
}
