package api

import (
	"fmt"
	"time"
)

type MediaKind string

const (
	KindAll         MediaKind = "all"
	KindMovie       MediaKind = "movie"
	KindTVShow      MediaKind = "tvShow"
	KindDocumentary MediaKind = "documentary"
	KindAudiobook   MediaKind = "audiobook"
	KindEbook       MediaKind = "ebook"
)

type MediaFormat string

const (
	FormatMP4     MediaFormat = "mp4"
	FormatMOV     MediaFormat = "mov"
	FormatM4V     MediaFormat = "m4v"
	FormatMKV     MediaFormat = "mkv"
	FormatWEBM    MediaFormat = "webm"
	FormatAVI     MediaFormat = "avi"
	FormatMPG     MediaFormat = "mpg"
	FormatMPEG    MediaFormat = "mpeg"
	FormatTS      MediaFormat = "ts"
	FormatM2TS    MediaFormat = "m2ts"
	FormatEPUB    MediaFormat = "epub"
	FormatPDF     MediaFormat = "pdf"
	FormatM4B     MediaFormat = "m4b"
	FormatMP3     MediaFormat = "mp3"
	FormatM4A     MediaFormat = "m4a"
	FormatUnknown MediaFormat = "unknown"
)

var formatByExt = map[string]MediaFormat{
	"mp4": FormatMP4, "mov": FormatMOV, "m4v": FormatM4V, "mkv": FormatMKV,
	"webm": FormatWEBM, "avi": FormatAVI, "mpg": FormatMPG, "mpeg": FormatMPEG,
	"ts": FormatTS, "m2ts": FormatM2TS, "epub": FormatEPUB, "pdf": FormatPDF,
	"m4b": FormatM4B, "mp3": FormatMP3, "m4a": FormatM4A,
}

func FormatForExtension(ext string) MediaFormat {
	if f, ok := formatByExt[ext]; ok {
		return f
	}
	return FormatUnknown
}

var contentTypes = map[MediaFormat]string{
	FormatMP4: "video/mp4", FormatM4V: "video/mp4", FormatMOV: "video/quicktime",
	FormatMKV: "video/x-matroska", FormatWEBM: "video/webm", FormatAVI: "video/x-msvideo",
	FormatMPG: "video/mpeg", FormatMPEG: "video/mpeg", FormatTS: "video/mp2t",
	FormatM2TS: "video/mp2t", FormatEPUB: "application/epub+zip", FormatPDF: "application/pdf",
	FormatM4B: "audio/mp4", FormatMP3: "audio/mpeg", FormatM4A: "audio/mp4",
	FormatUnknown: "application/octet-stream",
}

func (f MediaFormat) ContentType() string { return contentTypes[f] }

var avPlayerPlayable = map[MediaFormat]bool{
	FormatMP4: true, FormatMOV: true, FormatM4V: true, FormatM4B: true, FormatMP3: true, FormatM4A: true,
}

func (f MediaFormat) IsPlayableInAVPlayer() bool { return avPlayerPlayable[f] }

type TrackKind string

const (
	TrackEmbedded TrackKind = "embedded"
	TrackSidecar  TrackKind = "sidecar"
)

type PlaybackTrack struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	LanguageCode *string   `json:"languageCode"`
	Kind         TrackKind `json:"kind"`
	URL          *string   `json:"url"`
}

