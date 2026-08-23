package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/probe"
)

var subtitleExts = map[string]bool{
	"srt": true, "vtt": true, "ass": true, "ssa": true,
}

var artworkPosterNames = []string{
	"poster.jpg", "poster.png", "cover.jpg", "cover.png",
	"folder.jpg", "folder.png",
}

var artworkBackdropNames = []string{
	"background.jpg", "background.png", "fanart.jpg", "fanart.png",
	"backdrop.jpg", "backdrop.png",
}

// ScanState summarizes current/last scan activity for /api/status.
type ScanState struct {
	Scanning   bool       `json:"scanning"`
	LastScanAt *time.Time `json:"lastScanAt"`
	ItemsSeen  int        `json:"itemsSeen"`
	Added      int        `json:"added"`
	Updated    int        `json:"updated"`
	Removed    int        `json:"removed"`
	Skipped    int        `json:"skippedUnchanged"`
}

// Scanner walks configured libraries and reconciles them into the Store.
type Scanner struct {
	store   *Store
	prober  Prober
	thumbFn ThumbFunc
	workers int

	mu    sync.Mutex
	state ScanState
}

// Prober supplies ffprobe-derived metadata for new or changed files.
type Prober interface {
	ProbeResult(ctx context.Context, path string, size int64, mod time.Time) (*probe.Result, error)
}

// ThumbFunc generates a poster image for one item, returning the stored
// path. Implementations must tolerate being called twice (idempotent).
type ThumbFunc func(ctx context.Context, itemID, videoPath string, durationSeconds float64) (string, error)

func NewScanner(store *Store) *Scanner {
	return &Scanner{store: store, workers: 2}
}

// SetProber enables post-scan probing with a worker pool (default 2).
func (sc *Scanner) SetProber(p Prober, workers int) {
	sc.prober = p
	if workers > 0 {
		sc.workers = workers
	}
}

// SetThumbnailGen enables poster generation for videos without artwork.
// It runs on the probe worker pool after each successful probe.
func (sc *Scanner) SetThumbnailGen(fn ThumbFunc) {
	sc.thumbFn = fn
}

func (sc *Scanner) State() ScanState {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.state
}

// ScanResult aggregates one full scan pass across libraries.
type ScanResult struct {
	Added   int
	Updated int
	Removed int
	Skipped int
}

// ScanAll scans every library, then prunes vanished files.
func (sc *Scanner) ScanAll(libs []config.Library) (ScanResult, error) {
	sc.mu.Lock()
	if sc.state.Scanning {
		sc.mu.Unlock()
		return ScanResult{}, ErrScanInProgress
	}
	sc.state.Scanning = true
	sc.state.Added, sc.state.Updated, sc.state.Removed, sc.state.Skipped = 0, 0, 0, 0
	sc.mu.Unlock()

	defer func() {
		now := time.Now().UTC()
		sc.mu.Lock()
		sc.state.Scanning = false
		sc.state.LastScanAt = &now
		sc.mu.Unlock()
	}()

	var res ScanResult
	keep := make(map[string]bool)
	var pending []probeJob
	for _, lib := range libs {
		r, err := sc.scanLibraryInto(lib, keep, &pending)
		if err != nil {
			// A broken library (unreadable subtree, vanished mount) must
			// not abort the whole pass: later libraries still scan and,
			// critically, RetainOnly still runs so items whose files were
			// renamed/deleted in OTHER libraries get pruned.
			continue
		}
		res.Added += r.Added
		res.Updated += r.Updated
		res.Skipped += r.Skipped
	}
	if removed := sc.store.RetainOnly(keep); removed > 0 {
		res.Removed = removed
		sc.store.RecordActivity("Library pruned",
			itoa(removed)+" missing item(s) removed", "trash")
	}

	sc.probePending(pending)

	sc.mu.Lock()
	sc.state.ItemsSeen = sc.store.Count()
	sc.state.Added, sc.state.Updated, sc.state.Removed, sc.state.Skipped =
		res.Added, res.Updated, res.Removed, res.Skipped
	sc.mu.Unlock()

	sc.store.RecordActivity("Library scanned",
		"added "+itoa(res.Added)+", updated "+itoa(res.Updated)+
			", removed "+itoa(res.Removed), "magazine")
	return res, nil
}

