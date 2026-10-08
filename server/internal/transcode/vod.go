package transcode

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const defaultVODLimit int64 = 20 << 30

type VODStatus struct {
	Status   string  `json:"status"`
	Phase    string  `json:"phase,omitempty"`
	Progress float64 `json:"progress"`
	Error    string  `json:"error,omitempty"`
	CacheKey string  `json:"cacheKey"`
	ItemID   string  `json:"itemID"`
	Bytes    int64   `json:"bytes,omitempty"`
}
type VODAsset struct {
	mu     sync.Mutex
	state  VODStatus
	dir    string
	cancel context.CancelFunc
}

func (a *VODAsset) Snapshot() VODStatus { a.mu.Lock(); defer a.mu.Unlock(); return a.state }
func VODKey(id, version string) string {
	hash := sha256.Sum256([]byte(id + "|" + version + "|h264-main-720-aac-stereo-v1"))
	return hex.EncodeToString(hash[:])
}
func VODLimit() int64 {
	if n, e := strconv.ParseInt(os.Getenv("SONDER_VOD_CACHE_MAX_BYTES"), 10, 64); e == nil && n >= 1<<30 {
		return n
	}
	return defaultVODLimit
}
func directoryBytes(root string) int64 {
	var n int64
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if st, e := d.Info(); e == nil {
				n += st.Size()
			}
		}
		return nil
	})
	return n
}

func loadVOD(root, id, key string) *VODAsset {
	if len(key) != 64 {
		return nil
	}
	if _, err := hex.DecodeString(key); err != nil {
		return nil
	}
	dir := filepath.Join(root, key)
	data, err := os.ReadFile(filepath.Join(dir, "ready.json"))
	if err != nil {
		return nil
	}
	var state VODStatus
	if json.Unmarshal(data, &state) != nil || state.ItemID != id || state.CacheKey != key || state.Status != "ready" {
		return nil
	}
	if st, err := os.Stat(filepath.Join(dir, "download.mp4")); err != nil || st.Size() == 0 {
		return nil
	}
	return &VODAsset{state: state, dir: dir, cancel: func() {}}
}
func (m *Manager) VOD(root, id, key string) *VODAsset {
	registry := root + "|" + key
	if value, ok := m.vod.Load(registry); ok {
		asset := value.(*VODAsset)
		if asset.Snapshot().ItemID != id {
			return nil
		}
		return asset
	}
	if a := loadVOD(root, id, key); a != nil {
		actual, _ := m.vod.LoadOrStore(registry, a)
		return actual.(*VODAsset)
	}
	return nil
}
func (m *Manager) PrepareVOD(root, id, version string, req Request, duration float64, release func()) (*VODAsset, error) {
	m.vodMu.Lock()
	defer m.vodMu.Unlock()
	key := VODKey(id, version)
	if a := m.VOD(root, id, key); a != nil && a.Snapshot().Status != "failed" {
		release()
		return a, nil
	}
	queued := 0
	m.vod.Range(func(_, v any) bool {
		if v.(*VODAsset).Snapshot().Status == "preparing" {
			queued++
		}
		return true
	})
	if queued >= 8 {
		release()
		return nil, errors.New("Movie preparation queue is full")
	}
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		release()
		return nil, errors.New("Movie duration must be indexed before preparation")
	}
	// Reserve space for the segmented encode and its downloadable MP4 copy.
	reserve := int64(math.Ceil(duration*4_500_000/8))*2 + (32 << 20)
	if reserve > VODLimit() {
		release()
		return nil, errors.New("Movie exceeds the configured prepared-media cache budget")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		release()
		return nil, err
	}
	dir := filepath.Join(root, key)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	a := &VODAsset{dir: dir, cancel: cancel, state: VODStatus{Status: "preparing", Phase: "Queued", ItemID: id, CacheKey: key}}
	m.vod.Store(root+"|"+key, a)
	go m.encodeVOD(ctx, root, a, req, duration, reserve, release)
	return a, nil
}

