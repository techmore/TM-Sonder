package library

import (
	"path/filepath"
	"sort"
	"strings"

	"tm-sonder/server/internal/api"
)

// Show- and season-level TV artwork.
//
// An episode's poster is the most specific art found for it: art beside the
// episode file, then the season folder, then the show folder. That single
// URL cannot tell a show card from a season card apart, so a season poster
// (or an extras-folder poster, which parses into a regular season number)
// could end up as the whole show's cover. These helpers classify where each
// episode's art actually lives and stamp every member of a show group with
// the group's show-folder art and its own season's art. All values are
// derived per request from stored paths; nothing here changes the catalog,
// so no parser bump is needed.

// tvArtLevel classifies where an item's poster art lives relative to the
// show and season folders.
type tvArtLevel int

const (
	tvArtNone tvArtLevel = iota
	tvArtShow
	tvArtSeason
	tvArtOther // episode-specific or outside the show/season folders
)

// tvArtPair carries one grouped wire item plus the stored fields needed to
// classify its art. internal and wire describe the same catalog item.
type tvArtPair struct {
	internal *Item
	wire     *api.MediaItem
}

// tvSeasonKey matches the client's season bucketing (seasonNumber ?? 0).
func tvSeasonKey(season *int) int {
	if season == nil {
		return 0
	}
	return *season
}

// tvRelativeDirs resolves library-relative slash-separated directories for
// the media file and its poster art. showFolder is the group's show folder
// name (first path component). It returns the season directory containing
// the media and the directory containing the art.
func tvRelativeDirs(item *Item, root browsingRoot, showFolder string) (seasonDir, artDir string, ok bool) {
	for _, base := range []string{root.path, root.realPath} {
		if base == "" {
			continue
		}
		var relMedia string
		if item.SourceRelativePath != "" {
			relMedia = filepath.Clean(filepath.FromSlash(item.SourceRelativePath))
			if filepath.IsAbs(relMedia) || relMedia == "." || relMedia == ".." ||
				strings.HasPrefix(relMedia, ".."+string(filepath.Separator)) {
				continue
			}
		} else if item.FilePath != "" {
			candidate, err := filepath.Rel(base, item.FilePath)
			if err != nil || candidate == "." || candidate == ".." ||
				strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
				continue
			}
			relMedia = candidate
		} else {
			continue
		}
		relMediaSlash := filepath.ToSlash(relMedia)
		parts := strings.Split(relMediaSlash, "/")
		if len(parts) == 0 || parts[0] != showFolder {
			continue
		}
		if item.PosterPath == "" {
			return "", "", false
		}
		relArt, err := filepath.Rel(base, item.PosterPath)
		if err != nil {
			continue
		}
		relArtSlash := filepath.ToSlash(relArt)
		if relArtSlash == "." || relArtSlash == ".." || strings.HasPrefix(relArtSlash, "../") {
			continue
		}
		return dirOf(relMediaSlash), dirOf(relArtSlash), true
	}
	return "", "", false
}

func dirOf(slashPath string) string {
	if i := strings.LastIndex(slashPath, "/"); i >= 0 {
		return slashPath[:i]
	}
	return "."
}