// RefreshTracks re-probes one item's embedded tracks using the configured
// prober and persists the result. Implements httpapi.TrackRefresher.
func (sc *Scanner) RefreshTracks(itemID string) error {
	if sc.prober == nil {
		return errProbingDisabled
	}
	it, ok := sc.store.Get(itemID)
	if !ok {
		return os.ErrNotExist
	}
	res, err := sc.prober.ProbeResult(context.Background(), it.FilePath, it.SizeBytes, it.ModTime)
	if err != nil {
		return err
	}
	applyProbe(it, res)
	sc.store.Upsert(it)
	return nil
}

// errProbingDisabled is returned when no prober is configured.
var errProbingDisabled = errProbeDisabled{}

type errProbeDisabled struct{}

func (errProbeDisabled) Error() string { return "library: probing disabled" }

// ErrScanInProgress guards against overlapping manual+startup scans.
var ErrScanInProgress = scanInProgressError{}

type scanInProgressError struct{}

func (scanInProgressError) Error() string { return "library: scan already in progress" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// probeJob identifies one file whose metadata must be (re)probed.
type probeJob struct {
	ItemID string
	Path   string
	Size   int64
	Mod    time.Time
}

// probePending runs new/updated files through the prober with a small worker
// pool and merges results back into the catalog, generating posters for
// videos that have none. No-op when probing is disabled.
func (sc *Scanner) probePending(jobs []probeJob) {
	if sc.prober == nil || len(jobs) == 0 {
		return
	}
	workers := sc.workers
	if workers < 1 {
		workers = 2
	}
	ctx := context.Background()
	in := make(chan probeJob)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				res, err := sc.prober.ProbeResult(ctx, j.Path, j.Size, j.Mod)
				if err != nil || res == nil {
					continue
				}
				it, ok := sc.store.Get(j.ItemID)
				if !ok {
					continue
				}
				applyProbe(it, res)
				if sc.thumbFn != nil && it.ProbedWidth != nil && it.PosterPath == "" {
					if p, err := sc.thumbFn(ctx, it.ID, it.FilePath, res.DurationSeconds); err == nil && p != "" {
						it.PosterPath = p
						u := "/artwork/poster/" + it.ID
						it.PosterURL = &u
						it.PosterSource = "thumbnail"
					}
				}
				sc.store.Upsert(it)
			}
		}()
	}
	for _, j := range jobs {
		in <- j
	}
	close(in)
	wg.Wait()
}

// applyProbe merges a probe Result into an Item per Swift field semantics:
// probed* wire fields plus embedded track tables; duration fills in when the
// catalog has none.
func applyProbe(it *Item, res *probe.Result) {
	if res.DurationSeconds > 0 && it.DurationSeconds == 0 {
		it.DurationSeconds = res.DurationSeconds
	}
	it.ProbedWidth = res.Width
	it.ProbedHeight = res.Height
	it.ProbedCodec = res.Codec
	it.ProbedBitrate = res.Bitrate
	now := time.Now().UTC()
	it.TrackProbeUpdatedAt = &now
	it.EmbeddedAudioTracks = append([]api.PlaybackTrack(nil), res.AudioTracks...)
	it.EmbeddedSubtitleTracks = append([]api.PlaybackTrack(nil), res.SubtitleTracks...)
}

