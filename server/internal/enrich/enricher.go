// Package enrich ports SonderMetadataEnrichment.swift: keyless metadata
// lookups (Audnexus for audiobooks, Open Library for ebooks, Wikipedia
// fallback) with an on-disk SHA256-keyed cache.
package enrich

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Input carries the catalog fields the query builders need, decoupled from
// the library package.
type Input struct {
	Title            string
	Kind             string // movie|tvShow|documentary|audiobook|ebook
	Year             int
	Studio           string
	Edition          string
	Summary          string
	ShowTitle        string
	Season           int
	Episode          int
	MetadataIDSource string
	MetadataID       string
}

type Enrichment struct {
	Summary      string   `json:"summary"`
	Publisher    string   `json:"publisher"`
	PosterPath   string   `json:"posterPath,omitempty"`
	BackdropPath string   `json:"backdropPath,omitempty"`
	Tags         []string `json:"tags"`
}

type Enricher struct {
	CacheRoot string
	Client    *http.Client
}

func New(cacheRoot string) *Enricher {
	return &Enricher{CacheRoot: cacheRoot, Client: &http.Client{Timeout: 20 * time.Second}}
}

// Enrich returns cached metadata when present, else queries the provider for
// the item kind and populates the cache. A nil result means "nothing found".
func (e *Enricher) Enrich(ctx context.Context, in Input) (*Enrichment, error) {
	query := makeQuery(in)
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	sum := sha256.Sum256([]byte(query))
	key := hex.EncodeToString(sum[:])
	cacheJSON := filepath.Join(e.CacheRoot, key+".json")
	cachePoster := filepath.Join(e.CacheRoot, key+".jpg")
	cacheBackdrop := filepath.Join(e.CacheRoot, key+"-backdrop.jpg")

	if payload, err := os.ReadFile(cacheJSON); err == nil {
		var cached Enrichment
		if json.Unmarshal(payload, &cached) == nil {
			cached.PosterPath = existingOrEmpty(cachePoster)
			cached.BackdropPath = existingOrEmpty(cacheBackdrop)
			return &cached, nil
		}
	}

	var (
		result *Enrichment
		err    error
	)
	switch in.Kind {
	case "audiobook":
		result, err = e.audnexusLookup(ctx, in, cacheJSON, cachePoster)
	case "ebook":
		result, err = e.openLibraryLookup(ctx, in, cacheJSON, cachePoster)
	default:
		result, err = e.wikipediaSearch(ctx, query, cacheJSON, cachePoster, cacheBackdrop)
	}
	if err != nil || result == nil {
		return result, err
	}
	result.PosterPath = existingOrEmpty(cachePoster)
	result.BackdropPath = existingOrEmpty(cacheBackdrop)
	return result, nil
}

func existingOrEmpty(path string) string {
	if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() > 0 {
		return path
	}
	return ""
}

// makeQuery ports SonderMetadataEnricher.makeQuery.
func makeQuery(in Input) string {
	var year string
	if in.Year != 0 {
		year = strconv.Itoa(in.Year)
	}
	join := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, " ")
	}
	switch in.Kind {
	case "movie":
		return join(in.Title, in.Edition, year, "film", "Wikipedia")
	case "documentary":
		return join(in.Title, in.Edition, year, "documentary", "Wikipedia")
	case "tvShow":
		show := in.ShowTitle
		if show == "" {
			show = in.Title
		}
		if in.Season > 0 && in.Episode > 0 {
			code := fmt.Sprintf("S%02dE%02d", in.Season, in.Episode)
			return join(show, code, in.Title, "episode", "Wikipedia")
		}
		return join(show, "television series", "Wikipedia")
	case "ebook":
		return join(in.Title, in.Edition, in.Studio, year, "book", "Wikipedia")
	case "audiobook":
		return join(in.Title, in.Edition, in.Studio, year, "audiobook", "Wikipedia")
	default:
		return ""
	}
}

// --- HTTP helpers ---

func (e *Enricher) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TM-Sonder/1.0 (metadata enricher)")
	resp, err := e.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("enrich: %s -> %d", hostOf(rawURL), resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func (e *Enricher) downloadTo(ctx context.Context, rawURL, dest string) {
	data, err := e.fetch(ctx, rawURL)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(dest), 0o755)
	_ = os.WriteFile(dest, data, 0o600)
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}
	return rawURL
}

func writeCache(path string, v any) {
	if data, err := json.Marshal(v); err == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		_ = os.WriteFile(path, data, 0o600)
	}
}
