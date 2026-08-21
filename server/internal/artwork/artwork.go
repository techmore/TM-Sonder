// Package artwork generates poster thumbnails for videos via ffmpeg,
// mirroring what Plex does for media without embedded/local artwork.
package artwork

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Generator produces JPEG posters for media files.
type Generator struct {
	FFmpegPath string
	OutDir     string // e.g. DataDir/artwork
}

// PathFor is the canonical output path for one item ID.
func (g *Generator) PathFor(itemID string) string {
	return filepath.Join(g.OutDir, itemID+".jpg")
}

// pickTimestamp chooses a representative frame position: 10% into the
// duration clamped to [2s, 180s], falling back to 3s when duration unknown.
func pickTimestamp(durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 3
	}
	at := durationSeconds * 0.10
	if at < 2 {
		at = 2
	}
	if at > 180 {
		at = 180
	}
	if at > durationSeconds {
		return 3
	}
	return at
}

// Args builds the ffmpeg command line for one thumbnail.
func Args(ffmpegPath, srcPath, outPath string, atSeconds float64) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.2f", atSeconds),
		"-i", srcPath,
		"-frames:v", "1",
		"-vf", "scale=480:-2",
		"-q:v", "4",
		"-y", outPath,
	}
}

// Generate extracts one poster frame. It returns the output path on success.
// Audio-only or undecodable inputs yield an error the caller can ignore.
func (g *Generator) Generate(ctx context.Context, srcPath, itemID string, durationSeconds float64) (string, error) {
	out := g.PathFor(itemID)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil // already generated
	}
	if err := os.MkdirAll(g.OutDir, 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp.jpg"
	cmd := exec.CommandContext(ctx, g.FFmpegPath, Args(g.FFmpegPath, srcPath, tmp, pickTimestamp(durationSeconds))...)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("artwork: thumbnail %s: %v: %.200s", filepath.Base(srcPath), err, outBytes)
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return out, nil
}