type MediaItem struct {
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Subtitle        string      `json:"subtitle"`
	Kind            MediaKind   `json:"kind"`
	Studio          string      `json:"studio"`
	Series          string      `json:"series,omitempty"`
	SeriesNumber    float64     `json:"seriesNumber,omitempty"`
	Year            int         `json:"year"`
	DurationSeconds float64     `json:"durationSeconds"`
	Format          MediaFormat `json:"format"`
	LibraryID       *string     `json:"libraryID"`
	Tags            []string    `json:"tags"`
	// Genres is the curated genre list (distinct from Tags, which also holds
	// free-form provider keywords and people names).
	Genres           []string `json:"genres,omitempty"`
	Summary          string   `json:"summary"`
	ProgressSeconds  float64  `json:"progressSeconds"`
	ShowTitle        *string  `json:"showTitle"`
	ShowGroupID      *string  `json:"showGroupID,omitempty"`
	ShowGroupTitle   *string  `json:"showGroupTitle,omitempty"`
	SeasonNumber     *int     `json:"seasonNumber"`
	EpisodeNumber    *int     `json:"episodeNumber"`
	MetadataIDSource *string  `json:"metadataIDSource"`
	MetadataID       *string  `json:"metadataID"`
	Edition          *string  `json:"edition"`
	SplitPart        *string  `json:"splitPart"`
	// Author/Narrator are populated for audiobooks and ebooks (enrichment
	// and filename parsing); they are distinct from Tags, which holds
	// genres plus provider keywords.
	Author   *string `json:"author,omitempty"`
	Narrator *string `json:"narrator,omitempty"`
	// BookGroupID/BookGroupTitle identify the *book* an audiobook file belongs
	// to, which is the folder it lives in. A book delivered as many files (a
	// multi-part recording) is one book with N parts, not N books, and clients
	// that list per file render a 148-file book as 148 rows. BookPartIndex and
	// BookPartCount give the position within the book so a client can present
	// one entry and still play the parts in order.
	BookGroupID    *string `json:"bookGroupID,omitempty"`
	BookGroupTitle *string `json:"bookGroupTitle,omitempty"`
	BookPartIndex  int     `json:"bookPartIndex,omitempty"`
	BookPartCount  int     `json:"bookPartCount,omitempty"`
	// SortTitle is the key an A-Z listing must order by, and is deliberately not
	// the Title. Title is whatever the library file is called, which may start
	// with a publication year ("2011 - The Martian") or embed a narrator credit
	// ("Ubik (Daniels)"). Ordering by Title put such a shelf in year order and
	// split one book across two places in the alphabet.
	SortTitle string `json:"sortTitle,omitempty"`
	// SeriesPosition and PublicationYear carry the leading number and the year
	// that SortTitle removed, so ordering on the cleaner key loses nothing.
	SeriesPosition         string          `json:"seriesPosition,omitempty"`
	SeriesName             string          `json:"seriesName,omitempty"`
	PublicationYear        int             `json:"publicationYear,omitempty"`
	CoverAvailable         bool            `json:"coverAvailable"`
	CoverEmbedded          bool            `json:"coverEmbedded"`
	IsPlaceholder          bool            `json:"isPlaceholder"`
	PosterURL              *string         `json:"posterURL"`
	BackdropURL            *string         `json:"backdropURL"`
	EmbeddedAudioTracks    []PlaybackTrack `json:"embeddedAudioTracks"`
	EmbeddedSubtitleTracks []PlaybackTrack `json:"embeddedSubtitleTracks"`
	TrackProbeUpdatedAt    *time.Time      `json:"trackProbeUpdatedAt"`
	ProbedWidth            *int            `json:"probedWidth"`
	ProbedHeight           *int            `json:"probedHeight"`
	ProbedCodec            *string         `json:"probedCodec"`
	ProbedAudioCodecs      []string        `json:"probedAudioCodecs,omitempty"`
	ProbedBitrate          *int            `json:"probedBitrate"`
	BookValidation         *string         `json:"bookValidation"`
	CoverSource            *string         `json:"coverSource"`

	FilePath string `json:"-"`
}

func (m *MediaItem) EpisodeCode() string {
	if m.SeasonNumber == nil || m.EpisodeNumber == nil {
		return "Not episodic"
	}
	return fmt.Sprintf("S%02dE%02d", *m.SeasonNumber, *m.EpisodeNumber)
}

type ServerSettings struct {
	IsEnabled          bool   `json:"isEnabled"`
	AllowLAN           bool   `json:"allowLAN"`
	Port               int    `json:"port"`
	WebPort            int    `json:"webPort,omitempty"`
	APIPort            int    `json:"apiPort,omitempty"`
	Version            string `json:"version"`
	ThemePreset        string `json:"themePreset"`
	LibraryLayout      string `json:"libraryLayout"`
	HideEmptyLibraries bool   `json:"hideEmptyLibraries"`
	RequiresPairing    bool   `json:"requiresPairing"`
}

type MediaDirectory struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Kind                 string     `json:"kind"`
	LibraryID            string     `json:"libraryID"`
	LastIndexedCount     int        `json:"lastIndexedCount"`
	LastScannedFileCount int        `json:"lastScannedFileCount"`
	LastScannedAt        *time.Time `json:"lastScannedAt"`
}

type ActivityEvent struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Icon   string    `json:"icon"`
	Date   time.Time `json:"date"`
}

type ThemeSnapshot struct {
	Preset     string `json:"preset"`
	Background string `json:"background"`
	Sidebar    string `json:"sidebar"`
	Surface    string `json:"surface"`
	Border     string `json:"border"`
	Accent     string `json:"accent"`
	Text       string `json:"text"`
}

type ProgressRecord struct {
	ID               string    `json:"id"`
	ItemID           string    `json:"itemID"`
	Seconds          float64   `json:"seconds"`
	Duration         float64   `json:"duration"`
	UpdatedAt        time.Time `json:"updatedAt"`
	AudioTrackID     *string   `json:"audioTrackID,omitempty"`
	SubtitleTrackID  *string   `json:"subtitleTrackID,omitempty"`
	SubtitlesEnabled *bool     `json:"subtitlesEnabled,omitempty"`
}

