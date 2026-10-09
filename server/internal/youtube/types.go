// Package youtube runs persistent channel subscriptions through yt-dlp.
package youtube

import (
	"context"
	"sync"
	"time"
)

const GB int64 = 1000 * 1000 * 1000
const jobBudget int64 = 24 * GB

type Settings struct {
	Paused    bool   `json:"paused"`
	MinFreeGB int64  `json:"minFreeGB"`
	MaxHeight int    `json:"maxHeight"`
	Profile   string `json:"profile"`
}
type Channel struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	Name          string    `json:"name"`
	YouTubeID     string    `json:"youtubeID,omitempty"`
	IntervalHours int       `json:"intervalHours"`
	Backfill      int       `json:"backfill"`
	Paused        bool      `json:"paused"`
	LastCheck     time.Time `json:"lastCheck"`
	NextCheck     time.Time `json:"nextCheck"`
	Error         string    `json:"error,omitempty"`
	Initialized   bool      `json:"initialized"`
}
type Job struct {
	ID            string    `json:"id"`
	ChannelID     string    `json:"channelID"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	Detail        string    `json:"detail,omitempty"`
	Path          string    `json:"path,omitempty"`
	Bytes         int64     `json:"bytes"`
	OriginalBytes int64     `json:"originalBytes"`
	SavedBytes    int64     `json:"savedBytes"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
type State struct {
	Settings Settings  `json:"settings"`
	Channels []Channel `json:"channels"`
	Jobs     []Job     `json:"jobs"`
}
type Snapshot struct {
	State
	Counts          map[string]int  `json:"counts"`
	SavedBytes      int64           `json:"savedBytes"`
	StoredBytes     int64           `json:"storedBytes"`
	StoragePath     string          `json:"storagePath"`
	FreeBytes       int64           `json:"freeBytes"`
	Ready           bool            `json:"ready"`
	Blocked         string          `json:"blocked,omitempty"`
	Active          string          `json:"active,omitempty"`
	Dependencies    map[string]bool `json:"dependencies"`
	Checking        string          `json:"checking,omitempty"`
	KnownArchiveIDs int             `json:"knownArchiveIDs"`
}
type Manager struct {
	mu                              sync.Mutex
	state                           State
	path, root, yt, ffmpeg, ffprobe string
	known                           map[string]bool
	ctx                             context.Context
	cancel                          context.CancelFunc
	activeCancel                    context.CancelFunc
	checkCancel                     context.CancelFunc
	checking                        string
	checkWake                       chan struct{}
	checkDone                       chan struct{}
	active, activeChannel, blocked  string
	wake                            chan struct{}
	done                            chan struct{}
	onComplete                      func()
}
