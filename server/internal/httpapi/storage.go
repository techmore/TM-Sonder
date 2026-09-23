package httpapi

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

type storageNode struct {
	Name      string        `json:"name"`
	SizeBytes int64         `json:"sizeBytes"`
	ItemCount int           `json:"itemCount"`
	Folders   int           `json:"folders"`
	Children  []storageNode `json:"children,omitempty"`
	Files     []storageFile `json:"files,omitempty"`
}

type storageFile struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Kind      string `json:"kind"`
}

type storageLibrary struct {
	ID   string      `json:"id"`
	Name string      `json:"name"`
	Kind string      `json:"kind"`
	Root storageNode `json:"root"`
}

type storagePayload struct {
	TotalBytes  int64            `json:"totalBytes"`
	ItemCount   int              `json:"itemCount"`
	FolderCount int              `json:"folderCount"`
	ArtistCount int              `json:"artistCount"`
	ShowCount   int              `json:"showCount"`
	Libraries   []storageLibrary `json:"libraries"`
}

type storageBuilder struct {
	node     storageNode
	children map[string]*storageBuilder
	files    []storageFile
}

func (b *storageBuilder) add(parts []string, file storageFile) {
	b.node.SizeBytes += file.SizeBytes
	b.node.ItemCount++
	if len(parts) == 0 {
		b.files = append(b.files, file)
		return
	}
	name := parts[0]
	child := b.children[name]
	if child == nil {
		child = &storageBuilder{node: storageNode{Name: name}, children: map[string]*storageBuilder{}}
		b.children[name] = child
		b.node.Folders++
	}
	before := child.node.Folders
	child.add(parts[1:], file)
	b.node.Folders += child.node.Folders - before
}

func (b *storageBuilder) result() storageNode {
	out := b.node
	out.Files = append([]storageFile(nil), b.files...)
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].SizeBytes > out.Files[j].SizeBytes })
	for _, child := range b.children {
		out.Children = append(out.Children, child.result())
	}
	sort.Slice(out.Children, func(i, j int) bool {
		if out.Children[i].SizeBytes == out.Children[j].SizeBytes {
			return out.Children[i].Name < out.Children[j].Name
		}
		return out.Children[i].SizeBytes > out.Children[j].SizeBytes
	})
	return out
}

// handleLibraryStorage summarizes indexed media sizes and directory structure.
// It never walks the configured storage roots; missing/unindexed files are not
// included in this report.
func (s *Server) handleLibraryStorage(w http.ResponseWriter, r *http.Request) {
	libs := s.cfg().Libraries
	byID := make(map[string]*storageBuilder, len(libs))
	paths := make(map[string]string, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = &storageBuilder{node: storageNode{Name: filepath.Base(filepath.Clean(lib.Path))}, children: map[string]*storageBuilder{}}
		paths[lib.ID] = filepath.Clean(lib.Path)
	}
	artists, shows := map[string]bool{}, map[string]bool{}
	var total int64
	items := s.store.InternalItems()
	indexedCount := 0
	for _, item := range items {
		if item.LibraryID == nil || item.FilePath == "" {
			continue
		}
		root := byID[*item.LibraryID]
		if root == nil {
			continue
		}
		libPath := paths[*item.LibraryID]
		rel, err := filepath.Rel(libPath, item.FilePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
		if len(parts) == 1 && parts[0] == "." {
			parts = nil
		}
		root.add(parts, storageFile{Name: filepath.Base(item.FilePath), SizeBytes: item.SizeBytes, Kind: string(item.Kind)})
		indexedCount++
		total += item.SizeBytes
		if item.Author != nil && strings.TrimSpace(*item.Author) != "" {
			artists[strings.ToLower(strings.TrimSpace(*item.Author))] = true
		}
		if item.Studio != "" {
			artists[strings.ToLower(strings.TrimSpace(item.Studio))] = true
		}
		if item.ShowTitle != nil && strings.TrimSpace(*item.ShowTitle) != "" {
			shows[strings.ToLower(strings.TrimSpace(*item.ShowTitle))] = true
		}
	}
	out := storagePayload{TotalBytes: total, ItemCount: indexedCount, ArtistCount: len(artists), ShowCount: len(shows), Libraries: []storageLibrary{}}
	for _, lib := range libs {
		b := byID[lib.ID]
		root := b.result()
		out.FolderCount += root.Folders
		out.Libraries = append(out.Libraries, storageLibrary{ID: lib.ID, Name: lib.Name, Kind: lib.Kind, Root: root})
	}
	writeJSON(w, http.StatusOK, out)
}
