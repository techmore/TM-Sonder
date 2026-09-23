package library

import (
	"sort"
	"strings"

	"tm-sonder/server/internal/api"
)

// OptimizationQueue is a read-only catalog analysis. It never starts an
// encoder or promotes a replacement. Audio values are header-based estimates;
// video savings are unknown until a representative AV1 sample is encoded.
type OptimizationQueue struct {
	EstimatedAudioSavingsBytes int64                `json:"estimatedAudioSavingsBytes"`
	AudioCandidates            int                  `json:"audioCandidates"`
	VideoSampleCandidates      int                  `json:"videoSampleCandidates"`
	ReviewCount                int                  `json:"reviewCount"`
	QualityNotice              string               `json:"qualityNotice"`
	Jobs                       []OptimizationJob    `json:"jobs"`
	Reviews                    []OptimizationReview `json:"reviews"`
}

type OptimizationJob struct {
	ID                    string   `json:"id"`
	Title                 string   `json:"title"`
	Kind                  string   `json:"kind"`
	Family                string   `json:"family"`
	SourceCodec           string   `json:"sourceCodec"`
	TargetCodec           string   `json:"targetCodec"`
	CurrentBytes          int64    `json:"currentBytes"`
	Channels              int      `json:"channels,omitempty"`
	TargetBitrateKbps     int      `json:"targetBitrateKbps,omitempty"`
	EstimatedOutputBytes  *int64   `json:"estimatedOutputBytes,omitempty"`
	EstimatedSavingsBytes *int64   `json:"estimatedSavingsBytes,omitempty"`
	EstimatedSavingsPct   *float64 `json:"estimatedSavingsPct,omitempty"`
	EstimateBasis         string   `json:"estimateBasis"`
	State                 string   `json:"state"`
	EmbeddedCoverPresent  bool     `json:"embeddedCoverPresent"`
	CatalogCoverAvailable bool     `json:"catalogCoverAvailable"`
	Reason                string   `json:"reason"`
	PlaybackNote          string   `json:"playbackNote"`
}

type OptimizationReview struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Family       string `json:"family"`
	CurrentBytes int64  `json:"currentBytes"`
	Reason       string `json:"reason"`
}

const qualityNotice = "AV1 and AAC-to-Opus are lossy encoding paths. Size estimates do not predict preserved quality. Audiobook jobs stage a full separate trial for listening; AV1 still requires representative scene samples. Confirm target-device playback before any replacement."