// tvArtBase returns the media file's base name without extension, used to
// detect episode-specific art (e.g. "S01E01-poster.jpg" beside S01E01.mkv).
func tvArtBase(item *Item) string {
	name := filepath.Base(item.FilePath)
	if name == "" || name == "." {
		if item.SourceRelativePath != "" {
			name = filepath.Base(filepath.FromSlash(item.SourceRelativePath))
		}
	}
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

// tvClassifyArt reports which folder level an item's poster art comes from.
// showFolder is the group's show folder name; seasonDir and artDir are
// library-relative slash directories from tvRelativeDirs.
func tvClassifyArt(item *Item, showFolder, seasonDir, artDir string) tvArtLevel {
	switch {
	case artDir == showFolder:
		return tvArtShow
	case artDir == seasonDir && seasonDir != showFolder:
		// Art beside the episode file (base.jpg / base-poster.jpg) lives in
		// the season folder but belongs to one episode, not the season.
		artBase := strings.TrimSuffix(filepath.Base(item.PosterPath), filepath.Ext(item.PosterPath))
		epBase := tvArtBase(item)
		if epBase != "" && (strings.EqualFold(artBase, epBase) || strings.EqualFold(artBase, epBase+"-poster")) {
			return tvArtOther
		}
		return tvArtSeason
	default:
		return tvArtOther
	}
}

// tvArtOrder sorts group members deterministically so representative picks
// are stable across requests.
func tvArtOrder(a, b *tvArtPair) bool {
	sa, sb := tvSeasonKey(a.internal.SeasonNumber), tvSeasonKey(b.internal.SeasonNumber)
	if sa != sb {
		return sa < sb
	}
	ea, eb := 1<<30, 1<<30
	if a.internal.EpisodeNumber != nil {
		ea = *a.internal.EpisodeNumber
	}
	if b.internal.EpisodeNumber != nil {
		eb = *b.internal.EpisodeNumber
	}
	if ea != eb {
		return ea < eb
	}
	if a.internal.Title != b.internal.Title {
		return a.internal.Title < b.internal.Title
	}
	return a.internal.ID < b.internal.ID
}

// applyTVArtPosters stamps ShowPosterURL and SeasonPosterURL on grouped TV
// wire items. pairs must hold one entry per wire result in the same order.
func applyTVArtPosters(pairs []tvArtPair, roots map[string]browsingRoot) {
	byGroup := make(map[string][]int)
	for idx := range pairs {
		if pairs[idx].wire.ShowGroupID == nil {
			continue
		}
		g := *pairs[idx].wire.ShowGroupID
		byGroup[g] = append(byGroup[g], idx)
	}
	for _, idxs := range byGroup {
		sort.Slice(idxs, func(a, b int) bool { return tvArtOrder(&pairs[idxs[a]], &pairs[idxs[b]]) })
		var showRep *tvArtPair
		type seasonRep struct {
			rep   *tvArtPair
			depth int
		}
		seasonReps := make(map[int]seasonRep)
		for _, idx := range idxs {
			p := &pairs[idx]
			if p.wire.PosterURL == nil || p.internal.PosterPath == "" {
				continue
			}
			root, ok := roots[derefLibraryID(p.internal.LibraryID)]
			if !ok || p.wire.ShowGroupTitle == nil {
				continue
			}
			// ShowGroupTitle is the source-folder display name; the folder
			// itself is its first path component only when the group came
			// from showFolderTitle. Re-derive it the same way instead.
			showFolder, ok := showFolderForArt(p.internal, root)
			if !ok {
				continue
			}
			seasonDir, artDir, ok := tvRelativeDirs(p.internal, root, showFolder)
			if !ok {
				continue
			}
			level := tvClassifyArt(p.internal, showFolder, seasonDir, artDir)
			if level == tvArtShow && showRep == nil {
				showRep = p
				continue
			}
			if level == tvArtSeason {
				key := tvSeasonKey(p.internal.SeasonNumber)
				depth := strings.Count(seasonDir, "/")
				// Only a season folder directly inside the show folder may
				// represent the season. Deeper art (Extras/Season 04 Extras)
				// belongs to a nested folder, not the season, and must lose
				// to the show fallback rather than pose as Season 4.
				if depth != strings.Count(showFolder, "/")+1 {
					continue
				}
				if cur, exists := seasonReps[key]; !exists || depth < cur.depth {
					seasonReps[key] = seasonRep{rep: p, depth: depth}
				}
			}
		}
		// A show whose folder carries no art (every season has its own)
		// falls back to the lowest season's art rather than a placeholder.
		showURL := ""
		if showRep != nil {
			showURL = *showRep.wire.PosterURL
		} else {
			best := 1 << 30
			for key, rep := range seasonReps {
				if rep.rep != nil && rep.rep.wire.PosterURL != nil && key < best {
					best = key
					showURL = *rep.rep.wire.PosterURL
				}
			}
		}
		for _, idx := range idxs {
			p := &pairs[idx]
			key := tvSeasonKey(p.internal.SeasonNumber)
			if rep, ok := seasonReps[key]; ok && rep.rep != nil && rep.rep.wire.PosterURL != nil {
				u := *rep.rep.wire.PosterURL
				p.wire.SeasonPosterURL = &u
			} else if showURL != "" {
				u := showURL
				p.wire.SeasonPosterURL = &u
			} else if p.wire.PosterURL != nil {
				u := *p.wire.PosterURL
				p.wire.SeasonPosterURL = &u
			}
			if showURL != "" {
				u := showURL
				p.wire.ShowPosterURL = &u
			} else if p.wire.PosterURL != nil {
				u := *p.wire.PosterURL
				p.wire.ShowPosterURL = &u
			}
		}
	}
}

// showFolderForArt re-derives the source folder name for art comparison. It
// mirrors showFolderTitle's first-component rule without its minimum-depth
// requirement, because flat TV files keep their art beside the file.
func showFolderForArt(item *Item, root browsingRoot) (string, bool) {
	relative := ""
	if item.SourceRelativePath != "" {
		relative = filepath.Clean(filepath.FromSlash(item.SourceRelativePath))
		if filepath.IsAbs(relative) || relative == "." || relative == ".." ||
			strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			relative = ""
		}
	}
	if relative == "" && item.FilePath != "" {
		for _, base := range []string{root.path, root.realPath} {
			if base == "" {
				continue
			}
			candidate, err := filepath.Rel(base, item.FilePath)
			if err == nil && candidate != "." && candidate != ".." &&
				!strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
				relative = candidate
				break
			}
		}
	}
	if relative == "" {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." || parts[0] == ".." {
		return "", false
	}
	return parts[0], true
}

func derefLibraryID(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
