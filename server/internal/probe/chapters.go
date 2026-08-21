package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"

	"tm-sonder/server/internal/api"
)

func unmarshalJSON(data []byte, v any) error { return json.Unmarshal(data, v) }

type ffprobeChapters struct {
	Chapters []struct {
		ID       int     `json:"id"`
		Start    float64 `json:"start"`
		End      float64 `json:"end"`
		StartStr string  `json:"start_time"`
		EndStr   string  `json:"end_time"`
		Tags     struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"chapters"`
}

// Chapters extracts audiobook chapter markers via ffprobe -show_chapters.
// Titles fall back to "Chapter N"; endSeconds is nil when missing or zero.
func Chapters(ctx context.Context, ffprobePath, mediaPath string) ([]api.AudiobookChapter, error) {
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet", "-print_format", "json", "-show_chapters", mediaPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("probe: ffprobe chapters %s: %w", mediaPath, err)
	}
	return ParseChapters(out), nil
}

// ParseChapters maps raw ffprobe chapter JSON to wire DTOs.
func ParseChapters(data []byte) []api.AudiobookChapter {
	var raw ffprobeChapters
	if err := unmarshalJSON(data, &raw); err != nil {
		return nil
	}
	out := make([]api.AudiobookChapter, 0, len(raw.Chapters))
	for i, c := range raw.Chapters {
		ch := api.AudiobookChapter{
			Index:        i,
			Title:        c.Tags.Title,
			StartSeconds: firstNonZeroF(atofOr(c.StartStr, 0), c.Start),
		}
		if ch.Title == "" {
			ch.Title = "Chapter " + strconv.Itoa(i+1)
		}
		if end := firstNonZeroF(atofOr(c.EndStr, 0), c.End); end > 0 {
			ch.EndSeconds = &end
		}
		out = append(out, ch)
	}
	return out
}

func atofOr(s string, fallback float64) float64 {
	if v, ok := atof(s); ok {
		return v
	}
	return fallback
}

func firstNonZeroF(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}
