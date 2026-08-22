package library

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"tm-sonder/server/internal/api"
)

// swiftSnapshot mirrors the macOS app's SonderSnapshot encoding
// (JSONEncoder.sonder: ISO8601 dates, camelCase keys). Only the fields the Go
// server consumes are declared; unknown Swift-only keys are ignored.
type swiftSnapshot struct {
	Items            []swiftItem          `json:"items"`
	Progress         []api.ProgressRecord `json:"progress"`
	MediaDirectories []api.MediaDirectory `json:"mediaDirectories"`
	ServerSettings   *api.ServerSettings  `json:"serverSettings"`
}

type swiftItem struct {
	ID                     string              `json:"id"`
	Title                  string              `json:"title"`
	Subtitle               string              `json:"subtitle"`
	Kind                   api.MediaKind       `json:"kind"`
	Studio                 string              `json:"studio"`
	Year                   int                 `json:"year"`
	DurationSeconds        float64             `json:"durationSeconds"`
	Format                 api.MediaFormat     `json:"format"`
	LibraryID              *string             `json:"libraryID"`
	Tags                   []string            `json:"tags"`
	Summary                string              `json:"summary"`
	ProgressSeconds        float64             `json:"progressSeconds"`
	ShowTitle              *string             `json:"showTitle"`
	SeasonNumber           *int                `json:"seasonNumber"`
	EpisodeNumber          *int                `json:"episodeNumber"`
	MetadataIDSource       *string             `json:"metadataIDSource"`
	MetadataID             *string             `json:"metadataID"`
	Edition                *string             `json:"edition"`
	SplitPart              *string             `json:"splitPart"`
	IsPlaceholder          bool                `json:"isPlaceholder"`
	LocalPosterPath        *string             `json:"localPosterPath"`
	LocalBackdropPath      *string             `json:"localBackdropPath"`
	SubtitlePaths          []string            `json:"subtitlePaths"`
	EmbeddedAudioTracks    []api.PlaybackTrack `json:"embeddedAudioTracks"`
	EmbeddedSubtitleTracks []api.PlaybackTrack `json:"embeddedSubtitleTracks"`
	TrackProbeUpdatedAt    *time.Time          `json:"trackProbeUpdatedAt"`
	ProbedWidth            *int                `json:"probedWidth"`
	ProbedHeight           *int                `json:"probedHeight"`
	ProbedCodec            *string             `json:"probedCodec"`
	ProbedBitrate          *int                `json:"probedBitrate"`
	BookValidation         *string             `json:"bookValidation"`
	CoverSource            *string             `json:"coverSource"`
	SourcePath             *string             `json:"sourcePath"`
}

// ImportResult reports what a Swift snapshot migration yielded.
type ImportResult struct {
	Items         int
	SkippedNoFile int
	MissingOnDisk int
	Progress      int
	Directories   int
}

// ImportSwiftLibraryJSON reads the Mac app's library.json and loads it into
// the store, preserving IDs so iOS clients keep their bookmarks/progress.
// Items whose source file no longer exists on disk are counted but skipped.
func ImportSwiftLibraryJSON(s *Store, path string) (*ImportResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("library: read Swift snapshot %s: %w", path, err)
	}
	var snap swiftSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("library: parse Swift snapshot %s: %w", path, err)
	}

	res := &ImportResult{Progress: len(snap.Progress), Directories: len(snap.MediaDirectories)}
	items := make([]*Item, 0, len(snap.Items))
	for _, si := range snap.Items {
		if si.ID == "" || si.SourcePath == nil || *si.SourcePath == "" {
			res.SkippedNoFile++
			continue
		}
		src := *si.SourcePath
		if st, err := os.Stat(src); err != nil {
			res.MissingOnDisk++
			continue
		} else if it := itemFromSwift(si, src, st.Size(), st.ModTime()); it != nil {
			items = append(items, it)
			res.Items++
		}
	}

	s.Upsert(items...)
	for _, p := range snap.Progress {
		s.SetProgress(p)
	}
	if len(snap.MediaDirectories) > 0 {
		s.SetDirectories(snap.MediaDirectories)
	}
	s.RecordActivity("Library imported", fmt.Sprintf("Migrated %d items from macOS app", res.Items), "shippingbox")
	return res, nil
}

func itemFromSwift(si swiftItem, sourcePath string, size int64, mod time.Time) *Item {
	it := &Item{
		MediaItem: api.MediaItem{
			ID:                     si.ID,
			Title:                  si.Title,
			Subtitle:               si.Subtitle,
			Kind:                   si.Kind,
			Studio:                 si.Studio,
			Year:                   si.Year,
			DurationSeconds:        si.DurationSeconds,
			Format:                 si.Format,
			LibraryID:              si.LibraryID,
			Tags:                   si.Tags,
			Summary:                si.Summary,
			ProgressSeconds:        si.ProgressSeconds,
			ShowTitle:              si.ShowTitle,
			SeasonNumber:           si.SeasonNumber,
			EpisodeNumber:          si.EpisodeNumber,
			MetadataIDSource:       si.MetadataIDSource,
			MetadataID:             si.MetadataID,
			Edition:                si.Edition,
			SplitPart:              si.SplitPart,
			IsPlaceholder:          si.IsPlaceholder,
			EmbeddedAudioTracks:    si.EmbeddedAudioTracks,
			EmbeddedSubtitleTracks: si.EmbeddedSubtitleTracks,
			TrackProbeUpdatedAt:    si.TrackProbeUpdatedAt,
			ProbedWidth:            si.ProbedWidth,
			ProbedHeight:           si.ProbedHeight,
			ProbedCodec:            si.ProbedCodec,
			ProbedBitrate:          si.ProbedBitrate,
			BookValidation:         si.BookValidation,
			CoverSource:            si.CoverSource,
		},
		FilePath:     sourcePath,
		SidecarPaths: si.SubtitlePaths,
		SizeBytes:    size,
		ModTime:      mod,
	}
	if si.LocalPosterPath != nil {
		it.PosterPath = *si.LocalPosterPath
		it.PosterSource = "local"
	}
	if si.LocalBackdropPath != nil {
		it.BackdropPath = *si.LocalBackdropPath
	}
	return it
}
