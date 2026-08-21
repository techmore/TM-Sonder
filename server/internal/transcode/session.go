// Package transcode runs ffmpeg as a sidecar producing fragmented MP4 on
// stdout, with a session registry that reuses warm sessions for nearby seeks
// and kills processes when clients disconnect.
package transcode

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"tm-sonder/server/internal/library"
)

const (
	// SeekWindow is how far a new request may be from a running session's
	// position and still attach instead of respawning ffmpeg.
	SeekWindow = 30 * time.Second

	// KillGrace bounds how long Wait waits for the pipe to drain after cancel.
	KillGrace = time.Second

	chunkSize = 64 << 10
)

type Mode string

const (
	ModeAuto   Mode = "auto"
	ModeRemux  Mode = "remux"
	ModeEncode Mode = "encode"
)

// Config carries the ffmpeg settings from server config.
type Config struct {
	FFmpegPath    string
	HWAccel       string // videotoolbox|none|vaapi|qsv
	Preset        string
	MaxConcurrent int
}

// Request describes one transcoded stream.
type Request struct {
	Path          string
	Mode          Mode
	StartSeconds  float64
	BurnSubtitleN int // embedded-subtitle:N for burn-in; -1 disables
}

func (r Request) key() string {
	return fmt.Sprintf("%s|%s|%d", r.Path, r.Mode, r.BurnSubtitleN)
}

// Session is one live ffmpeg process feeding zero or more attached readers.
type Session struct {
	req        Request
	start      time.Time // wall clock when spawned
	posSeconds float64   // media position at spawn

	cancel context.CancelFunc
	cmd    *exec.Cmd
	stdout io.ReadCloser

	mu      sync.Mutex
	readers map[*reader]struct{}
	done    chan struct{}
	dead    bool
}

type reader struct {
	sess *Session
	ch   chan []byte
	pend []byte // bytes left from last chunk; owned by single Read caller
	once sync.Once
}

func (r *reader) Read(p []byte) (int, error) {
	if len(r.pend) > 0 {
		n := copy(p, r.pend)
		r.pend = r.pend[n:]
		return n, nil
	}
	b, ok := <-r.ch
	if !ok {
		return 0, io.EOF
	}
	n := copy(p, b)
	if n < len(b) {
		r.pend = b[n:]
	}
	return n, nil
}

func (r *reader) Close() error {
	s := r.sess
	s.mu.Lock()
	_, ok := s.readers[r]
	if ok {
		delete(s.readers, r)
	}
	s.mu.Unlock()
	if ok {
		r.once.Do(func() { close(r.ch) })
	}
	return nil
}

func (s *Session) closeReader(r *reader) {
	r.once.Do(func() {
		s.mu.Lock()
		delete(s.readers, r)
		s.mu.Unlock()
		close(r.ch)
	})
}

// drainReaders closes every attached reader, signalling EOF. Called when the
// ffmpeg process ends.
func (s *Session) drainReaders() {
	s.mu.Lock()
	rs := make([]*reader, 0, len(s.readers))
	for r := range s.readers {
		rs = append(rs, r)
		delete(s.readers, r)
	}
	s.mu.Unlock()
	for _, r := range rs {
		r.once.Do(func() { close(r.ch) })
	}
}

// Manager owns the registry and concurrency semaphore.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*Session
	sem      chan struct{}
}

func NewManager(cfg Config) *Manager {
	if cfg.MaxConcurrent < 1 {
		cfg.MaxConcurrent = 2
	}
	if cfg.Preset == "" {
		cfg.Preset = "veryfast"
	}
	if cfg.HWAccel == "" {
		cfg.HWAccel = "videotoolbox"
	}
	return &Manager{
		cfg:      cfg,
		sessions: make(map[string]*Session),
		sem:      make(chan struct{}, cfg.MaxConcurrent),
	}
}

// StopAll terminates every running ffmpeg (shutdown path).
func (m *Manager) StopAll() {
	m.mu.Lock()
	ss := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		ss = append(ss, s)
	}
	m.mu.Unlock()
	for _, s := range ss {
		s.stop()
	}
}

// sessionCount reports live registered sessions (test/introspection aid).
func (m *Manager) sessionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// Attach returns a reader for the requested stream, reusing a warm session
// when the seek target falls inside its fMP4 window; otherwise it respawns.
func (m *Manager) Attach(ctx context.Context, item *library.Item, mode Mode, startSeconds float64, burnSub int) (io.ReadCloser, func(), error) {
	if mode == ModeAuto {
		mode = m.pickMode(item)
	}
	req := Request{Path: item.FilePath, Mode: mode, StartSeconds: startSeconds, BurnSubtitleN: burnSub}

	m.mu.Lock()
	s, exists := m.sessions[req.key()]
	if exists && !s.isDead() {
		if absDiff(startSeconds, s.posSeconds) <= SeekWindow.Seconds() {
			r := s.attach()
			m.mu.Unlock()
			return r, func() { s.closeReader(r) }, nil
		}
	}
	delete(m.sessions, req.key())
	m.mu.Unlock()

	// Stop outside the registry lock: stop() waits for the distributor
	// goroutine, which itself needs m.mu to unregister.
	if exists && !s.isDead() {
		s.stop()
	}

	s, err := m.spawn(req)
	if err != nil {
		return nil, nil, err
	}
	r := s.attach()

	cleanup := func() {
		s.closeReader(r)
	}
	return r, cleanup, nil
}

