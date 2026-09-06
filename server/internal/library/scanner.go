package library

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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
	store        *Store
	prober       Prober
	thumbFn      ThumbFunc
	thumbDir     string // artwork output dir (<id>.jpg); enables orphan reattach
	workers      int
	thumbWorkers int

	mu    sync.Mutex
	state ScanState

	// 1-entry memo of the last directory listing consulted by the unchanged
	// check, fresh once per scan: ScanAll clears dirlistFresh, so each
	// folder is ReadDir'd at most once per pass and the listing is reused
	// across that folder's files (TV walks visit the same folder once per
	// episode). Refreshing per scan means subtitles added between scans
	// are still detected on unchanged media files.
	dirlistMu      sync.Mutex
	dirlistPath    string
	dirlistEntries []os.DirEntry
	dirlistFresh   bool
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

// SetThumbnailDir tells the scanner where generated posters live
// (<thumbDir>/<itemID>.jpg). buildItem reattaches an orphaned thumbnail when
// local discovery finds nothing — e.g. after a rebuild dropped the reference
// while the image file survived.
func (sc *Scanner) SetThumbnailDir(dir string) {
	sc.thumbDir = dir
}

// thumbExists reports whether a generated poster file survives on disk.
func (sc *Scanner) thumbExists(id string) bool {
	if sc.thumbDir == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(sc.thumbDir, id+".jpg"))
	return err == nil && !st.IsDir()
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

	// Directory listings are memoized per folder for exactly one scan pass.
	sc.dirlistMu.Lock()
	sc.dirlistFresh = false
	sc.dirlistMu.Unlock()

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
			strconv.Itoa(removed)+" missing item(s) removed", "trash")
	}

	sc.probePending(pending)

	sc.mu.Lock()
	sc.state.ItemsSeen = sc.store.Count()
	sc.state.Added, sc.state.Updated, sc.state.Removed, sc.state.Skipped =
		res.Added, res.Updated, res.Removed, res.Skipped
	sc.mu.Unlock()

	sc.store.RecordActivity("Library scanned",
		"added "+strconv.Itoa(res.Added)+", updated "+strconv.Itoa(res.Updated)+
			", removed "+strconv.Itoa(res.Removed), "magazine")
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

	// Thumbnails run in their own small pool: probing is I/O-bound on the
	// media mount while poster generation is CPU-bound ffmpeg work, and
	// running both back-to-back in one worker slot doubles per-file latency.
	type thumbJob struct {
		itemID   string
		path     string
		duration float64
	}
	thumbs := make(chan thumbJob)
	thumbWorkers := sc.thumbWorkers
	if thumbWorkers <= 0 {
		thumbWorkers = runtime.NumCPU() / 4
		if thumbWorkers < 1 {
			thumbWorkers = 1
		}
		if thumbWorkers > 4 {
			thumbWorkers = 4
		}
	}
	var twg sync.WaitGroup
	if sc.thumbFn != nil {
		for i := 0; i < thumbWorkers; i++ {
			twg.Add(1)
			go func() {
				defer twg.Done()
				for tj := range thumbs {
					p, err := sc.thumbFn(ctx, tj.itemID, tj.path, tj.duration)
					if err != nil || p == "" {
						continue
					}
					it, ok := sc.store.Get(tj.itemID)
					if !ok {
						continue
					}
					it.PosterPath = p
					u := "/artwork/poster/" + it.ID
					it.PosterURL = &u
					it.PosterSource = "thumbnail"
					sc.store.Upsert(it)
				}
			}()
		}
	}

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
				sc.store.Upsert(it)
				// Upsert first so the thumbnail pool observes probed state.
				if sc.thumbFn != nil && it.ProbedWidth != nil && it.PosterPath == "" {
					thumbs <- thumbJob{itemID: it.ID, path: it.FilePath, duration: res.DurationSeconds}
				}
			}
		}()
	}
	for _, j := range jobs {
		in <- j
	}
	close(in)
	wg.Wait()
	if sc.thumbFn != nil {
		close(thumbs)
		twg.Wait()
	}
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
			existing.ParseVersion == ParserVersion &&
			len(existing.SidecarPaths) == sc.sidecarCount(path)
		wantsFirstProbe := unchanged && sc.thumbFn != nil &&
			existing.PosterPath == "" && existing.TrackProbeUpdatedAt == nil
		newLocalArt := false
		if unchanged && !wantsFirstProbe && existing.PosterPath == "" {
			b := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if firstExisting(filepath.Dir(path), filepath.Dir(filepath.Dir(path)), artworkPosterNames, b) != "" {
				newLocalArt = true
			}
			// Orphaned thumbnail: the generated file survives while the item
			// reference was lost. Rebuild reattaches it in buildItem (no
			// re-probe: TrackProbeUpdatedAt carries over).
			if !newLocalArt && sc.thumbExists(id) {
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
		// For books the parser's "series" slot carries the filename-extracted
		// author; mirror it so the API layer doesn't guess from Tags.
		if kind == api.KindEbook || kind == api.KindAudiobook {
			a := parsed.Series
			item.Author = &a
		}
	}

	item.SidecarPaths = findSidecars(path)
	item.ParseVersion = ParserVersion

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
	// Orphan reattach: a generated thumbnail whose reference was lost (e.g.
	// dropped by an older rebuild) still lives at <thumbDir>/<id>.jpg.
	if item.PosterPath == "" && sc.thumbExists(id) {
		item.PosterPath = filepath.Join(sc.thumbDir, id+".jpg")
		u := "/artwork/poster/" + id
		item.PosterURL = &u
		item.PosterSource = "thumbnail"
	}

	// Preserve progress already recorded for this item (rescan safety).
	if prev, ok := sc.store.ProgressFor(id); ok {
		item.ProgressSeconds = prev.Seconds
	}
	// Preserve enrichment- and probe-carried state across rebuilds (parser
	// version bumps, library reassignment): these are not parse outputs and
	// would otherwise vanish until the next probe/enrich pass.
	if prev, ok := sc.store.Get(id); ok {
		item.Summary = prev.Summary
		item.Tags = prev.Tags
		item.Author = prev.Author
		item.Narrator = prev.Narrator
		item.ProbedWidth = prev.ProbedWidth
		item.ProbedHeight = prev.ProbedHeight
		item.ProbedCodec = prev.ProbedCodec
		item.ProbedBitrate = prev.ProbedBitrate
		item.TrackProbeUpdatedAt = prev.TrackProbeUpdatedAt
		if item.DurationSeconds == 0 {
			item.DurationSeconds = prev.DurationSeconds
		}
		// Keep provider artwork when local discovery came up empty. Thumbnails
		// included: the generated file survives rebuilds even when the reference
		// was dropped (see orphan reattach below).
		if item.PosterPath == "" && prev.PosterPath != "" {
			item.PosterPath = prev.PosterPath
			item.PosterURL = prev.PosterURL
			item.PosterSource = prev.PosterSource
		}
		if item.BackdropPath == "" && prev.BackdropPath != "" {
			item.BackdropPath = prev.BackdropPath
			item.BackdropURL = prev.BackdropURL
		}
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
		u := "/subtitles/" + item.ID + "/" + strconv.Itoa(i)
		out = append(out, api.PlaybackTrack{
			ID:    "sidecar:" + strconv.Itoa(i),
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
		if isSubtitleSidecar(e, base) {
			out = append(out, filepath.Join(folder, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// sidecarCount mirrors len(findSidecars(path)) but reuses a memoized
// directory listing, fresh once per scan (ScanAll clears dirlistFresh).
// TV folders are hit once per episode, so the memo collapses n-1 redundant
// ReadDir calls per folder while still noticing subtitles added between scans.
func (sc *Scanner) sidecarCount(mediaPath string) int {
	folder := filepath.Dir(mediaPath)
	sc.dirlistMu.Lock()
	defer sc.dirlistMu.Unlock()
	if sc.dirlistPath != folder || !sc.dirlistFresh {
		entries, err := os.ReadDir(folder)
		if err != nil {
			sc.dirlistPath, sc.dirlistEntries, sc.dirlistFresh = "", nil, false
			return 0
		}
		sc.dirlistPath, sc.dirlistEntries, sc.dirlistFresh = folder, entries, true
	}
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	n := 0
	for _, e := range sc.dirlistEntries {
		if isSubtitleSidecar(e, base) {
			n++
		}
	}
	return n
}

// isSubtitleSidecar reports whether a directory entry is a subtitle file
// belonging to the media with the given base name (no extension).
func isSubtitleSidecar(e os.DirEntry, base string) bool {
	if e.IsDir() {
		return false
	}
	en := e.Name()
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(en), "."))
	if !subtitleExts[ext] {
		return false
	}
	bn := strings.TrimSuffix(en, filepath.Ext(en))
	return strings.HasPrefix(bn, base)
}

// firstExisting finds the first present artwork file in folder or parent.
// Exact-case probes run first; if none hit, a case-insensitive listing
// fallback catches Cover.jpg / Poster.PNG style names on case-sensitive
// mounts (ext4 NFS shares) without hurting case-correct folders.
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
	for _, dir := range []string{folder, parent} {
		if p := caseInsensitiveMatch(dir, candidates); p != "" {
			return p
		}
	}
	return ""
}

// caseInsensitiveMatch returns the first candidate present in dir under any
// letter casing. Empty when dir is unreadable or holds no candidate.
func caseInsensitiveMatch(dir string, candidates []string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	lower := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower[strings.ToLower(e.Name())] = e.Name()
	}
	for _, n := range candidates {
		if actual, ok := lower[strings.ToLower(n)]; ok {
			return filepath.Join(dir, actual)
		}
	}
	return ""
}
