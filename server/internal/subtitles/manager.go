// Package subtitles maintains durable, playable subtitle sidecars for the video catalog.
package subtitles

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/library"
)

const apiURL = "https://api.opensubtitles.com/api/v1"
const maxSubtitleBytes = 8 << 20

type Record struct {
	Title       string    `json:"title"`
	Language    string    `json:"language"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail,omitempty"`
	Source      string    `json:"source,omitempty"`
	CheckedAt   time.Time `json:"checkedAt"`
	RetryAt     time.Time `json:"retryAt,omitempty"`
	Fingerprint string    `json:"fingerprint"`
}
type Status struct {
	Running            bool              `json:"running"`
	ProviderConfigured bool              `json:"providerConfigured"`
	Languages          []string          `json:"languages"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	Counts             map[string]int    `json:"counts"`
	Items              map[string]Record `json:"items"`
}
type Manager struct {
	lastSaved                                       time.Time
	mu                                              sync.Mutex
	state                                           Status
	store                                           *library.Store
	path, snapshot, ffmpeg, key, username, password string
	client                                          *http.Client
	wake                                            chan struct{}
}

func New(store *library.Store, dataDir, snapshot, ffmpeg string) *Manager {
	m := &Manager{store: store, path: filepath.Join(dataDir, "subtitle-maintenance.json"), snapshot: snapshot, ffmpeg: ffmpeg,
		key: os.Getenv("SONDER_OPENSUBTITLES_API_KEY"), username: os.Getenv("SONDER_OPENSUBTITLES_USERNAME"), password: os.Getenv("SONDER_OPENSUBTITLES_PASSWORD"),
		client: &http.Client{Timeout: 30 * time.Second}, wake: make(chan struct{}, 1)}
	if b, err := os.ReadFile(m.path); err == nil {
		_ = json.Unmarshal(b, &m.state)
	}
	m.state.Running = false
	m.state.ProviderConfigured = m.key != ""
	m.state.Languages = []string{"en"}
	if raw := os.Getenv("SONDER_SUBTITLE_LANGUAGES"); raw != "" {
		langs := []string{}
		seen := map[string]bool{}
		for _, v := range strings.Split(raw, ",") {
			v = strings.ToLower(strings.TrimSpace(v))
			if len(v) == 2 && v[0] >= 'a' && v[0] <= 'z' && v[1] >= 'a' && v[1] <= 'z' && !seen[v] {
				langs = append(langs, v)
				seen[v] = true
			}
		}
		if len(langs) > 0 {
			m.state.Languages = langs
		}
	}
	if m.state.Items == nil {
		m.state.Items = map[string]Record{}
	}
	return m
}
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.state
	out.Items = make(map[string]Record, len(m.state.Items))
	out.Counts = map[string]int{}
	out.Languages = append([]string(nil), m.state.Languages...)
	for k, v := range m.state.Items {
		out.Items[k] = v
		out.Counts[v.Status]++
	}
	return out
}
func (m *Manager) Trigger() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) Run(ctx context.Context) {
	timer := time.NewTicker(time.Hour)
	defer timer.Stop()
	for {
		m.pass(ctx)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-m.wake:
		}
	}
}
func (m *Manager) save() {
	m.lastSaved = time.Now()
	state := m.Status()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(m.path), 0755)
	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, m.path)
	}
}
func (m *Manager) record(key string, r Record) {
	m.mu.Lock()
	m.state.Items[key] = r
	m.state.UpdatedAt = time.Now().UTC()
	m.mu.Unlock()
	if time.Since(m.lastSaved) >= 15*time.Second {
		m.save()
	}
}
func videoItem(it *library.Item) bool {
	return it.FilePath != "" && (it.Kind == api.KindMovie || it.Kind == api.KindTVShow || it.Kind == api.KindDocumentary)
}
func (m *Manager) pass(ctx context.Context) {
	m.mu.Lock()
	m.state.Running = true
	m.mu.Unlock()
	m.save()
	defer func() {
		m.mu.Lock()
		m.state.Running = false
		m.mu.Unlock()
		m.save()
		if m.snapshot != "" {
			_ = m.store.Flush(m.snapshot)
		}
	}()
	items := m.store.InternalItems()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	valid := map[string]bool{}
	languages := m.Status().Languages
	for _, it := range items {
		if videoItem(it) {
			for _, lang := range languages {
				valid[it.ID+":"+lang] = true
			}
		}
	}
	m.mu.Lock()
	for _, it := range items {
		if !videoItem(it) {
			continue
		}
		for _, lang := range languages {
			key := it.ID + ":" + lang
			if _, ok := m.state.Items[key]; !ok {
				m.state.Items[key] = Record{Title: it.Title, Language: lang, Status: "pending"}
			}
		}
	}
	for k := range m.state.Items {
		if !valid[k] {
			delete(m.state.Items, k)
		}
	}
	m.mu.Unlock()
	m.save()
	token, providerBlock := "", ""
	if m.key == "" {
		providerBlock = "OpenSubtitles API key is not configured"
	}
	for _, it := range items {
		if ctx.Err() != nil {
			return
		}
		if !videoItem(it) {
			continue
		}
		for _, lang := range languages {
			if ctx.Err() != nil {
				return
			}
			key := it.ID + ":" + lang
			now := time.Now().UTC()
			fingerprint := fmt.Sprintf("%s|%d|%d", it.FilePath, it.SizeBytes, it.ModTime.UnixNano())
			r := Record{Title: it.Title, Language: lang, CheckedAt: now, Fingerprint: fingerprint}
			if p := existingSidecar(it, lang); p != "" {
				m.attach(it, p)
				r.Status = "available"
				r.Source = "sidecar"
				m.record(key, r)
				continue
			}
			m.mu.Lock()
			old := m.state.Items[key]
			m.mu.Unlock()
			if old.Fingerprint == fingerprint && now.Before(old.RetryAt) && !(old.Status == "blocked" && m.key != "" && strings.Contains(old.Detail, "not configured")) {
				continue
			}
			r.Status = "checking"
			m.record(key, r)
			target := strings.TrimSuffix(it.FilePath, filepath.Ext(it.FilePath)) + "." + lang + ".sonder.vtt"
			extracted := false
			embedded := false
			for _, track := range it.EmbeddedSubtitleTracks {
				if track.LanguageCode == nil || !languageMatches(*track.LanguageCode, lang) {
					continue
				}
				embedded = true
				n, err := strconv.Atoi(strings.TrimPrefix(track.ID, "embedded-subtitle:"))
				if err != nil || n < 0 {
					continue
				}
				if m.extract(ctx, it.FilePath, n, target) == nil {
					extracted = true
					break
				}
			}
			if extracted {
				m.attach(it, target)
				r.Status = "available"
				r.Source = "embedded text"
				m.record(key, r)
				continue
			}
			if providerBlock == "" && m.username != "" && token == "" {
				var response struct {
					Token string `json:"token"`
				}
				err := m.request(ctx, "POST", "/login", map[string]string{"username": m.username, "password": m.password}, "", &response)
				if err != nil || response.Token == "" {
					providerBlock = "OpenSubtitles login failed; check account credentials"
				} else {
					token = response.Token
				}
			}
			if providerBlock != "" {
				r.Status = "blocked"
				r.Detail = providerBlock
				r.RetryAt = now.Add(24 * time.Hour)
			} else {
				p, err := m.download(ctx, it, lang, token)
				if err == nil && p != "" {
					m.attach(it, p)
					r.Status = "available"
					r.Source = "OpenSubtitles"
				} else if err == nil {
					r.Status = "not_found"
					r.Detail = "No confidently matching subtitle found"
					r.RetryAt = now.Add(7 * 24 * time.Hour)
				} else {
					r.Status = "error"
					r.Detail = err.Error()
					r.RetryAt = now.Add(24 * time.Hour)
					if strings.Contains(r.Detail, "HTTP 401") || strings.Contains(r.Detail, "HTTP 403") || strings.Contains(r.Detail, "HTTP 406") || strings.Contains(r.Detail, "HTTP 429") {
						providerBlock = r.Detail
					}
				}
			}
			if embedded {
				r.Detail = "Embedded subtitles are available for burn-in; text extraction failed. " + r.Detail
			}
			m.record(key, r)
			if providerBlock == "" {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}
	}
}
func languageMatches(v, lang string) bool {
	v = strings.ToLower(strings.Split(v, "-")[0])
	if lang == "en" && v == "eng" {
		return true
	}
	return v == lang
}
func existingSidecar(it *library.Item, lang string) string {
	base := strings.TrimSuffix(filepath.Base(it.FilePath), filepath.Ext(it.FilePath))
	entries, _ := os.ReadDir(filepath.Dir(it.FilePath))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".srt" && ext != ".vtt" && ext != ".ass" && ext != ".ssa" {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if !strings.HasPrefix(stem, base) {
			continue
		}
		suffix := strings.TrimPrefix(stem, base)
		if suffix != "" && !strings.ContainsRune("._- ", rune(suffix[0])) {
			continue
		}
		tagged := false
		for _, tag := range strings.FieldsFunc(suffix, func(r rune) bool { return r == '.' || r == '_' || r == '-' || r == ' ' }) {
			if languageMatches(tag, lang) {
				tagged = true
			}
		}
		// Untagged sidecars remain playable, but are not assumed to be English.
		if tagged {
			p := filepath.Join(filepath.Dir(it.FilePath), name)
			if info, err := os.Stat(p); err == nil && info.Size() > 0 {
				return p
			}
		}
	}
	return ""
}
func (m *Manager) attach(it *library.Item, p string) {
	m.store.Update(it.ID, func(cur *library.Item) bool {
		if cur.FilePath != it.FilePath {
			return false
		}
		for _, existing := range cur.SidecarPaths {
			if existing == p {
				return false
			}
		}
		cur.SidecarPaths = append(cur.SidecarPaths, p)
		sort.Strings(cur.SidecarPaths)
		return true
	})
}
func (m *Manager) extract(ctx context.Context, path string, index int, target string) error {
	if m.ffmpeg == "" {
		return fmt.Errorf("ffmpeg unavailable")
	}
	job, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".sonder-subtitle-*.vtt")
	if err != nil {
		return err
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	cmd := exec.CommandContext(job, m.ffmpeg, "-nostdin", "-v", "error", "-y", "-i", path, "-map", fmt.Sprintf("0:s:%d", index), "-c:s", "webvtt", "-f", "webvtt", name)
	if err = cmd.Run(); err != nil {
		return fmt.Errorf("embedded track cannot be extracted as text")
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if !validSubtitle(b) {
		return fmt.Errorf("empty subtitle track")
	}
	return install(name, target)
}
func validSubtitle(b []byte) bool {
	return len(b) > 20 && len(b) <= maxSubtitleBytes && strings.Contains(string(b), "-->") && !strings.Contains(strings.ToLower(string(b)), "<html")
}

// Hard-link publication is atomic and never overwrites a user supplied sidecar.
func install(tmp, target string) error {
	if err := os.Chmod(tmp, 0644); err != nil {
		return err
	}
	return os.Link(tmp, target)
}
func movieHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() < 131072 {
		return "", fmt.Errorf("media too small for hash")
	}
	sum := uint64(info.Size())
	b := make([]byte, 65536)
	for _, off := range []int64{0, info.Size() - 65536} {
		if _, err = f.ReadAt(b, off); err != nil {
			return "", err
		}
		for i := 0; i < len(b); i += 8 {
			sum += binary.LittleEndian.Uint64(b[i : i+8])
		}
	}
	return fmt.Sprintf("%016x", sum), nil
}
func (m *Manager) request(ctx context.Context, method, path string, body any, token string, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", m.key)
	req.Header.Set("User-Agent", "Sonder v1")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("OpenSubtitles request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("OpenSubtitles HTTP %d (authentication, quota, or provider failure)", res.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid OpenSubtitles response")
	}
	return nil
}

type candidate struct {
	Attributes struct {
		Language    string `json:"language"`
		HashMatch   bool   `json:"moviehash_match"`
		ForeignOnly bool   `json:"foreign_parts_only"`
		AI          bool   `json:"ai_translated"`
		Machine     bool   `json:"machine_translated"`
		Feature     struct {
			Title     string `json:"title"`
			MovieName string `json:"movie_name"`
			Year      int    `json:"year"`
			Season    int    `json:"season_number"`
			Episode   int    `json:"episode_number"`
		} `json:"feature_details"`
		Files []struct {
			ID int `json:"file_id"`
		} `json:"files"`
	} `json:"attributes"`
}

func normalized(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func (m *Manager) download(ctx context.Context, it *library.Item, lang, token string) (string, error) {
	hash, err := movieHash(it.FilePath)
	if err != nil {
		return "", fmt.Errorf("cannot hash media file")
	}
	q := url.Values{"languages": {lang}, "moviehash": {hash}, "order_by": {"download_count"}, "order_direction": {"desc"}, "foreign_parts_only": {"exclude"}, "machine_translated": {"exclude"}, "ai_translated": {"exclude"}}
	if it.Kind == api.KindTVShow {
		if it.ShowTitle == nil || it.SeasonNumber == nil || it.EpisodeNumber == nil {
			return "", nil
		}
		q.Set("query", *it.ShowTitle)
		q.Set("season_number", strconv.Itoa(*it.SeasonNumber))
		q.Set("episode_number", strconv.Itoa(*it.EpisodeNumber))
		q.Set("type", "episode")
	} else {
		q.Set("query", it.Title)
		q.Set("type", "movie")
		if it.Year > 0 {
			q.Set("year", strconv.Itoa(it.Year))
		}
	}
	var results struct {
		Data []candidate `json:"data"`
	}
	if err = m.request(ctx, "GET", "/subtitles?"+q.Encode(), nil, token, &results); err != nil {
		return "", err
	}
	fileID := 0
	// A hash match is preferred. Title-only downloads require exact movie title
	// and year; episodes require a hash match to avoid same-named series/remakes.
	for _, hashOnly := range []bool{true, false} {
		for _, c := range results.Data {
			a := c.Attributes
			if !languageMatches(a.Language, lang) || a.ForeignOnly || a.AI || a.Machine || len(a.Files) != 1 {
				continue
			}
			matched := a.HashMatch
			if !hashOnly && it.Kind != api.KindTVShow && it.Year > 0 && a.Feature.Year == it.Year && (normalized(a.Feature.Title) == normalized(it.Title) || normalized(a.Feature.MovieName) == normalized(it.Title)) {
				matched = true
			}
			if matched {
				fileID = a.Files[0].ID
				break
			}
		}
		if fileID > 0 {
			break
		}
	}
	if fileID == 0 {
		return "", nil
	}
	var download struct {
		Link string `json:"link"`
	}
	if err = m.request(ctx, "POST", "/download", map[string]any{"file_id": fileID, "sub_format": "srt"}, token, &download); err != nil {
		return "", err
	}
	u, err := url.Parse(download.Link)
	if err != nil || u.Scheme != "https" || !(u.Hostname() == "opensubtitles.com" || strings.HasSuffix(u.Hostname(), ".opensubtitles.com")) {
		return "", fmt.Errorf("invalid subtitle download host")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("invalid subtitle link")
	}
	res, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("subtitle file download failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("subtitle file HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxSubtitleBytes+1))
	if err != nil || !validSubtitle(b) {
		return "", fmt.Errorf("subtitle file is empty or invalid")
	}
	target := strings.TrimSuffix(it.FilePath, filepath.Ext(it.FilePath)) + "." + lang + ".opensubtitles.srt"
	f, err := os.CreateTemp(filepath.Dir(target), ".sonder-subtitle-*.srt")
	if err != nil {
		return "", fmt.Errorf("cannot write subtitles beside media")
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return "", fmt.Errorf("cannot save subtitle")
	}
	if err = install(f.Name(), target); err != nil {
		return "", fmt.Errorf("cannot publish subtitle (existing file or storage error)")
	}
	return target, nil
}