// pickMode implements the remux-first fast path using probed codec info.
func (m *Manager) pickMode(item *library.Item) Mode {
	if item.ProbedCodec != nil {
		switch *item.ProbedCodec {
		case "h264", "avc1":
			return ModeRemux
		}
	}
	return ModeEncode
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func (s *Session) attach() *reader {
	r := &reader{sess: s, ch: make(chan []byte, 32)}
	s.mu.Lock()
	s.readers[r] = struct{}{}
	s.mu.Unlock()
	return r
}

func (s *Session) isDead() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Session) stop() {
	s.mu.Lock()
	if s.dead {
		s.mu.Unlock()
		return
	}
	s.dead = true
	s.cancel()
	s.mu.Unlock()
	<-s.done
}

// spawn starts ffmpeg with stdout piped and a distributor goroutine.
func (m *Manager) spawn(req Request) (*Session, error) {
	m.sem <- struct{}{} // bounded by MaxConcurrent

	ctx, cancel := context.WithCancel(context.Background())
	args := buildArgs(req, m.cfg)
	cmd := exec.CommandContext(ctx, m.cfg.FFmpegPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Setpgid makes the child its own group leader; kill the whole group
		// so ffmpeg cannot orphan helpers.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return syscall.ECANCELED
	}
	cmd.WaitDelay = KillGrace

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		<-m.sem
		return nil, fmt.Errorf("transcode: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		<-m.sem
		return nil, fmt.Errorf("transcode: start ffmpeg: %w", err)
	}

	s := &Session{
		req:        req,
		start:      time.Now(),
		posSeconds: req.StartSeconds,
		cancel:     cancel,
		cmd:        cmd,
		stdout:     stdout,
		readers:    make(map[*reader]struct{}),
		done:       make(chan struct{}),
	}

	go func() {
		defer func() {
			s.drainReaders()
			close(s.done)
			cmd.Wait()
			<-m.sem
			m.mu.Lock()
			if cur, ok := m.sessions[req.key()]; ok && cur == s {
				delete(m.sessions, req.key())
			}
			m.mu.Unlock()
		}()
		buf := make([]byte, chunkSize)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				s.mu.Lock()
				for r := range s.readers {
					select {
					case r.ch <- chunk:
					default:
						// Slow consumer: drop it rather than stall ffmpeg.
						go s.closeReader(r)
					}
				}
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	m.mu.Lock()
	m.sessions[req.key()] = s
	m.mu.Unlock()
	return s, nil
}

// buildArgs assembles the ffmpeg command line: fragmented MP4 to stdout,
// remux or encode modes, HWAccel variants, optional subtitle burn-in.
func buildArgs(req Request, cfg Config) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(req.StartSeconds, 'f', 3, 64),
		"-i", req.Path,
		"-map", "0:v:0",
		"-map", "0:a:0?",
	}

	switch {
	case req.Mode == ModeRemux:
		args = append(args, "-c:v", "copy", "-c:a", "copy")
	case req.Mode == ModeEncode && cfg.HWAccel == "videotoolbox":
		args = append(args, "-c:v", "h264_videotoolbox", "-b:v", "4M")
		extra := []string{"-c:a", "aac", "-b:a", "192k"}
		args = append(args, extra...)
	case req.Mode == ModeEncode && cfg.HWAccel == "vaapi":
		args = append(args,
			"-hwaccel", "vaapi", "-hwaccel_output_format", "vaapi",
			"-vf", "format=nv12,hwupload",
			"-c:v", "h264_vaapi", "-b:v", "4M")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	case req.Mode == ModeEncode && cfg.HWAccel == "qsv":
		args = append(args,
			"-hwaccel", "qsv",
			"-c:v", "h264_qsv", "-global_quality", "23")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	default: // software encode
		args = append(args,
			"-c:v", "libx264",
			"-preset", cfg.Preset,
			"-crf", "20")
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	}

	if req.BurnSubtitleN >= 0 && req.Mode != ModeRemux &&
		(cfg.HWAccel != "vaapi" && cfg.HWAccel != "qsv") {
		vf := fmt.Sprintf("subtitles='%s':si=%d", escapeFilterPath(req.Path), req.BurnSubtitleN)
		if idx := indexOfArg(args, "-vf"); idx >= 0 {
			args[idx+1] = args[idx+1] + "," + vf
		} else {
			args = append(args, "-vf", vf)
		}
	}

	args = append(args,
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"pipe:1")
	return args
}

func indexOfArg(args []string, name string) int {
	for i, a := range args {
		if a == name {
			return i
		}
	}
	return -1
}

// escapeFilterPath quotes a filesystem path for use inside an ffmpeg filter
// graph option value.
func escapeFilterPath(p string) string {
	r := strings.NewReplacer("\\", "\\\\", "'", "\\'", ":", "\\:", ",", "\\,", "[", "\\[", "]", "\\]")
	return r.Replace(p)
}