func (m *Manager) makeVODSpace(root string, reserve int64, keep string) error {
	type candidate struct {
		path, key string
		used      time.Time
	}
	entries, _ := os.ReadDir(root)
	var candidates []candidate
	for _, e := range entries {
		if !e.IsDir() || e.Name() == keep || len(e.Name()) != 64 {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if value, ok := m.vod.Load(root + "|" + e.Name()); ok && value.(*VODAsset).Snapshot().Status == "preparing" {
			continue
		}
		st, err := os.Stat(filepath.Join(dir, "access"))
		if err != nil {
			st, err = e.Info()
		}
		if err != nil {
			continue
		}
		if time.Since(st.ModTime()) < 30*time.Minute {
			continue
		}
		candidates = append(candidates, candidate{dir, e.Name(), st.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].used.Before(candidates[j].used) })
	for _, c := range candidates {
		if directoryBytes(root)+reserve <= VODLimit() && time.Since(c.used) < 7*24*time.Hour {
			break
		}
		os.RemoveAll(c.path)
		m.vod.Delete(root + "|" + c.key)
	}
	if directoryBytes(root)+reserve > VODLimit() {
		return errors.New("Prepared-media cache is full; recently used movies are protected")
	}
	var disk syscall.Statfs_t
	if syscall.Statfs(root, &disk) == nil && int64(disk.Bavail)*int64(disk.Bsize) < reserve+(2<<30) {
		return errors.New("Insufficient free disk space to prepare this movie")
	}
	return nil
}
func (a *VODAsset) phase(text string) { a.mu.Lock(); a.state.Phase = text; a.mu.Unlock() }
func (m *Manager) encodeVOD(ctx context.Context, root string, a *VODAsset, req Request, duration float64, reserve int64, release func()) {
	defer release()
	defer a.cancel()
	finish := func(err error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.state.Status = "failed"
			a.state.Error = err.Error()
			a.state.Phase = "Preparation failed"
			os.RemoveAll(a.dir)
		}
	}
	select {
	case m.vodSerial <- struct{}{}:
	case <-ctx.Done():
		finish(ctx.Err())
		return
	}
	defer func() { <-m.vodSerial }()
	m.vodMu.Lock()
	err := m.makeVODSpace(root, reserve, a.Snapshot().CacheKey)
	m.vodMu.Unlock()
	if err != nil {
		finish(err)
		return
	}
	// Remove only this disposable incomplete cache, never source media.
	os.RemoveAll(a.dir)
	if err = os.Mkdir(a.dir, 0700); err != nil {
		finish(err)
		return
	}
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		finish(ctx.Err())
		return
	}
	defer func() { <-m.sem }()
	req.Mode = ModeEncode
	req.CopyAudio = false
	req.StartSeconds = 0
	cfg := m.cfg
	cfg.HWAccel = "none"
	args := buildArgs(req, cfg)
	args = args[:len(args)-5]
	if i := indexOfArg(args, "-force_key_frames"); i >= 0 {
		args[i+1] = "expr:gte(t,n_forced*6)"
	}
	args = append(args, "-vf", "scale=w=min(1280\\,iw):h=-2", "-profile:v", "main", "-level:v", "4.1", "-maxrate", "4M", "-bufsize", "8M", "-ac", "2", "-progress", "pipe:1", "-nostats", "-f", "hls", "-hls_time", "6", "-hls_list_size", "0", "-hls_playlist_type", "vod", "-hls_flags", "independent_segments+temp_file", "-hls_segment_filename", filepath.Join(a.dir, "segment%06d.ts"), filepath.Join(a.dir, "media.m3u8"))
	monitorDone := make(chan struct{})
	defer close(monitorDone)
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-monitorDone:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				var disk syscall.Statfs_t
				lowDisk := syscall.Statfs(root, &disk) == nil && int64(disk.Bavail)*int64(disk.Bsize) < 2<<30
				if lowDisk || directoryBytes(root) > VODLimit() || directoryBytes(a.dir) > reserve {
					a.cancel()
					return
				}
			}
		}
	}()
	a.phase("Preparing Apple-compatible video")
	if err = m.runVOD(ctx, a, args, duration); err != nil {
		finish(errors.New("Video preparation interrupted or failed; retry preparation"))
		return
	}
	if directoryBytes(a.dir) > reserve/2 {
		finish(errors.New("Encoded movie exceeded its reserved cache budget"))
		return
	}
	a.phase("Preparing downloadable MP4")
	args = []string{"-hide_banner", "-loglevel", "error", "-i", filepath.Join(a.dir, "media.m3u8"), "-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-movflags", "+faststart", filepath.Join(a.dir, "download.tmp.mp4")}
	if err = m.runVOD(ctx, a, args, 0); err != nil {
		finish(errors.New("Download preparation failed; retry preparation"))
		return
	}
	if err = os.Rename(filepath.Join(a.dir, "download.tmp.mp4"), filepath.Join(a.dir, "download.mp4")); err != nil {
		finish(err)
		return
	}
	if directoryBytes(root) > VODLimit() {
		finish(errors.New("Prepared-media cache budget exceeded"))
		return
	}
	media, err := os.ReadFile(filepath.Join(a.dir, "media.m3u8"))
	if err != nil || !strings.Contains(string(media), "#EXT-X-ENDLIST") {
		finish(errors.New("Movie playlist is incomplete"))
		return
	}
	var totalBytes int64
	var totalDuration, segmentDuration, peak float64
	for _, line := range strings.Split(string(media), "\n") {
		if strings.HasPrefix(line, "#EXTINF:") {
			segmentDuration, _ = strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(line, "#EXTINF:"), ","), 64)
		}
		if strings.HasPrefix(line, "segment") && segmentDuration > 0 {
			if st, e := os.Stat(filepath.Join(a.dir, line)); e == nil {
				totalBytes += st.Size()
				totalDuration += segmentDuration
				peak = math.Max(peak, float64(st.Size()*8)/segmentDuration)
			}
		}
	}
	if totalDuration <= 0 {
		finish(errors.New("Movie segments could not be indexed"))
		return
	}
	master := fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,CODECS=\"avc1.4d4029,mp4a.40.2\"\nmedia.m3u8\n", int64(math.Ceil(peak)), int64(float64(totalBytes*8)/totalDuration))
	if err = os.WriteFile(filepath.Join(a.dir, "index.m3u8"), []byte(master), 0600); err != nil {
		finish(err)
		return
	}
	a.mu.Lock()
	a.state.Status = "ready"
	a.state.Phase = "Ready"
	a.state.Progress = 100
	a.state.Bytes = directoryBytes(a.dir)
	snapshot := a.state
	a.mu.Unlock()
	data, _ := json.Marshal(snapshot)
	if err = os.WriteFile(filepath.Join(a.dir, "ready.tmp.json"), data, 0600); err == nil {
		err = os.Rename(filepath.Join(a.dir, "ready.tmp.json"), filepath.Join(a.dir, "ready.json"))
	}
	if err != nil {
		finish(err)
		return
	}
	os.WriteFile(filepath.Join(a.dir, "access"), nil, 0600)
}
func (m *Manager) runVOD(ctx context.Context, a *VODAsset, args []string, duration float64) error {
	executable := m.cfg.FFmpegPath
	if nice, err := exec.LookPath("nice"); err == nil {
		args = append([]string{"-n", "19", executable}, args...)
		executable = nice
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = KillGrace
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	scan := bufio.NewScanner(stdout)
	for scan.Scan() {
		line := scan.Text()
		if duration > 0 && strings.HasPrefix(line, "out_time_us=") {
			n, _ := strconv.ParseFloat(strings.TrimPrefix(line, "out_time_us="), 64)
			a.mu.Lock()
			a.state.Progress = math.Min(98, n/1e6/duration*98)
			a.mu.Unlock()
		}
	}
	return cmd.Wait()
}
func (a *VODAsset) Open(name string) (*os.File, error) {
	valid := name == "index.m3u8" || name == "media.m3u8" || name == "download.mp4" || (strings.HasPrefix(name, "segment") && strings.HasSuffix(name, ".ts") && filepath.Base(name) == name && !strings.Contains(name, ".."))
	if !valid || a.Snapshot().Status != "ready" {
		return nil, os.ErrNotExist
	}
	f, err := os.Open(filepath.Join(a.dir, name))
	if err == nil {
		now := time.Now()
		os.Chtimes(filepath.Join(a.dir, "access"), now, now)
	}
	return f, err
}
