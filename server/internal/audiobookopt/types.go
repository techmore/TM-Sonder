// Package audiobookopt runs staged, validated audiobook conversions. Originals
// remain in their scanned library; outputs are copied to a hidden review tree.
package audiobookopt

import "time"

type Status string

const (
	StatusQueued      Status = "queued"
	StatusCopying     Status = "copying-source"
	StatusEncoding    Status = "encoding"
	StatusValidating  Status = "validating"
	StatusReturning   Status = "copying-result"
	StatusStaged      Status = "staged-for-review"
	StatusAccepted    Status = "accepted"
	StatusFailed      Status = "failed"
	StatusCanceled    Status = "canceled"
	StatusInterrupted Status = "interrupted"
)

type Validation struct {
	Container         string  `json:"container"`
	AudioCodec        string  `json:"audioCodec"`
	Channels          int     `json:"channels"`
	DurationSeconds   float64 `json:"durationSeconds"`
	ChapterCount      int     `json:"chapterCount"`
	CoverPresent      bool    `json:"coverPresent"`
	FullDecode        bool    `json:"fullDecode"`
	MetadataPreserved bool    `json:"metadataPreserved"`
	ChaptersPreserved bool    `json:"chaptersPreserved"`
}

type Receipt struct {
	CreatedAt          time.Time  `json:"createdAt"`
	SourceBytes        int64      `json:"sourceBytes"`
	OutputBytes        int64      `json:"outputBytes"`
	SavedBytes         int64      `json:"savedBytes"`
	SavedPercent       float64    `json:"savedPercent"`
	SourceSHA256       string     `json:"sourceSha256"`
	OutputSHA256       string     `json:"outputSha256"`
	BitrateKbps        int        `json:"bitrateKbps"`
	CoverSource        string     `json:"coverSource"`
	Validation         Validation `json:"validation"`
	PlaybackReview     string     `json:"playbackReview"`
	StagedRelativePath string     `json:"stagedRelativePath"`
}

// Job is the public API view. Source paths and internal workspace paths are
// deliberately absent.
type Job struct {
	ID                    string    `json:"id"`
	ItemID                string    `json:"itemID"`
	Title                 string    `json:"title"`
	Status                Status    `json:"status"`
	Phase                 string    `json:"phase"`
	Progress              float64   `json:"progress"`
	CurrentBytes          int64     `json:"currentBytes"`
	SourceBytes           int64     `json:"sourceBytes"`
	BitrateKbps           int       `json:"bitrateKbps"`
	Attempt               int       `json:"attempt"`
	CatalogCoverAvailable bool      `json:"catalogCoverAvailable"`
	EmbeddedCoverPresent  bool      `json:"embeddedCoverPresent"`
	PosterURL             string    `json:"posterURL,omitempty"`
	CreatedAt             time.Time `json:"createdAt"`
	StartedAt             time.Time `json:"startedAt,omitempty"`
	UpdatedAt             time.Time `json:"updatedAt"`
	Error                 string    `json:"error,omitempty"`
	Receipt               *Receipt  `json:"receipt,omitempty"`
}

type Snapshot struct {
	Paused      bool      `json:"paused"`
	ActiveJobID string    `json:"activeJobID,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Jobs        []Job     `json:"jobs"`
}

type diskJob struct {
	Job
	SourcePath       string `json:"sourcePath"`
	LibraryRoot      string `json:"libraryRoot"`
	RelativePath     string `json:"relativePath"`
	SourceMtimeNS    int64  `json:"sourceMtimeNS"`
	UseCatalogCover  bool   `json:"useCatalogCover"`
	CatalogCoverPath string `json:"catalogCoverPath,omitempty"`
}

type diskState struct {
	Paused    bool      `json:"paused"`
	UpdatedAt time.Time `json:"updatedAt"`
	Jobs      []diskJob `json:"jobs"`
}
