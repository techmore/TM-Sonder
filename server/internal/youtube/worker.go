package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (m *Manager) worker() {
	defer close(m.done)
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		if m.step() {
			continue
		}
		select {
		case <-m.ctx.Done():
			return
		case <-m.wake:
		case <-tick.C:
		}
	}
}
func (m *Manager) step() bool {
	s := m.Status()
	if !s.Ready || s.Settings.Paused {
		return false
	}
	m.mu.Lock()
	var job *Job
	var channel Channel
	for _, j := range m.state.Jobs {
		if j.Status != "queued" {
			continue
		}
		for _, c := range m.state.Channels {
			if c.ID == j.ChannelID && !c.Paused {
				copy := j
				job = &copy
				channel = c
				break
			}
		}
		if job != nil {
			break
		}
	}
	m.mu.Unlock()
	if job == nil {
		return false
	}
	if s.FreeBytes < s.Settings.MinFreeGB*GB+jobBudget {
		return false
	}
	m.download(*job, channel, s.Settings)
	return true
}

// Killing the whole process group also stops yt-dlp's FFmpeg child processes.
func (m *Manager) run(ctx context.Context, cmd string, args []string, limit int) ([]byte, error) {
	process := exec.CommandContext(ctx, cmd, args...)
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error { return syscall.Kill(-process.Process.Pid, syscall.SIGKILL) }
	process.WaitDelay = 5 * time.Second
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: 4096, tail: true}
	process.Stdout = stdout
	process.Stderr = stderr
	err := process.Run()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(cmd), err, strings.TrimSpace(string(stderr.data)))
	}
	return stdout.data, nil
}

