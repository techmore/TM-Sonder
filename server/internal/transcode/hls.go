package transcode

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// HLSSession serves atomic TS segments to native HLS players. Shared encoder
// limits, input pacing, rolling segments and idle eviction bound resources.
type HLSSession struct {
	ID, ItemID, dir string
	owner           string
	cancel          context.CancelFunc
	done            chan struct{}
	mu              sync.Mutex
	touched         time.Time
}

func (m *Manager) StartHLS(ctx context.Context, root, itemID, owner string, req Request, release func()) (*HLSSession, error) {
	if len(owner) >= 20 && len(owner) <= 100 {
		var previous []*HLSSession
		m.hls.Range(func(_, value any) bool {
			old := value.(*HLSSession)
			if old.owner == owner {
				old.cancel()
				previous = append(previous, old)
			}
			return true
		})
		for _, old := range previous {
			select {
			case <-old.done:
			case <-ctx.Done():
				release()
				return nil, ctx.Err()
			}
		}
	} else {
		owner = ""
	}
	select {
	case m.sem <- struct{}{}:
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
	fail := func(err error) (*HLSSession, error) { <-m.sem; release(); return nil, err }
	if err := os.MkdirAll(root, 0700); err != nil {
		return fail(err)
	}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) != 32 {
			continue
		}
		if _, active := m.hls.Load(entry.Name()); active {
			continue
		}
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > 5*time.Minute {
			os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return fail(err)
	}
	id := hex.EncodeToString(nonce)
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return fail(err)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s := &HLSSession{ID: id, ItemID: itemID, dir: dir, owner: owner, cancel: cancel, done: make(chan struct{}), touched: time.Now()}
	req.Mode = ModeEncode
	req.CopyAudio = false
	cfg := m.cfg
	cfg.HWAccel = "none"
	args := buildArgs(req, cfg)
	args = args[:len(args)-5]
	if i := indexOfArg(args, "-vf"); i >= 0 {
		args[i+1] += ",scale=w=min(1280\\,iw):h=-2"
	} else {
		args = append(args, "-vf", "scale=w=min(1280\\,iw):h=-2")
	}
	args = append(args[:4], append([]string{"-readrate", "1"}, args[4:]...)...)
	args = append(args, "-maxrate", "4M", "-bufsize", "8M", "-ac", "2", "-f", "hls", "-hls_time", "2", "-hls_list_size", "90", "-hls_delete_threshold", "15", "-hls_flags", "delete_segments+independent_segments+temp_file", "-hls_segment_filename", filepath.Join(dir, "segment%06d.ts"), filepath.Join(dir, "index.m3u8"))
	cmd := exec.CommandContext(runCtx, m.cfg.FFmpegPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = KillGrace
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(dir)
		return fail(err)
	}
	m.hls.Store(id, s)
	go func() { cmd.Wait(); close(s.done); <-m.sem; release() }()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			idle := time.Since(s.touched) > 90*time.Second
			s.mu.Unlock()
			if idle {
				cancel()
				<-s.done
				m.hls.Delete(id)
				os.RemoveAll(dir)
				return
			}
		}
	}()
	return s, nil
}

func (m *Manager) HLS(id, itemID string) (*HLSSession, bool) {
	value, ok := m.hls.Load(id)
	if !ok {
		return nil, false
	}
	s := value.(*HLSSession)
	return s, s.ItemID == itemID
}

func (s *HLSSession) ReadAsset(ctx context.Context, name string) ([]byte, error) {
	if name != "index.m3u8" && !(strings.HasPrefix(name, "segment") && strings.HasSuffix(name, ".ts") && filepath.Base(name) == name && !strings.Contains(name, "..")) {
		return nil, os.ErrNotExist
	}
	s.mu.Lock()
	s.touched = time.Now()
	s.mu.Unlock()
	timeout := time.NewTimer(20 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(s.dir, name))
		if err == nil && (name != "index.m3u8" || strings.Count(string(data), "#EXTINF:") >= 3 || strings.Contains(string(data), "#EXT-X-ENDLIST")) {
			return data, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, errors.New("HLS segment timed out")
		case <-s.done:
			if err == nil {
				return data, nil
			}
			return nil, errors.New("HLS encoder stopped")
		case <-ticker.C:
		}
	}
}
