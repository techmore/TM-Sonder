package youtube

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var channelURL = regexp.MustCompile(`^https://(?:www\.)?youtube\.com/(?:@[A-Za-z0-9_.-]+|channel/UC[A-Za-z0-9_-]{22}|c/[A-Za-z0-9_.-]+|user/[A-Za-z0-9_.-]+)(?:/(?:videos|shorts|streams))?/?$`)

func New(dataDir, root, yt, ffmpeg, ffprobe string, onComplete func()) (*Manager, error) {
	m := &Manager{path: filepath.Join(dataDir, "youtube-subscriptions.json"), root: root, yt: yt, ffmpeg: ffmpeg, ffprobe: ffprobe, known: map[string]bool{}, wake: make(chan struct{}, 1), done: make(chan struct{}), onComplete: onComplete}
	m.state = State{Settings: Settings{MinFreeGB: 500, MaxHeight: 1080, Profile: "efficient"}, Channels: []Channel{}, Jobs: []Job{}}
	b, err := os.ReadFile(m.path)
	if err == nil {
		if err = json.Unmarshal(b, &m.state); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err = validateSettings(m.state.Settings); err != nil {
		return nil, err
	}
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.Status == "downloading" || j.Status == "converting" {
			j.Status = "queued"
			j.Detail = "Resuming after server restart"
		}
	}
	if root != "" {
		if b, err := os.ReadFile(filepath.Join(root, "downloaded.txt")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				p := strings.Fields(line)
				if len(p) == 2 && strings.EqualFold(p[0], "youtube") && videoID.MatchString(p[1]) {
					m.known[p[1]] = true
				}
			}
		}
	}
	if err = m.saveLocked(); err != nil {
		return nil, err
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	go m.worker()
	return m, nil
}
func validateSettings(s Settings) error {
	if s.MinFreeGB < 1 || s.MinFreeGB > 100000 {
		return errors.New("Free-space reserve must be between 1 and 100000 GB")
	}
	if s.MaxHeight != 480 && s.MaxHeight != 720 && s.MaxHeight != 1080 && s.MaxHeight != 1440 && s.MaxHeight != 2160 {
		return errors.New("Choose a supported resolution")
	}
	if s.Profile != "efficient" && s.Profile != "compatible" && s.Profile != "compact" {
		return errors.New("Choose efficient, compatible or compact")
	}
	return nil
}
func (m *Manager) saveLocked() error {
	b, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.path), ".youtube-queue-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, m.path)
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	if m.activeCancel != nil {
		m.activeCancel()
	}
	m.mu.Unlock()
	<-m.done
}
func free(path string) (int64, error) {
	var st syscall.Statfs_t
	err := syscall.Statfs(path, &st)
	return int64(st.Bavail) * int64(st.Bsize), err
}
func available(cmd string) bool {
	if cmd == "" {
		return false
	}
	_, e := exec.LookPath(cmd)
	return e == nil
}
func (m *Manager) Status() Snapshot {
	m.mu.Lock()

	b, _ := json.Marshal(m.state)
	var st State
	_ = json.Unmarshal(b, &st)
	active, blocked, known := m.active, m.blocked, len(m.known)
	m.mu.Unlock()
	d := map[string]bool{"yt-dlp": available(m.yt), "ffmpeg": available(m.ffmpeg), "ffprobe": available(m.ffprobe), "deno": available("deno")}
	s := Snapshot{State: st, StoragePath: m.root, Dependencies: d, Active: active, Blocked: blocked, KnownArchiveIDs: known}
	s.Ready = d["yt-dlp"] && d["ffmpeg"] && d["ffprobe"] && d["deno"] && m.root != ""
	if m.root == "" {
		s.Blocked = "Set SONDER_YOUTUBE_DIR to the mounted NAS download folder"
	} else if n, e := free(m.root); e == nil {
		s.FreeBytes = n
		if n < s.Settings.MinFreeGB*GB+jobBudget {
			s.Blocked = "Waiting for NAS free space above the reserve plus a 24 GB work allowance"
		}
	} else {
		s.Ready = false
		s.Blocked = "NAS download folder is unavailable"
	}
	if !d["yt-dlp"] || !d["ffmpeg"] || !d["ffprobe"] || !d["deno"] {
		s.Blocked = "Install yt-dlp, FFmpeg, FFprobe and Deno on the server"
	}
	s.Counts = map[string]int{}
	for _, j := range s.Jobs {
		s.Counts[j.Status]++
		s.SavedBytes += j.SavedBytes
		if j.Status == "completed" {
			s.StoredBytes += j.Bytes
		}
	}
	if len(s.Jobs) > 300 {
		recent := append([]Job(nil), s.Jobs[len(s.Jobs)-300:]...)
		selected := map[string]bool{}
		for _, j := range recent {
			selected[j.ID] = true
		}
		for _, j := range s.Jobs {
			if len(recent) >= 600 {
				break
			}
			if !selected[j.ID] && (j.Status == "queued" || j.Status == "downloading" || j.Status == "converting" || j.Status == "failed") {
				recent = append(recent, j)
			}
		}
		s.Jobs = recent
	}
	return s
}
func (m *Manager) Configure(s Settings) error {
	if err := validateSettings(s); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.state.Settings
	m.state.Settings = s
	m.blocked = ""
	if err := m.saveLocked(); err != nil {
		m.state.Settings = old
		return err
	}
	if m.activeCancel != nil {
		m.activeCancel()
	}
	m.notify()
	return nil
}
func (m *Manager) Add(url string, hours, backfill int) (Channel, error) {
	url = strings.TrimSpace(url)
	if !channelURL.MatchString(url) {
		return Channel{}, errors.New("Use a YouTube channel URL, such as https://www.youtube.com/@channel")
	}
	if hours != 1 && hours != 24 {
		return Channel{}, errors.New("Check interval must be hourly or daily")
	}
	if backfill < 0 || backfill > 100 {
		return Channel{}, errors.New("Initial downloads must be between 0 and 100")
	}
	url = strings.TrimSuffix(url, "/")
	for _, tab := range []string{"/videos", "/shorts", "/streams"} {
		url = strings.TrimSuffix(url, tab)
	}
	url = strings.Replace(url, "https://youtube.com/", "https://www.youtube.com/", 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.state.Channels) >= 200 {
		return Channel{}, errors.New("Maximum 200 subscriptions")
	}
	for _, c := range m.state.Channels {
		if c.URL == url {
			return Channel{}, errors.New("Channel is already subscribed")
		}
	}
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Channel{}, err
	}
	c := Channel{ID: hex.EncodeToString(id[:]), URL: url, Name: strings.TrimPrefix(url, "https://www.youtube.com/"), IntervalHours: hours, Backfill: backfill}
	m.state.Channels = append(m.state.Channels, c)
	if err := m.saveLocked(); err != nil {
		m.state.Channels = m.state.Channels[:len(m.state.Channels)-1]
		return Channel{}, err
	}
	m.notify()
	return c, nil
}
func (m *Manager) ChannelAction(id, action string, hours int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Channels {
		c := &m.state.Channels[i]
		if c.ID != id {
			continue
		}
		old := *c
		switch action {
		case "pause":
			c.Paused = true
		case "resume":
			c.Paused = false
		case "check":
			c.NextCheck = time.Time{}
		case "interval":
			if hours != 1 && hours != 24 {
				return errors.New("Interval must be 1 or 24 hours")
			}
			c.IntervalHours = hours
			c.NextCheck = time.Time{}
		default:
			return errors.New("Unknown channel action")
		}
		if err := m.saveLocked(); err != nil {
			*c = old
			return err
		}
		if c.Paused && m.activeChannel == id && m.activeCancel != nil {
			m.activeCancel()
		}
		m.notify()
		return nil
	}
	return errors.New("Channel not found")
}
func (m *Manager) JobAction(id, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID != id {
			continue
		}
		old := *j
		switch action {
		case "retry":
			if j.Status == "completed" || j.Status == "skipped" || j.Status == "downloading" || j.Status == "converting" {
				return errors.New("Video cannot be retried in its current state")
			}
			j.Status = "queued"
			j.Detail = "Retry requested"
		case "cancel":
			if j.Status == "completed" || j.Status == "skipped" {
				return errors.New("Completed media is preserved")
			}
			j.Status = "cancelled"
			j.Detail = "Cancelled by owner"
		default:
			return errors.New("Unknown job action")
		}
		j.UpdatedAt = time.Now().UTC()
		if err := m.saveLocked(); err != nil {
			*j = old
			return err
		}
		if m.active == id && m.activeCancel != nil {
			m.activeCancel()
		}
		m.notify()
		return nil
	}
	return errors.New("Video not found")
}
func (m *Manager) CheckAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Channels {
		m.state.Channels[i].NextCheck = time.Time{}
	}
	err := m.saveLocked()
	m.notify()
	return err
}
