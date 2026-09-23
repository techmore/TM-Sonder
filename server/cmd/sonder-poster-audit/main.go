// sonder-poster-audit inspects image headers without modifying artwork.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sort"

	"tm-sonder/server/internal/library"
)

type row struct {
	Path   string   `json:"path"`
	Titles []string `json:"titles"`
	IDs    []string `json:"ids"`
	Width  int      `json:"width"`
	Height int      `json:"height"`
	Reason string   `json:"reason"`
}

func main() {
	path := flag.String("snapshot", "", "catalog snapshot")
	kind := flag.String("kind", "tvShow", "movie, tvShow, audiobook or ebook")
	flag.Parse()
	data, err := os.ReadFile(*path)
	if err != nil {
		panic(err)
	}
	var snapshot library.Snapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		panic(err)
	}
	groups := map[string]*row{}
	for _, item := range snapshot.Items {
		if string(item.Kind) != *kind || item.PosterPath == "" {
			continue
		}
		r := groups[item.PosterPath]
		if r == nil {
			r = &row{Path: item.PosterPath, Titles: []string{}, IDs: []string{}}
			groups[item.PosterPath] = r
		}
		title := item.Title
		if item.ShowTitle != nil {
			title = *item.ShowTitle
		}
		found := false
		for _, t := range r.Titles {
			if t == title {
				found = true
			}
		}
		if !found {
			r.Titles = append(r.Titles, title)
		}
		r.IDs = append(r.IDs, item.ID)
	}
	rows := []*row{}
	for _, r := range groups {
		file, err := os.Open(r.Path)
		if err != nil {
			r.Reason = "unreadable"
		} else {
			cfg, _, err := image.DecodeConfig(file)
			file.Close()
			if err != nil {
				r.Reason = "unsupported or corrupt"
			} else {
				r.Width, r.Height = cfg.Width, cfg.Height
				ratio := float64(cfg.Width) / float64(cfg.Height)
				maxRatio, minHeight := 0.85, 300
				if *kind == "audiobook" {
					maxRatio, minHeight = 1.1, 200
				}
				if ratio > maxRatio {
					r.Reason = "not portrait"
				} else if ratio < 0.5 {
					r.Reason = "too narrow"
				} else if cfg.Width < 200 || cfg.Height < minHeight {
					r.Reason = "low resolution"
				}
			}
		}
		sort.Strings(r.Titles)
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