// scanLibraryInto walks one library root. Items whose stable ID lands in keep
// are marked retained; incremental skips use size+mtime comparisons against
// existing catalog entries.
func (sc *Scanner) scanLibraryInto(lib config.Library, keep map[string]bool, pending *[]probeJob) (ScanResult, error) {
	var res ScanResult
	root := lib.Path
	info, err := os.Stat(root)
	if err != nil {
		return res, err
	}
	if !info.IsDir() {
		return res, filepath.SkipDir
	}
	// Resolve a symlinked library root to its target: WalkDir Lstats the
	// root and would otherwise treat the link itself as a plain file and
	// never descend into the library.
	if resolved, rerr := filepath.EvalSymlinks(root); rerr == nil {
		root = resolved
	}

	libID := lib.ID
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// NAS roots routinely contain permission-restricted directories
			// (#recycle, backups, root-only shares). Skip them rather than
			// aborting the whole library walk.
			if errors.Is(err, fs.ErrPermission) {
				return filepath.SkipDir
			}
			return err
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && path != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
		format := api.FormatForExtension(ext)
		if format == api.FormatUnknown {
			return nil
		}

		canonical, cerr := filepath.Abs(path)
		if cerr != nil {
			canonical = filepath.Clean(path)
		}
		id := StableID(canonical)
		keep[id] = true

		st, serr := d.Info()
		if serr != nil {
			return serr
		}

		existing, found := sc.store.Get(id)
		// Self-healing artwork: an unchanged file still gets a probe job when
		// it has never been probed and thumbnail generation could give it a
		// poster, or when Plex-style local artwork appeared since last scan.
		// Items also rebuild when their library assignment went stale (e.g.
		// the library table was edited between scans). Everything else
		// unchanged is skipped entirely.
		libStale := found && (existing.LibraryID == nil || *existing.LibraryID != lib.ID)
		unchanged := found && !libStale &&
			existing.SizeBytes == st.Size() && existing.ModTime.Equal(st.ModTime()) &&
			len(existing.SidecarPaths) == countSidecars(path, st)
		wantsFirstProbe := unchanged && sc.thumbFn != nil &&
			existing.PosterPath == "" && existing.TrackProbeUpdatedAt == nil
		newLocalArt := false
		if unchanged && !wantsFirstProbe && existing.PosterPath == "" {
			b := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if firstExisting(filepath.Dir(path), filepath.Dir(filepath.Dir(path)), artworkPosterNames, b) != "" {
				newLocalArt = true
			}
		}
		if unchanged && !wantsFirstProbe && !newLocalArt {
			res.Skipped++
			return nil
		}

		item := sc.buildItem(canonical, id, st, format, libID, lib.Kind)
		sc.store.Upsert(item)
		// Skip ffprobe for formats it can't meaningfully report on (ebooks,
		// text). Video/audio still probe for tracks + thumbnails.
		if probeWorthy(format) {
			*pending = append(*pending, probeJob{ItemID: id, Path: canonical, Size: st.Size(), Mod: st.ModTime()})
		}
		if found {
			res.Updated++
		} else {
			res.Added++
		}
		return nil
	})
	if err == filepath.SkipDir {
		err = nil
	}
	return res, err
}

// probeWorthy reports whether ffprobe can extract useful metadata for the
// format. Ebooks (and unknown types) are registered in the catalog but never
// queued for probing — ffprobe fails on them and thumbnails don't apply.
func probeWorthy(f api.MediaFormat) bool {
	switch f {
	case api.FormatEPUB, api.FormatPDF, api.FormatUnknown:
		return false
	}
	return true
}

// inferKind resolves the item kind: audiobook/ebook by extension, else the
// configured library kind ("all" falls back to episode detection + movie).
func inferKind(path string, format api.MediaFormat, libKind string) api.MediaKind {
	switch format {
	case api.FormatM4B, api.FormatMP3, api.FormatM4A:
		return api.KindAudiobook
	case api.FormatEPUB, api.FormatPDF:
		return api.KindEbook
	case api.FormatMP4, api.FormatMOV, api.FormatMKV, api.FormatAVI:
		// Stray video inside an ebook library must not masquerade as a
		// book; everywhere else normal kind resolution applies.
		if libKind == string(api.KindEbook) {
			return api.KindMovie
		}
	}
	switch libKind {
	case string(api.KindTVShow):
		return api.KindTVShow
	case string(api.KindDocumentary):
		return api.KindDocumentary
	case string(api.KindAudiobook):
		return api.KindAudiobook
	case string(api.KindEbook):
		return api.KindEbook
	case string(api.KindAll):
		if looksEpisodic(filepath.Base(path)) {
			return api.KindTVShow
		}
		return api.KindMovie
	default:
		return api.KindMovie
	}
}