type LibraryResponse struct {
	Items            []MediaItem      `json:"items"`
	Progress         []ProgressRecord `json:"progress"`
	MediaDirectories []MediaDirectory `json:"mediaDirectories"`
	Activity         []ActivityEvent  `json:"activity"`
	ServerSettings   *ServerSettings  `json:"serverSettings"`
	Theme            *ThemeSnapshot   `json:"theme"`
}

type PlaybackStateUpdate struct {
	Seconds          float64 `json:"seconds"`
	Duration         float64 `json:"duration"`
	AudioTrackID     *string `json:"audioTrackID,omitempty"`
	SubtitleTrackID  *string `json:"subtitleTrackID,omitempty"`
	SubtitlesEnabled *bool   `json:"subtitlesEnabled,omitempty"`
}

// AudiobookChapter mirrors SonderAudiobookChapter: endSeconds is null for
// open-ended chapters (e.g. the last one before duration is known).
type AudiobookChapter struct {
	Index        int      `json:"index"`
	Title        string   `json:"title"`
	StartSeconds float64  `json:"startSeconds"`
	EndSeconds   *float64 `json:"endSeconds"`
}

type PlaybackSession struct {
	ItemID           *string         `json:"itemID,omitempty"`
	StreamURL        string          `json:"streamURL"`
	Seconds          float64         `json:"seconds"`
	Duration         float64         `json:"duration"`
	Percent          float64         `json:"percent"`
	UpdatedAt        *time.Time      `json:"updatedAt,omitempty"`
	AudioTrackID     *string         `json:"audioTrackID,omitempty"`
	SubtitleTrackID  *string         `json:"subtitleTrackID,omitempty"`
	SubtitlesEnabled *bool           `json:"subtitlesEnabled,omitempty"`
	AudioTracks      []PlaybackTrack `json:"audioTracks"`
	SubtitleTracks   []PlaybackTrack `json:"subtitleTracks"`
}

type HealthResponse struct {
	Status          string `json:"status"`
	Name            string `json:"name"`
	App             string `json:"app"`
	ID              string `json:"id"`
	Service         string `json:"service"`
	Library         string `json:"library"`
	AllowLAN        bool   `json:"allowLAN"`
	RequiresPairing bool   `json:"requiresPairing"`
}

type DiscoveryCapabilities struct {
	Books          bool `json:"books"`
	Ebooks         bool `json:"ebooks"`
	Audiobooks     bool `json:"audiobooks"`
	Themes         bool `json:"themes"`
	ThemeSync      bool `json:"themeSync"`
	ProgressSync   bool `json:"progressSync"`
	MediaStreaming bool `json:"mediaStreaming"`
	VideoStreaming bool `json:"videoStreaming"`
	Artwork        bool `json:"artwork"`
	LibrarySync    bool `json:"librarySync"`
	RemoteCatalog  bool `json:"remoteCatalog"`
}

type DiscoveryEndpoints struct {
	Health               string  `json:"health"`
	Library              string  `json:"library"`
	Audiobooks           *string `json:"audiobooks"`
	AudiobookBrowser     *string `json:"audiobookBrowser"`
	Discovery            string  `json:"discovery"`
	Progress             string  `json:"progress"`
	Playback             string  `json:"playback"`
	PlaybackTrackRefresh *string `json:"playbackTrackRefresh"`
	RefreshTracks        *string `json:"refreshTracks"`
	Stream               string  `json:"stream"`
	Subtitles            *string `json:"subtitles"`
	Poster               *string `json:"poster"`
	Backdrop             *string `json:"backdrop"`
	MovieMetadata        *string `json:"movieMetadata,omitempty"`
	NetworkInterfaces    *string `json:"networkInterfaces,omitempty"`
	NetworkStatus        *string `json:"networkStatus,omitempty"`
	NetworkRebind        *string `json:"networkRebind,omitempty"`
	NetworkExposure      *string `json:"networkExposure,omitempty"`
	CacheStatus          *string `json:"cacheStatus,omitempty"`
}

type DiscoveryResponse struct {
	App              string                `json:"app"`
	Name             string                `json:"name"`
	ServerID         string                `json:"serverID"`
	Version          string                `json:"version"`
	Build            string                `json:"build"`
	IsEnabled        bool                  `json:"isEnabled"`
	AllowLAN         bool                  `json:"allowLAN"`
	RequiresPairing  bool                  `json:"requiresPairing"`
	Port             int                   `json:"port"`
	LocalURL         string                `json:"localURL"`
	LanURL           *string               `json:"lanURL"`
	DiscoveryMethods []string              `json:"discoveryMethods"`
	TailscaleHint    string                `json:"tailscaleHint"`
	Capabilities     DiscoveryCapabilities `json:"capabilities"`
	Endpoints        DiscoveryEndpoints    `json:"endpoints"`
	Theme            ThemeSnapshot         `json:"theme"`
}