type boundedBuffer struct {
	data  []byte
	limit int
	tail  bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.tail {
		b.data = append(b.data, p...)
		if len(b.data) > b.limit {
			b.data = b.data[len(b.data)-b.limit:]
		}
		return n, nil
	}
	if len(b.data)+n > b.limit {
		return 0, errors.New("Command output exceeded its limit")
	}
	b.data = append(b.data, p...)
	return n, nil
}
func (m *Manager) begin(label, channel string, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	m.mu.Lock()
	if m.state.Settings.Paused {
		cancel()
	}
	for _, c := range m.state.Channels {
		if c.ID == channel && c.Paused {
			cancel()
		}
	}
	if videoID.MatchString(label) {
		for _, j := range m.state.Jobs {
			if j.ID == label && j.Status == "cancelled" {
				cancel()
			}
		}
	}
	m.active = label
	m.activeChannel = channel
	m.activeCancel = cancel
	m.mu.Unlock()
	return ctx, cancel
}
func (m *Manager) end() {
	m.mu.Lock()
	m.active = ""
	m.activeChannel = ""
	m.activeCancel = nil
	m.mu.Unlock()
}
func (m *Manager) check(c Channel) {
	ctx, cancel := context.WithTimeout(m.ctx, 3*time.Minute)
	m.mu.Lock()
	if m.state.Settings.Paused {
		cancel()
	}
	m.checking = c.ID
	m.checkCancel = cancel
	m.mu.Unlock()
	defer func() { cancel(); m.mu.Lock(); m.checking = ""; m.checkCancel = nil; m.mu.Unlock() }()
	raw, err := m.run(ctx, m.yt, []string{"--ignore-config", "--no-plugin-dirs", "--flat-playlist", "--dump-single-json", "--playlist-end", "200", "--socket-timeout", "20", "--retries", "2", "--", c.URL + "/videos"}, 12*1024*1024)
	var info struct {
		Channel   string `json:"channel"`
		ChannelID string `json:"channel_id"`
		Entries   []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			LiveStatus string `json:"live_status"`
		} `json:"entries"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &info)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Channels {
		current := &m.state.Channels[i]
		if current.ID != c.ID {
			continue
		}
		current.NextCheck = time.Now().UTC().Add(time.Duration(current.IntervalHours) * time.Hour)
		if err != nil {
			current.Error = err.Error()
			_ = m.saveLocked()
			return
		}
		if ctx.Err() != nil || current.Paused || m.state.Settings.Paused {
			return
		}
		if info.ChannelID != "" {
			for _, other := range m.state.Channels {
				if other.ID != c.ID && other.YouTubeID == info.ChannelID {
					current.Paused = true
					current.Error = "This resolved channel is already subscribed"
					_ = m.saveLocked()
					return
				}
			}
			current.YouTubeID = info.ChannelID
		}
		if info.Channel != "" {
			current.Name = info.Channel
		}
		current.LastCheck = time.Now().UTC()
		current.Error = ""
		known := make(map[string]bool, len(m.state.Jobs))
		for _, j := range m.state.Jobs {
			known[j.ID] = true
		}
		for n, entry := range info.Entries {
			if !videoID.MatchString(entry.ID) || known[entry.ID] || entry.LiveStatus == "is_live" || entry.LiveStatus == "is_upcoming" {
				continue
			}
			status := "queued"
			detail := "Waiting for download"
			if m.known[entry.ID] {
				status = "skipped"
				detail = "Already in existing NAS download archive"
			} else if !current.Initialized && n >= current.Backfill {
				status = "skipped"
				detail = "Before subscription baseline"
			}
			m.state.Jobs = append(m.state.Jobs, Job{ID: entry.ID, ChannelID: c.ID, Title: entry.Title, Status: status, Detail: detail, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
			known[entry.ID] = true
		}
		current.Initialized = true
		if err = m.saveLocked(); err != nil {
			m.state.Settings.Paused = true
			m.blocked = "Cannot save subscription state: " + err.Error()
		}
		return
	}
}
func (m *Manager) update(id, status, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		j := &m.state.Jobs[i]
		if j.ID == id {
			if j.Status == "cancelled" {
				return
			}
			j.Status = status
			j.Detail = detail
			j.UpdatedAt = time.Now().UTC()
			if err := m.saveLocked(); err != nil {
				m.state.Settings.Paused = true
				m.blocked = "Cannot save queue state: " + err.Error()
				if m.activeCancel != nil {
					m.activeCancel()
				}
			}
			return
		}
	}
}
func (m *Manager) guard(ctx context.Context, cancel context.CancelFunc, work, id string, reserve int64) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			n, e := free(m.root)
			var used int64
			walkErr := filepath.WalkDir(work, func(path string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type().IsRegular() {
					st, e := d.Info()
					if e != nil {
						return e
					}
					used += st.Size()
				}
				return nil
			})
			if e != nil || n < reserve+GB || used > jobBudget || walkErr != nil {
				m.update(id, "failed", "Download stopped: NAS reserve, work-size limit or storage availability; partial files retained")
				cancel()
				return
			}
		}
	}
}
func (m *Manager) download(j Job, c Channel, s Settings) {
	ctx, cancel := m.begin(j.ID, c.ID, 6*time.Hour)
	defer cancel()
	defer m.end()
	if ctx.Err() != nil {
		return
	}
	work := filepath.Join(m.root, ".sonder-staging", j.ID)
	destination := filepath.Join(m.root, "Sonder Channels", c.ID, j.ID)
	if _, err := os.Stat(destination); err == nil {
		m.complete(j, destination)
		return
	}
	if err := os.MkdirAll(work, 0o700); err != nil {
		m.update(j.ID, "failed", "Cannot create NAS workspace: "+err.Error())
		return
	}
	go m.guard(ctx, cancel, work, j.ID, s.MinFreeGB*GB)
	m.update(j.ID, "downloading", "Downloading to NAS; partial files resume after pause")
	format := fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]", s.MaxHeight, s.MaxHeight)
	if s.Profile == "compatible" {
		format = fmt.Sprintf("bv*[height<=%d][vcodec^=avc1]+ba[ext=m4a]/b[height<=%d][ext=mp4]", s.MaxHeight, s.MaxHeight)
	}
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--no-progress", "--continue", "--socket-timeout", "20", "--retries", "3", "--fragment-retries", "3", "--abort-on-unavailable-fragments", "--max-filesize", "8G", "--limit-rate", "20M", "--match-filters", "!is_live & !is_upcoming", "--format", format, "--format-sort", "vcodec:av01", "--merge-output-format", "mkv", "--write-info-json", "--write-thumbnail", "--convert-thumbnails", "jpg", "--write-subs", "--write-auto-subs", "--sub-langs", "en.*,es.*", "--sub-format", "vtt", "--no-simulate", "--ffmpeg-location", m.ffmpeg, "--output", filepath.Join(work, "%(title).160B [%(id)s].%(ext)s"), "--", "https://www.youtube.com/watch?v=" + j.ID}
	_, err := m.run(ctx, m.yt, args, 1024*1024)
	if err == nil {
		var file string
		file, err = mediaFile(work)
		if err == nil {
			err = m.validate(ctx, file)
			if err == nil && s.Profile == "compact" {
				m.update(j.ID, "converting", "Encoding AV1 with two threads; original kept unless a validated result is smaller")
				err = m.compact(ctx, file, j.ID)
			}
		}
	}
	if ctx.Err() != nil {
		m.mu.Lock()
		status := ""
		for _, current := range m.state.Jobs {
			if current.ID == j.ID {
				status = current.Status
			}
		}
		m.mu.Unlock()
		if status != "failed" && status != "cancelled" {
			m.update(j.ID, "queued", "Paused or interrupted; partial download retained")
		}
		return
	}
	if err != nil {
		m.update(j.ID, "failed", err.Error())
		return
	}
	// Stop the guard before moving its workspace; validate free space once more.
	cancel()
	if n, e := free(m.root); e != nil || n < s.MinFreeGB*GB {
		m.update(j.ID, "failed", "NAS reserve reached before publishing; complete file retained in staging")
		return
	}
	if err = os.Chmod(work, 0o755); err != nil {
		m.update(j.ID, "failed", err.Error())
		return
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0o755); err == nil {
		err = os.Rename(work, destination)
	}
	if err != nil {
		m.update(j.ID, "failed", "Could not publish completed media: "+err.Error())
		return
	}
	m.complete(j, destination)
}
func mediaFile(dir string) (string, error) {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return "", e
	}
	for _, f := range entries {
		switch strings.ToLower(filepath.Ext(f.Name())) {
		case ".mp4", ".mkv", ".webm":
			if !strings.HasPrefix(f.Name(), ".compact-") {
				return filepath.Join(dir, f.Name()), nil
			}
		}
	}
	return "", errors.New("No completed media file returned by yt-dlp")
}
func (m *Manager) duration(ctx context.Context, file string) (float64, error) {
	b, e := m.run(ctx, m.ffprobe, []string{"-v", "error", "-show_entries", "format=duration:stream=codec_type", "-of", "json", file}, 65536)
	if e != nil {
		return 0, e
	}
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return 0, e
	}
	var d float64
	_, e = fmt.Sscan(p.Format.Duration, &d)
	video, audio := false, false
	for _, s := range p.Streams {
		video = video || s.CodecType == "video"
		audio = audio || s.CodecType == "audio"
	}
	if e != nil || d <= 0 || !video || !audio {
		return 0, errors.New("Media validation requires video, audio and a positive duration")
	}
	return d, nil
}
func (m *Manager) validate(ctx context.Context, file string) error {
	_, e := m.duration(ctx, file)
	return e
}
func (m *Manager) compact(ctx context.Context, input, id string) error {
	original, e := os.Stat(input)
	if e != nil {
		return e
	}
	duration, e := m.duration(ctx, input)
	if e != nil {
		return e
	}
	output := filepath.Join(filepath.Dir(input), ".compact-"+id+".mp4")
	_ = os.Remove(output)
	_, e = m.run(ctx, m.ffmpeg, []string{"-nostdin", "-v", "error", "-y", "-i", input, "-map", "0:v:0", "-map", "0:a:0", "-c:v", "libsvtav1", "-preset", "8", "-crf", "35", "-threads", "2", "-svtav1-params", "lp=2", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", output}, 4096)
	if e != nil {
		_ = os.Remove(output)
		return e
	}
	got, e := m.duration(ctx, output)
	if e != nil || got < duration*0.98 || got > duration*1.02 {
		_ = os.Remove(output)
		return errors.New("Compact copy failed duration/audio/video validation; original preserved")
	}
	st, e := os.Stat(output)
	if e != nil {
		return e
	}
	if st.Size() >= original.Size() {
		return os.Remove(output)
	}
	target := strings.TrimSuffix(input, filepath.Ext(input)) + ".mp4"
	if e = os.Rename(output, target); e != nil {
		return e
	}
	if input != target {
		if e = os.Remove(input); e != nil {
			return e
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Jobs {
		if m.state.Jobs[i].ID == id {
			m.state.Jobs[i].OriginalBytes = original.Size()
			m.state.Jobs[i].SavedBytes = original.Size() - st.Size()
		}
	}
	return m.saveLocked()
}
func (m *Manager) complete(j Job, destination string) {
	file, err := mediaFile(destination)
	if err != nil {
		m.update(j.ID, "failed", err.Error())
		return
	}
	st, err := os.Stat(file)
	if err != nil {
		m.update(j.ID, "failed", err.Error())
		return
	}
	m.mu.Lock()
	for i := range m.state.Jobs {
		if m.state.Jobs[i].ID == j.ID {
			p := &m.state.Jobs[i]
			p.Status = "completed"
			p.Detail = "Stored on NAS"
			p.Path = file
			p.Bytes = st.Size()
			if p.OriginalBytes == 0 {
				p.OriginalBytes = p.Bytes
			}
			p.UpdatedAt = time.Now().UTC()
		}
	}
	err = m.saveLocked()
	if err != nil {
		m.state.Settings.Paused = true
		m.blocked = "Completed file preserved, but queue persistence failed"
	}
	m.mu.Unlock()
	if err == nil && m.onComplete != nil {
		m.onComplete()
	}
}

// Polling has its own bounded worker so a long encode cannot delay hourly checks.
func (m *Manager) checkWorker() {
	defer close(m.checkDone)
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
		ready := m.Status().Ready
		m.mu.Lock()
		var due *Channel
		if ready && !m.state.Settings.Paused {
			for _, c := range m.state.Channels {
				if !c.Paused && !c.NextCheck.After(time.Now()) {
					copy := c
					due = &copy
					break
				}
			}
		}
		m.mu.Unlock()
		if due != nil {
			m.check(*due)
			m.notify()
			continue
		}
		select {
		case <-m.ctx.Done():
			return
		case <-m.checkWake:
		case <-tick.C:
		}
	}
}