// looksEpisodic reports whether a filename carries SxxEyy / NxNN codes.
func looksEpisodic(base string) bool {
	name := strings.TrimSuffix(base, filepath.Ext(base))
	name = cleanMediaTitle(name)
	for _, re := range []*regexp.Regexp{reTVEpisodeShow, reTVBare, reTVCross} {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

// buildItem parses one discovered file into a full catalog Item.
func (sc *Scanner) buildItem(path, id string, st os.FileInfo, format api.MediaFormat, libraryID, libKind string) *Item {
	kind := inferKind(path, format, libKind)
	parsed := ParseFilename(path, string(kind))

	item := &Item{
		MediaItem: api.MediaItem{
			ID:               id,
			Title:            parsed.Title,
			Subtitle:         parsed.Subtitle,
			Kind:             kind,
			Year:             parsed.Year,
			DurationSeconds:  0,
			Format:           format,
			LibraryID:        &libraryID,
			Tags:             []string{},
			Summary:          "",
			ShowTitle:        nil,
			MetadataIDSource: nil,
			MetadataID:       nil,
			Edition:          nil,
			SplitPart:        nil,
		},
		FilePath:  path,
		SizeBytes: st.Size(),
		ModTime:   st.ModTime(),
	}
	if parsed.ShowTitle != "" {
		s := parsed.ShowTitle
		item.ShowTitle = &s
	}
	if parsed.Season != nil {
		item.SeasonNumber = intPtr(*parsed.Season)
	}
	if parsed.Episode != nil {
		item.EpisodeNumber = intPtr(*parsed.Episode)
	}
	if parsed.MetadataIDSource != "" {
		s := parsed.MetadataIDSource
		item.MetadataIDSource = &s
	}
	if parsed.MetadataID != "" {
		s := parsed.MetadataID
		item.MetadataID = &s
	}
	if parsed.Edition != "" {
		s := parsed.Edition
		item.Edition = &s
	}
	if parsed.SplitPart != "" {
		s := parsed.SplitPart
		item.SplitPart = &s
	}
	if parsed.Series != "" {
		item.Studio = parsed.Series
	}

	item.SidecarPaths = findSidecars(path)

	// Artwork discovery.
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parent := filepath.Dir(filepath.Dir(path))
	if p := firstExisting(filepath.Dir(path), parent, artworkPosterNames, base); p != "" {
		item.PosterPath = p
		u := "/artwork/poster/" + id
		item.PosterURL = &u
		item.PosterSource = "local"
	}
	if b := firstExisting(filepath.Dir(path), parent, artworkBackdropNames, base); b != "" {
		item.BackdropPath = b
		u := "/artwork/backdrop/" + id
		item.BackdropURL = &u
	}

	// Preserve progress already recorded for this item (rescan safety).
	if prev, ok := sc.store.ProgressFor(id); ok {
		item.ProgressSeconds = prev.Seconds
	}
	return item
}

// SidecarTracks builds the sidecar:N track list for an item, mirroring
// SonderHTTPServer.sidecarSubtitleTracks: labels are the subtitle filename
// minus extension, language codes are nil, URLs point at /subtitles/{id}/{n}.
func SidecarTracks(item *Item) []api.PlaybackTrack {
	out := make([]api.PlaybackTrack, 0, len(item.SidecarPaths))
	for i, sp := range item.SidecarPaths {
		bn := filepath.Base(sp)
		label := strings.TrimSuffix(bn, filepath.Ext(bn))
		u := "/subtitles/" + item.ID + "/" + itoa(i)
		out = append(out, api.PlaybackTrack{
			ID:    "sidecar:" + itoa(i),
			Label: label,
			Kind:  api.TrackSidecar,
			URL:   &u,
		})
	}
	return out
}

// MergedSubtitleTracks returns embedded then sidecar tracks in the order the
// Swift server composes them for playback sessions.
func MergedSubtitleTracks(item *Item) []api.PlaybackTrack {
	return append(append([]api.PlaybackTrack(nil), item.EmbeddedSubtitleTracks...), SidecarTracks(item)...)
}

// findSidecars lists subtitle files adjacent to media whose base name has the
// media's base name as a prefix (Swift SonderMediaParser.localAssets rule).
func findSidecars(mediaPath string) []string {
	folder := filepath.Dir(mediaPath)
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		en := e.Name()
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(en), "."))
		if !subtitleExts[ext] {
			continue
		}
		bn := strings.TrimSuffix(en, filepath.Ext(en))
		if strings.HasPrefix(bn, base) {
			out = append(out, filepath.Join(folder, en))
		}
	}
	sort.Strings(out)
	return out
}

func countSidecars(mediaPath string, _ os.FileInfo) int {
	return len(findSidecars(mediaPath))
}

// firstExisting finds the first present artwork file in folder or parent.
func firstExisting(folder, parent string, names []string, base string) string {
	candidates := append([]string{}, names...)
	candidates = append(candidates,
		base+".jpg", base+".png",
		base+"-poster.jpg", base+"-poster.png",
	)
	for _, dir := range []string{folder, parent} {
		for _, n := range candidates {
			p := filepath.Join(dir, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}
