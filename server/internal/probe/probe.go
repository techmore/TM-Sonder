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
	// AudioCodecs holds each audio stream's codec name in the same order as
	// AudioTracks, so the transcoder can decide whether audio can be copied
	// into an MP4 container or must be re-encoded.
	AudioCodecs        []string
	AudioChannels      []int
	AudioBitrates      []int
	VideoStreamCount   int
	HasAttachedPicture bool
	UnsupportedStreams int
	// StreamCount is the number of streams ffprobe reported. Zero means the
	// file had no readable streams (broken/unsupported), which callers use to
	// distinguish "probed successfully" from "probe returned nothing".
	StreamCount int
	// Tags carries the container's own metadata (the M4B atom tags written by
	// the retag pass and by other rippers). Author, narrator, series, and
	// summary live here rather than in the filename, so this is the only way
	// the catalog learns them without a metadata provider.
	Tags FileTags
}

// FileTags is the normalized subset of an ffprobe format.tag map that the
// catalog consumes. Keys are matched case-insensitively because MP4 atom tags
// arrive as "artist" from ffprobe but as "ARTIST" in FFMETADATA and "©ART" in
// ID3-derived containers.
type FileTags struct {
	Title       string
	Artist      string
	AlbumArtist string
	Album       string
	Composer    string
	Genre       string
	Date        string
	Comment     string
	Description string
}

// ffprobeStream is the subset of one ffprobe stream the catalog consumes.
// Named so the parser and track() cannot drift apart.
type ffprobeStream struct {
	Index       int    `json:"index"`
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	CodecTag    string `json:"codec_tag_string"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	BitRate     string `json:"bit_rate"`
	Channels    int    `json:"channels"`
	Duration    string `json:"duration"`
	Disposition struct {
		Default     int `json:"default"`
		AttachedPic int `json:"attached_pic"`
	} `json:"disposition"`
	Tags struct {
		Language string `json:"language"`
		Title    string `json:"title"`
	} `json:"tags"`
}

// ffprobeOutput models the relevant subset of -print_format json output.
type ffprobeOutput struct {
	Streams  []ffprobeStream   `json:"streams"`
	Chapters []json.RawMessage `json:"chapters"`
	Format   struct {
		Duration string            `json:"duration"`
		BitRate  string            `json:"bit_rate"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// Probe runs ffprobe against path and maps streams to wire tracks.
// Track IDs follow the Swift convention: embedded-audio:N /
// embedded-subtitle:N where N counts streams of that type in file order.
func Probe(ctx context.Context, ffprobePath, mediaPath string) (*Result, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters", mediaPath)
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

	res := &Result{StreamCount: len(raw.Streams)}
	audioN, subN := 0, 0
	for i := range raw.Streams {
		s := &raw.Streams[i]
		switch s.CodecType {
		case "audio":
			res.AudioTracks = append(res.AudioTracks, track(
				"embedded-audio", audioN, s))
			res.AudioCodecs = append(res.AudioCodecs, strings.ToLower(s.CodecName))
			res.AudioChannels = append(res.AudioChannels, s.Channels)
			if bitrate, ok := atoi64(s.BitRate); ok {
				res.AudioBitrates = append(res.AudioBitrates, int(bitrate))
			} else {
				res.AudioBitrates = append(res.AudioBitrates, 0)
			}
			audioN++
		case "subtitle":
			res.SubtitleTracks = append(res.SubtitleTracks, track(
				"embedded-subtitle", subN, s))
			subN++
		case "video":
			if s.Disposition.AttachedPic == 0 {
				res.VideoStreamCount++
			} else {
				res.HasAttachedPicture = true
			}
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
		default:
			if !(s.CodecType == "data" && s.CodecTag == "text" && len(raw.Chapters) > 0) {
				res.UnsupportedStreams++
			}
		}
	}
	if d, ok := atof(raw.Format.Duration); ok {
		res.DurationSeconds = d
	}
	// Fall back to the container bitrate when the video stream omits bit_rate.
	if res.Bitrate == nil {
		if br, ok := atoi64(raw.Format.BitRate); ok {
			b := int(br)
			res.Bitrate = &b
		}
	}
	res.Tags = parseFileTags(raw.Format.Tags)
	return res, nil
}

// tagAliases maps every spelling seen across containers onto one canonical
// field. ffprobe lower-cases most MP4 atoms, but FFMETADATA1 files written by
// the retag tools use the uppercase names, and ID3-derived containers use the
// "©" forms. Matching is done on a case-folded, ©-stripped key.
var tagAliases = map[string]string{
	"title":        "Title",
	"artist":       "Artist",
	"art":          "Artist", // ID3 "©ART" after the © is stripped
	"album_artist": "AlbumArtist",
	"albumartist":  "AlbumArtist",
	"album":        "Album",
	"composer":     "Composer",
	"genre":        "Genre",
	"date":         "Date",
	"year":         "Date",
	"comment":      "Comment",
	"description":  "Description",
}

// parseFileTags normalizes ffprobe's format.tags map into FileTags. Later
// duplicate keys win only when they are non-empty, so a container that lists
// both "©ART" and "artist" does not end up blank.
func parseFileTags(raw map[string]string) FileTags {
	var out FileTags
	if len(raw) == 0 {
		return out
	}
	assign := map[string]*string{
		"Title":       &out.Title,
		"Artist":      &out.Artist,
		"AlbumArtist": &out.AlbumArtist,
		"Album":       &out.Album,
		"Composer":    &out.Composer,
		"Genre":       &out.Genre,
		"Date":        &out.Date,
		"Comment":     &out.Comment,
		"Description": &out.Description,
	}
	for k, v := range raw {
		field, ok := tagAliases[normalizeTagKey(k)]
		if !ok {
			continue
		}
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if *assign[field] == "" {
			*assign[field] = v
		}
	}
	return out
}

// normalizeTagKey folds a tag name for alias lookup: lowercase, © dropped, and
// non-alphanumerics collapsed to underscores. A freeform atom
// ("----:com.apple.iTunes:ALBUMARTIST") is first reduced to its last dotted
// segment, since the real field name is the trailing component.
func normalizeTagKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	k = strings.ReplaceAll(k, "©", "")
	// A freeform atom is "----:com.apple.iTunes:FIELD": the real field name is
	// the trailing segment, so cut at the last separator of any kind.
	if i := strings.LastIndexAny(k, ".:/\\"); i >= 0 && i+1 < len(k) {
		k = k[i+1:]
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range k {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func track(prefix string, n int, s *ffprobeStream) api.PlaybackTrack {
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
