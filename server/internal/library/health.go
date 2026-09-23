package library

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/config"
)

type HealthIssue struct {
	Code    string   `json:"code"`
	Folder  string   `json:"folder"`
	Shows   []string `json:"shows"`
	Message string   `json:"message"`
}

type HealthReport struct {
	ShowCount          int           `json:"showCount"`
	BrowsingGroupCount int           `json:"browsingGroupCount"`
	RepresentedFolders int           `json:"representedFolders"`
	UnmappedItems      int           `json:"unmappedItems"`
	Issues             []HealthIssue `json:"issues"`
}

func healthName(s string) string {
	s = reYearParen.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// Health compares catalog identities with configured show-folder boundaries.
// It does not traverse the NAS, infer correctness, or change media/metadata.
func (s *Store) Health(libraries []config.Library) HealthReport {
	report := HealthReport{Issues: []HealthIssue{}}
	roots := map[string]string{}
	for _, lib := range libraries {
		roots[lib.ID] = lib.Path
	}
	folders := map[string]map[string]bool{}
	labels := map[string]string{}
	shows := map[string]bool{}
	for _, item := range s.InternalItems() {
		if item.Kind != api.KindTVShow {
			continue
		}
		name := ""
		if item.ShowTitle != nil {
			name = strings.TrimSpace(*item.ShowTitle)
		}
		if name != "" {
			shows[name] = true
		}
		root := ""
		if item.LibraryID != nil {
			root = roots[*item.LibraryID]
		}
		if root == "" {
			report.UnmappedItems++
			continue
		}
		relative, err := filepath.Rel(root, item.FilePath)
		parts := strings.Split(relative, string(filepath.Separator))
		if err != nil || len(parts) < 2 || parts[0] == ".." || parts[0] == "." {
			report.UnmappedItems++
			continue
		}
		key := root + "\x00" + parts[0]
		labels[key] = parts[0]
		if folders[key] == nil {
			folders[key] = map[string]bool{}
		}
		folders[key][name] = true
	}
	report.ShowCount, report.RepresentedFolders = len(shows), len(folders)
	groupIDs := map[string]bool{}
	for _, item := range s.GroupedItems(libraries) {
		if item.Kind != api.KindTVShow {
			continue
		}
		if item.ShowGroupID != nil {
			groupIDs[*item.ShowGroupID] = true
		} else if item.ShowTitle != nil {
			groupIDs[*item.ShowTitle] = true
		}
	}
	report.BrowsingGroupCount = len(groupIDs)
	keys := make([]string, 0, len(folders))
	for key := range folders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		names := []string{}
		mismatch := false
		for name := range folders[key] {
			names = append(names, name)
			if healthName(name) != healthName(labels[key]) {
				mismatch = true
			}
		}
		sort.Strings(names)
		code, message := "", ""
		if len(names) > 1 {
			code, message = "split_show_folder", "One source folder produces multiple show identities. Check parsing or mixed content."
		} else if mismatch {
			code, message = "show_folder_mismatch", "Show identity differs from its source folder. This may be an alias or misplaced content."
		}
		if code != "" {
			report.Issues = append(report.Issues, HealthIssue{code, labels[key], names, message})
		}
	}
	if report.ShowCount > report.RepresentedFolders && report.UnmappedItems == 0 {
		report.Issues = append(report.Issues, HealthIssue{"excess_show_identities", "", []string{}, "More show identities than represented source folders. Review split-folder warnings."})
	}
	return report
}