// OptimizationQueue analyzes known catalog probes and emits sample candidates
// without exposing filesystem paths to clients.
func (s *Store) OptimizationQueue() OptimizationQueue {
	result := OptimizationQueue{
		QualityNotice: qualityNotice,
		Jobs:          []OptimizationJob{},
		Reviews:       []OptimizationReview{},
	}
	for _, item := range s.InternalItems() {
		if item.Kind == api.KindAudiobook && item.Format == api.FormatM4B {
			if item.SizeBytes <= 0 || item.DurationSeconds <= 0 {
				appendOptimizationReview(&result, item, "audiobook-opus", "File size or duration is missing; refresh the media probe before estimating.")
				continue
			}
			analyzeAudiobookOptimization(&result, item)
		}
		if item.SizeBytes <= 0 || item.DurationSeconds <= 0 {
			continue
		}
		if isVideoKind(item.Kind) && isH264(item.ProbedCodec) &&
			(item.SizeBytes >= 2*1024*1024*1024 || (item.ProbedBitrate != nil && *item.ProbedBitrate >= 8_000_000)) {
			result.Jobs = append(result.Jobs, OptimizationJob{
				ID: item.ID, Title: item.Title, Kind: string(item.Kind),
				Family: "video-av1", SourceCodec: codecValue(item.ProbedCodec),
				TargetCodec: "av1", CurrentBytes: item.SizeBytes,
				EstimateBasis: "sample-required", State: "sample-required",
				Reason:       "Large H.264 source. Encode representative scenes at a quality target and compare measured output; no size estimate is claimed before sampling.",
				PlaybackNote: "Check HDR, bit depth, audio and subtitle preservation, and AV1 decode support on each target client before replacement.",
			})
			result.VideoSampleCandidates++
		}
	}
	sort.Slice(result.Jobs, func(i, j int) bool {
		a, b := result.Jobs[i], result.Jobs[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.EstimatedSavingsBytes != nil && b.EstimatedSavingsBytes != nil && *a.EstimatedSavingsBytes != *b.EstimatedSavingsBytes {
			return *a.EstimatedSavingsBytes > *b.EstimatedSavingsBytes
		}
		return a.CurrentBytes > b.CurrentBytes
	})
	sort.Slice(result.Reviews, func(i, j int) bool { return result.Reviews[i].CurrentBytes > result.Reviews[j].CurrentBytes })
	return result
}

func analyzeAudiobookOptimization(result *OptimizationQueue, item *Item) {
	codecs := item.ProbedAudioCodecs
	if len(codecs) == 0 {
		appendOptimizationReview(result, item, "audiobook-opus", "Audio codec probe is missing; refresh the probe before estimating.")
		return
	}
	allOpus := true
	containsAAC := false
	for _, codec := range codecs {
		allOpus = allOpus && strings.EqualFold(codec, "opus")
		containsAAC = containsAAC || strings.EqualFold(codec, "aac")
	}
	if allOpus || !containsAAC {
		if !allOpus {
			appendOptimizationReview(result, item, "audiobook-opus", "This source codec is outside the current AAC-to-Opus screening policy.")
		}
		return
	}
	review := func(reason string) { appendOptimizationReview(result, item, "audiobook-opus", reason) }
	if item.BookValidation != nil && strings.HasPrefix(strings.ToLower(*item.BookValidation), "invalid:") {
		review("Book integrity validation is marked invalid; resolve it before considering an encode.")
		return
	}
	if len(codecs) != 1 {
		review("Multiple audio tracks need an explicit preservation policy before an Opus estimate can be trusted.")
		return
	}
	if item.ProbedVideoStreams > 0 {
		review("Contains a non-cover video stream; review before audio-only encoding so content is not dropped.")
		return
	}
	if len(item.EmbeddedSubtitleTracks) > 0 || item.ProbedUnsupportedStreams > 0 {
		review("Source has subtitle or other non-audio streams that the current conversion policy does not preserve.")
		return
	}
	if !item.ProbedCoverKnown {
		review("Cover probe is missing; rescan before queuing so artwork preservation can be checked.")
		return
	}
	if !item.ProbedHasCover && (item.PosterPath == "" || item.PosterSource == "thumbnail") {
		review("No embedded cover or eligible catalog artwork is available; resolve and verify artwork before conversion.")
		return
	}
	if len(item.ProbedAudioChannels) != 1 || len(item.ProbedAudioBitrates) != 1 || item.ProbedAudioChannels[0] < 1 {
		review("Audio channel count or stream bitrate is missing; refresh the probe before estimating a target.")
		return
	}
	channels := item.ProbedAudioChannels[0]
	targetKbps := 0
	switch channels {
	case 1:
		targetKbps = 32
	case 2:
		targetKbps = 48
	default:
		review("More than two channels require a separate encoding policy and listening review.")
		return
	}
	if item.ProbedAudioBitrates[0] <= 0 {
		review("Audio bitrate is unavailable; run a representative sample instead of estimating from the container.")
		return
	}
	if item.ProbedAudioBitrates[0] <= targetKbps*2*1000 {
		return
	}
	// VBR/container overhead varies. A 5% allowance keeps this a rough estimate,
	// never a measured savings claim.
	estimated := int64(item.DurationSeconds * float64(targetKbps*125) * 1.05)
	if estimated >= item.SizeBytes || item.SizeBytes-estimated < 64*1024*1024 {
		return
	}
	savings := item.SizeBytes - estimated
	pct := float64(savings) * 100 / float64(item.SizeBytes)
	result.Jobs = append(result.Jobs, OptimizationJob{
		ID: item.ID, Title: item.Title, Kind: string(item.Kind),
		Family: "audiobook-opus", SourceCodec: "aac", TargetCodec: "opus-in-mp4",
		CurrentBytes: item.SizeBytes, EstimatedOutputBytes: &estimated,
		EstimatedSavingsBytes: &savings, EstimatedSavingsPct: &pct,
		Channels: channels, TargetBitrateKbps: targetKbps,
		EmbeddedCoverPresent:  item.ProbedHasCover,
		CatalogCoverAvailable: item.PosterPath != "" && item.PosterSource != "thumbnail",
		EstimateBasis:         "duration-and-target-bitrate", State: "full-staged-trial",
		Reason:       "AAC M4B has enough measured audio bitrate for a 32 kb/s mono or 48 kb/s stereo trial estimate. Queue a separate full-book output for listening review; the original remains untouched.",
		PlaybackNote: "Sonder serves M4B over byte ranges. iOS 27 simulator playback was confirmed; browser Opus-in-MP4 support varies and must be checked.",
	})
	result.AudioCandidates++
	result.EstimatedAudioSavingsBytes += savings
}

func appendOptimizationReview(result *OptimizationQueue, item *Item, family, reason string) {
	result.Reviews = append(result.Reviews, OptimizationReview{
		ID: item.ID, Title: item.Title, Family: family,
		CurrentBytes: item.SizeBytes, Reason: reason,
	})
	result.ReviewCount++
}

func isVideoKind(kind api.MediaKind) bool {
	return kind == api.KindMovie || kind == api.KindTVShow || kind == api.KindDocumentary
}

func isH264(codec *string) bool {
	return codec != nil && (strings.EqualFold(*codec, "h264") || strings.EqualFold(*codec, "avc1"))
}

func codecValue(codec *string) string {
	if codec == nil {
		return "unknown"
	}
	return strings.ToLower(*codec)
}
