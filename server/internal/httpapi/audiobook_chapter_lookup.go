package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
)

const chapterMapLimit = 1024 * 1024

var chapterASIN = regexp.MustCompile(`^[A-Z0-9]{10}$`)
var chapterRegions = map[string]bool{"au": true, "ca": true, "de": true, "es": true, "fr": true, "in": true, "it": true, "jp": true, "us": true, "uk": true}

// Upstream is fixed, never supplied by a browser request. Redirects are disabled.
var chapterLookupClient = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

type chapterLookupRequest struct {
	ASIN   string `json:"asin"`
	Region string `json:"region"`
}
type chapterMapMarker struct {
	Index        int     `json:"index"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
}
type chapterLookupPreview struct {
	ASIN                      string             `json:"asin"`
	Region                    string             `json:"region"`
	Title                     string             `json:"title"`
	Authors                   []string           `json:"authors"`
	Narrators                 []string           `json:"narrators"`
	Description               string             `json:"description"`
	Publisher                 string             `json:"publisher"`
	RuntimeSeconds            float64            `json:"runtimeSeconds"`
	LocalDurationSeconds      float64            `json:"localDurationSeconds"`
	DurationDifferenceSeconds float64            `json:"durationDifferenceSeconds"`
	MatchConfidence           string             `json:"matchConfidence"`
	SourceURL                 string             `json:"sourceURL"`
	IsAccurate                bool               `json:"isAccurate"`
	Chapters                  []chapterMapMarker `json:"chapters"`
}
type chapterMapRequest struct {
	Chapters         []chapterMapMarker `json:"chapters"`
	SourceURL        string             `json:"sourceURL"`
	EditionConfirmed bool               `json:"editionConfirmed"`
	RuntimeSeconds   float64            `json:"runtimeSeconds"`
	ASIN             string             `json:"asin,omitempty"`
	Region           string             `json:"region,omitempty"`
}
type chapterMapSidecar struct {
	Version     int                `json:"version"`
	Fingerprint string             `json:"fingerprint"`
	SourceURL   string             `json:"sourceURL"`
	ImportedAt  time.Time          `json:"importedAt"`
	Chapters    []chapterMapMarker `json:"chapters"`
}

func validChapterLookup(request chapterLookupRequest) (chapterLookupRequest, error) {
	request.ASIN = strings.ToUpper(strings.TrimSpace(request.ASIN))
	request.Region = strings.ToLower(strings.TrimSpace(request.Region))
	if request.Region == "" {
		request.Region = "us"
	}
	if !chapterASIN.MatchString(request.ASIN) || !chapterRegions[request.Region] {
		return request, errors.New("Provide a ten-character Audible ASIN and a supported region")
	}
	return request, nil
}

// chapterEditorAllowed scopes shared library edits to the owner, pairing-token
// administrator, or the same guarded loopback access used by the server.
func (s *Server) chapterEditorAllowed(r *http.Request) bool {
	if isLoopback(peerHost(r)) && hostIsLoopback(r.Host) {
		return true
	}
	if s.cfg().PairingToken != "" && s.tokenMatches(r) {
		return true
	}
	username, ok := s.sessionUsername(r)
	return ok && s.accounts != nil && username == s.accounts.Username()
}
func (s *Server) requireChapterEditor(w http.ResponseWriter, r *http.Request) bool {
	if s.chapterEditorAllowed(r) {
		return true
	}
	writeError(w, http.StatusForbidden, "Only the library owner can edit or look up shared chapter maps")
	return false
}
func (s *Server) chapterBookParts(id string) ([]catalogPart, error) {
	it, ok := s.store.Get(id)
	if !ok || it.Kind != api.KindAudiobook {
		return nil, errors.New("Audiobook not found")
	}
	parts, _, multi := s.bookPartsFor(it)
	if !multi {
		parts = []catalogPart{{ID: it.ID, Title: it.Title, Index: 1, DurationSeconds: it.DurationSeconds}}
	}
	for _, part := range parts {
		if part.DurationSeconds <= 0 || math.IsNaN(part.DurationSeconds) || math.IsInf(part.DurationSeconds, 0) {
			return nil, errors.New("Scan the recording to determine every part's runtime before importing chapters")
		}
	}
	return parts, nil
}
func chapterBookDuration(parts []catalogPart) float64 {
	var duration float64
	for _, part := range parts {
		duration += part.DurationSeconds
	}
	return duration
}
func (s *Server) chapterBookFingerprint(parts []catalogPart) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%s\x00%.6f\n", part.ID, part.DurationSeconds)
		if item, ok := s.store.Get(part.ID); ok {
			fmt.Fprintf(hash, "%s\x00", item.FilePath)
			if info, err := os.Stat(item.FilePath); err == nil {
				fmt.Fprintf(hash, "%d:%d\n", info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
func (s *Server) chapterMapPath(parts []catalogPart) string {
	// Stable group identity uses IDs, while the separate fingerprint also detects
	// changed durations. A rescan cannot accidentally apply stale timestamps.
	hash := sha256.New()
	for _, p := range parts {
		fmt.Fprintf(hash, "%s\x00", p.ID)
	}
	return filepath.Join(s.cfg().DataDir, "audiobook-chapter-maps", hex.EncodeToString(hash.Sum(nil))+".json")
}
func runtimeCompatible(remote, local float64) bool {
	return remote > 0 && local > 0 && math.Abs(remote-local) <= math.Max(60, local*0.02)
}

func lookupAudnexus(ctx context.Context, client *http.Client, base string, request chapterLookupRequest, localDuration float64) (chapterLookupPreview, error) {
	request, err := validChapterLookup(request)
	if err != nil {
		return chapterLookupPreview{}, err
	}
	fetch := func(suffix string, result any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/books/"+request.ASIN+suffix+"?region="+request.Region, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "TM-Sonder chapter review")
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("Chapter service could not be reached")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Chapter service returned HTTP %d; check the ASIN and region", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, chapterMapLimit+1))
		if err != nil {
			return err
		}
		if len(data) > chapterMapLimit {
			return errors.New("Chapter service response is too large")
		}
		return json.Unmarshal(data, result)
	}
	var book struct {
		Title   string `json:"title"`
		Authors []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Narrators []struct {
			Name string `json:"name"`
		} `json:"narrators"`
		Description string  `json:"description"`
		Publisher   string  `json:"publisherName"`
		Runtime     float64 `json:"runtimeLengthMin"`
	}
	var timings struct {
		Chapters []struct {
			Title  string  `json:"title"`
			Start  float64 `json:"startOffsetMs"`
			Length float64 `json:"lengthMs"`
		} `json:"chapters"`
		Runtime  float64 `json:"runtimeLengthMs"`
		Accurate bool    `json:"isAccurate"`
	}
	if err := fetch("", &book); err != nil {
		return chapterLookupPreview{}, err
	}
	if err := fetch("/chapters", &timings); err != nil {
		return chapterLookupPreview{}, err
	}
	result := chapterLookupPreview{ASIN: request.ASIN, Region: request.Region, Title: book.Title, Authors: []string{}, Narrators: []string{}, Description: book.Description, Publisher: book.Publisher, RuntimeSeconds: timings.Runtime / 1000, LocalDurationSeconds: localDuration, SourceURL: "https://api.audnex.us/books/" + request.ASIN + "/chapters?region=" + request.Region, IsAccurate: timings.Accurate, Chapters: []chapterMapMarker{}}
	if result.RuntimeSeconds <= 0 {
		result.RuntimeSeconds = book.Runtime * 60
	}
	for _, author := range book.Authors {
		result.Authors = append(result.Authors, author.Name)
	}
	for _, narrator := range book.Narrators {
		result.Narrators = append(result.Narrators, narrator.Name)
	}
	for i, ch := range timings.Chapters {
		result.Chapters = append(result.Chapters, chapterMapMarker{Index: i + 1, Title: ch.Title, StartSeconds: ch.Start / 1000, EndSeconds: (ch.Start + ch.Length) / 1000})
	}
	if _, err := validateChapterMarkers(result.Chapters, result.RuntimeSeconds); err != nil {
		return chapterLookupPreview{}, fmt.Errorf("Chapter service returned unusable timings: %w", err)
	}
	result.DurationDifferenceSeconds = result.RuntimeSeconds - localDuration
	result.MatchConfidence = "runtime-mismatch"
	if runtimeCompatible(result.RuntimeSeconds, localDuration) {
		result.MatchConfidence = "runtime-compatible-needs-edition-review"
	}
	return result, nil
}
func (s *Server) handleAudiobookChapterLookup(w http.ResponseWriter, r *http.Request) {
	if !s.requireChapterEditor(w, r) {
		return
	}
	parts, err := s.chapterBookParts(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request chapterLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid chapter lookup request")
		return
	}
	if _, err := validChapterLookup(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := lookupAudnexus(ctx, chapterLookupClient, "https://api.audnex.us", request, chapterBookDuration(parts))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func validateChapterMarkers(markers []chapterMapMarker, duration float64) ([]chapterMapMarker, error) {
	if len(markers) == 0 || len(markers) > 2000 || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return nil, errors.New("Provide 1–2000 chapter markers and a known recording runtime")
	}
	result := append([]chapterMapMarker(nil), markers...)
	for i := range result {
		ch := &result[i]
		ch.Title = strings.TrimSpace(ch.Title)
		ch.Index = i + 1
		if ch.Title == "" || len(ch.Title) > 500 || math.IsNaN(ch.StartSeconds) || math.IsInf(ch.StartSeconds, 0) || ch.StartSeconds < 0 || ch.StartSeconds >= duration {
			return nil, fmt.Errorf("Chapter %d needs a title and an in-range start time", i+1)
		}
		if i > 0 && ch.StartSeconds <= result[i-1].StartSeconds {
			return nil, errors.New("Chapter start times must be strictly increasing")
		}
		if ch.EndSeconds == 0 {
			ch.EndSeconds = duration
			if i+1 < len(result) {
				ch.EndSeconds = result[i+1].StartSeconds
			}
		}
		if math.IsNaN(ch.EndSeconds) || math.IsInf(ch.EndSeconds, 0) || ch.EndSeconds <= ch.StartSeconds || ch.EndSeconds > duration+0.01 {
			return nil, fmt.Errorf("Chapter %d has an out-of-range end time", i+1)
		}
		if i+1 < len(result) && ch.EndSeconds > result[i+1].StartSeconds+0.01 {
			return nil, errors.New("Chapter ranges must not overlap")
		}
		if ch.EndSeconds > duration {
			ch.EndSeconds = duration
		}
	}
	return result, nil
}
func validateChapterSource(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(raw) > 2048 {
		return errors.New("Provide an HTTPS edition source URL")
	}
	return nil
}
func importedChapterTimeline(itemID string, parts []catalogPart, sidecar chapterMapSidecar) audiobookChapterTimeline {
	timeline := audiobookChapterTimeline{ItemID: itemID, Available: true, PartCount: len(parts), DurationSeconds: chapterBookDuration(parts), Chapters: []audiobookTimelineChapter{}, Source: "reviewed-import", SourceURL: sidecar.SourceURL, ImportedAt: &sidecar.ImportedAt}
	for _, ch := range sidecar.Chapters {
		offset := 0.0
		for i, part := range parts {
			if ch.StartSeconds < offset+part.DurationSeconds {
				timeline.Chapters = append(timeline.Chapters, audiobookTimelineChapter{Index: ch.Index, Title: ch.Title, PartID: part.ID, PartIndex: i + 1, StartSeconds: ch.StartSeconds, EndSeconds: ch.EndSeconds})
				break
			}
			offset += part.DurationSeconds
		}
	}
	timeline.ChapterCount = len(timeline.Chapters)
	return timeline
}
func (s *Server) loadImportedChapterMap(parts []catalogPart) (chapterMapSidecar, error) {
	file, err := os.Open(s.chapterMapPath(parts))
	if err != nil {
		return chapterMapSidecar{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, chapterMapLimit+1))
	if err != nil || len(data) > chapterMapLimit {
		return chapterMapSidecar{}, errors.New("Invalid chapter map size")
	}
	var sidecar chapterMapSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return sidecar, err
	}
	if sidecar.Version != 1 || sidecar.Fingerprint != s.chapterBookFingerprint(parts) {
		return sidecar, errors.New("Recording changed since chapter import")
	}
	if err := validateChapterSource(sidecar.SourceURL); err != nil {
		return sidecar, err
	}
	sidecar.Chapters, err = validateChapterMarkers(sidecar.Chapters, chapterBookDuration(parts))
	return sidecar, err
}
func (s *Server) handleAudiobookChapterMap(w http.ResponseWriter, r *http.Request) {
	if !s.requireChapterEditor(w, r) {
		return
	}
	parts, err := s.chapterBookParts(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if r.Method == http.MethodDelete {
		if err := os.Remove(s.chapterMapPath(parts)); err != nil && !os.IsNotExist(err) {
			writeError(w, 500, "Unable to reset chapter map")
			return
		}
		writeJSON(w, 200, map[string]bool{"reset": true})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, chapterMapLimit)
	var request chapterMapRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, 400, "Invalid chapter map request")
		return
	}
	if !request.EditionConfirmed {
		writeError(w, 422, "Confirm the title, narrator and edition before applying external chapter timings")
		return
	}
	if err := validateChapterSource(request.SourceURL); err != nil {
		writeError(w, 422, err.Error())
		return
	}
	if request.RuntimeSeconds != 0 && !runtimeCompatible(request.RuntimeSeconds, chapterBookDuration(parts)) {
		writeError(w, 422, "Edition runtime does not match this recording; align the timestamps manually before import")
		return
	}
	markers, err := validateChapterMarkers(request.Chapters, chapterBookDuration(parts))
	if err != nil {
		writeError(w, 422, err.Error())
		return
	}
	sidecar := chapterMapSidecar{Version: 1, Fingerprint: s.chapterBookFingerprint(parts), SourceURL: request.SourceURL, ImportedAt: time.Now().UTC(), Chapters: markers}
	currentParts, err := s.chapterBookParts(r.PathValue("id"))
	if err != nil || s.chapterBookFingerprint(currentParts) != sidecar.Fingerprint {
		writeError(w, http.StatusConflict, "Recording changed during chapter import; reload and review again")
		return
	}
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		writeError(w, 500, "Unable to encode chapter map")
		return
	}
	path := s.chapterMapPath(parts)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		writeError(w, 500, "Unable to create chapter map directory")
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".chapter-map-*")
	if err != nil {
		writeError(w, 500, "Unable to save chapter map")
		return
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		writeError(w, 500, "Unable to save chapter map")
		return
	}
	writeJSON(w, 200, importedChapterTimeline(r.PathValue("id"), parts, sidecar))
}
