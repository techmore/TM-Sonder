// Package probe wraps ffprobe to extract stream metadata and embedded track
// listings, mirroring the Swift app's AVFoundation-based SonderMediaProbe.
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"tm-sonder/server/internal/api"
)

// Result carries the fields the catalog needs from one file.
type Result struct {
	DurationSeconds float64
	Width           *int
	Height          *int
	Codec           *string
	Bitrate         *int
	AudioTracks     []api.PlaybackTrack
	SubtitleTracks  []api.PlaybackTrack
}

// ffprobeOutput models the relevant subset of -print_format json output.
type ffprobeOutput struct {
	Streams []struct {
		Index       int    `json:"index"`
		CodecType   string `json:"codec_type"`
		CodecName   string `json:"codec_name"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		BitRate     string `json:"bit_rate"`
		Duration    string `json:"duration"`
		Disposition struct {
			Default int `json:"default"`
		} `json:"disposition"`
		Tags struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// Probe runs ffprobe against path and maps streams to wire tracks.
// Track IDs follow the Swift convention: embedded-audio:N /
// embedded-subtitle:N where N counts streams of that type in file order.
func Probe(ctx context.Context, ffprobePath, mediaPath string) (*Result, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", mediaPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("probe: ffprobe %s: %w", mediaPath, err)
	}
	return Parse(out)
}

// Parse maps raw ffprobe JSON to a Result. Exported for table tests.
func Parse(data []byte) (*Result, error) {
	var raw ffprobeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("probe: parse ffprobe output: %w", err)
	}

	res := &Result{}
	audioN, subN := 0, 0
	for i := range raw.Streams {
		s := &raw.Streams[i]
		switch s.CodecType {
		case "audio":
			res.AudioTracks = append(res.AudioTracks, track(
				"embedded-audio", audioN, s))
			audioN++
		case "subtitle":
			res.SubtitleTracks = append(res.SubtitleTracks, track(
				"embedded-subtitle", subN, s))
			subN++
		case "video":
			if res.Width == nil && s.Width > 0 {
				w, h := s.Width, s.Height
				res.Width, res.Height = &w, &h
				name := normalizeCodec(s.CodecName)
				if name != "" {
					res.Codec = &name
				}
				if br, ok := atoi64(s.BitRate); ok {
					b := int(br)
					res.Bitrate = &b
				}
			}
		}
	}
	if d, ok := atof(raw.Format.Duration); ok {
		res.DurationSeconds = d
	}
	return res, nil
}

func track(prefix string, n int, s *struct {
	Index       int    `json:"index"`
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	BitRate     string `json:"bit_rate"`
	Duration    string `json:"duration"`
	Disposition struct {
		Default int `json:"default"`
	} `json:"disposition"`
	Tags struct {
		Language string `json:"language"`
		Title    string `json:"title"`
	} `json:"tags"`
}) api.PlaybackTrack {
	id := fmt.Sprintf("%s:%d", prefix, n)
	code := languageCode(s.Tags.Language)
	label := strings.TrimSpace(s.Tags.Title)
	if label == "" && code != nil {
		if name, ok := displayNames[*code]; ok {
			label = name
		}
	}
	if label == "" {
		label = fmt.Sprintf("Track %d", n+1)
	}
	return api.PlaybackTrack{
		ID:           id,
		Label:        label,
		LanguageCode: code,
		Kind:         api.TrackEmbedded,
		URL:          nil,
	}
}

// languageCode ports SonderMediaProbe.languageCode: first component of a
// BCP-47 tag, lowercased; "und" (undetermined) becomes nil. ffprobe's ISO
// 639-2 codes are normalized to the two-letter forms AVFoundation produces.
func languageCode(tag string) *string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}
	first := strings.SplitN(tag, "-", 2)[0]
	code := strings.ToLower(first)
	if mapped, ok := iso6392ToBCP47[code]; ok {
		code = mapped
	}
	if code == "und" || code == "undetermined" {
		return nil
	}
	return &code
}

var iso6392ToBCP47 = map[string]string{
	"eng": "en", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de",
	"spa": "es", "ita": "it", "jpn": "ja", "rus": "ru", "chi": "zh",
	"zho": "zh", "kor": "ko", "por": "pt", "dut": "nl", "nld": "nl",
	"swe": "sv", "nor": "no", "dan": "da", "fin": "fi", "pol": "pl",
	"tur": "tr", "ara": "ar", "hin": "hi",
}

var displayNames = map[string]string{
	"en": "English", "ja": "Japanese", "fr": "French", "de": "German",
	"es": "Spanish", "it": "Italian", "pt": "Portuguese", "ru": "Russian",
	"zh": "Chinese", "ko": "Korean", "hi": "Hindi", "ar": "Arabic",
	"nl": "Dutch", "sv": "Swedish", "no": "Norwegian", "da": "Danish",
	"fi": "Finnish", "pl": "Polish", "tr": "Turkish",
}

// normalizeCodec maps ffprobe codec names toward the CMMediaType four-char
// style the Swift client sees (vp09 etc.), keeping h264/hevc as-is.
func normalizeCodec(name string) string {
	switch name {
	case "vp9":
		return "vp09"
	default:
		return name
	}
}

func atoi64(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

func atof(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return f, true
}
