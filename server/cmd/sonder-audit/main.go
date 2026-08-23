// Command sonder-audit audits a catalog for movies/TV missing official
// poster art, reports which items match Wikipedia, and optionally installs
// matched posters Plex-style.
//
// Two input modes:
//
//   - Live server (concurrent dry-run report):
//     sonder-audit -api http://127.0.0.1:8797 -kind movie -limit 500
//
//   - Snapshot file (supports -apply write-back; run with server stopped):
//     sonder-audit -snapshot data/library.json -cache data/metadata-cache -apply
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tm-sonder/server/internal/enrich"
	"tm-sonder/server/internal/library"
)

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type cand struct {
	ID, Title, Kind string
	Year            int
	Studio          string
	Edition         string
	MetaSource      string
	MetaID          string
	ShowTitle       string
}

type indexedCand struct {
	idx  int64
	cand cand
}

func main() {
	snapshot := flag.String("snapshot", "", "path to library.json snapshot (apply mode)")
	apiBase := flag.String("api", "", "live server base URL, e.g. http://127.0.0.1:8797")
	cacheRoot := flag.String("cache", "", "enricher metadata-cache dir")
	kind := flag.String("kind", "movie", "audit scope: movie|tvShow|all-media")
	limit := flag.Int("limit", 25, "max items to audit")
	apply := flag.Bool("apply", false, "install matched posters Plex-style (snapshot mode only)")
	timeout := flag.Duration("timeout", 30*time.Second, "per-item provider timeout")
	token := flag.String("token", "", "pairing token when the server requires one")
	workers := flag.Int("workers", 6, "concurrent provider lookups")
	pace := flag.Duration("pace", 100*time.Millisecond, "min interval between provider requests")
	flag.Parse()

	if *cacheRoot == "" {
		fmt.Fprintln(os.Stderr, "-cache is required (metadata-cache dir)")
		os.Exit(2)
	}
	if *apiBase == "" && *snapshot == "" {
		fmt.Fprintln(os.Stderr, "provide either -api or -snapshot")
		os.Exit(2)
	}
	if *apply && *apiBase != "" {
		log.Fatal("-apply works in snapshot mode only (server holds the live store)")
	}

	var candidates []cand
	var store *library.Store

	switch {
	case *apiBase != "":
		candidates = fetchCatalog(*apiBase, *token)
	default:
		store = library.New()
		if err := store.Load(*snapshot); err != nil {
			log.Fatal(err)
		}
		for _, it := range store.InternalItems() {
			candidates = append(candidates, cand{
				ID: it.ID, Title: it.Title, Kind: string(it.Kind), Year: it.Year,
				Studio: it.Studio, ShowTitle: derefStr(it.ShowTitle),
				MetaSource: derefStr(it.MetadataIDSource), MetaID: derefStr(it.MetadataID),
			})
		}
	}

	enricher := enrich.New(*cacheRoot)
	enricher.Pacing = *pace
	matched := runAudit(enricher, candidates, auditOptions{
		kind: *kind, limit: *limit,
		apply:   *apply && store != nil,
		workers: *workers, timeout: *timeout,
		store: store,
	})

	fmt.Printf("\nTOTAL MATCHES: %d\n", matched)

	if *apply && store != nil {
		out := strings.TrimSuffix(*snapshot, ".json") + "-audited.json"
		if err := store.Save(out); err != nil {
			log.Fatalf("save audited snapshot: %v", err)
		}
		fmt.Printf("audited snapshot written to %s\n(swap in as library.json while the server is stopped)\n", out)
	}
}

// ---- shared audit loop ----

type auditOptions struct {
	kind    string
	limit   int
	apply   bool
	workers int
	timeout time.Duration
	store   *library.Store // non-nil in apply mode
}

type resultRow struct {
	index         int
	title, status string
	detail        string
	isMatch       bool
}

func wantedKind(scope, itemKind string) bool {
	switch scope {
	case "all-media":
		return true
	case "movie":
		return itemKind == "movie" || itemKind == "documentary"
	case "tvShow":
		return itemKind == "tvShow"
	default:
		return false
	}
}

func runAudit(e *enrich.Enricher, candidates []cand, opt auditOptions) int {
	var wanted []indexedCand
	skipped := 0
	for _, c := range candidates {
		if !wantedKind(opt.kind, c.Kind) {
			skipped++
			continue
		}
		wanted = append(wanted, indexedCand{idx: int64(len(wanted)), cand: c})
	}
	limit := opt.limit
	if limit <= 0 || limit > len(wanted) {
		limit = len(wanted)
	}
	fmt.Fprintf(os.Stderr, "[audit] %d candidate(s) in scope, %d skipped (limit %d)\n", len(wanted), skipped, limit)

	results := make([]resultRow, len(wanted))
	in := make(chan indexedCand)
	var wg sync.WaitGroup
	var audited atomic.Int64
	worker := func() {
		defer wg.Done()
		for jc := range in {
			n := audited.Add(1)
			if n%100 == 0 {
				fmt.Fprintf(os.Stderr, "[audit] %d/%d\n", n, limit)
			}

			c := jc.cand
			in2 := enrich.Input{
				Title: c.Title, Kind: c.Kind, Year: c.Year, Studio: c.Studio,
				Edition:          c.Edition,
				MetadataIDSource: c.MetaSource,
				MetadataID:       c.MetaID,
				ShowTitle:        c.ShowTitle,
			}
			ctx, cancel := context.WithTimeout(context.Background(), opt.timeout)
			res, err := e.Enrich(ctx, in2)
			cancel()

			row := resultRow{index: int(jc.idx), title: c.Title}
			switch {
			case err != nil:
				row.status = "ERROR"
				row.detail = err.Error()
			case res == nil:
				row.status = "NO MATCH"
			case strings.TrimSpace(res.Summary) == "" && res.PosterPath == "":
				row.status = "EMPTY RESULT"
			default:
				row.status = "MATCH"
				row.isMatch = true
				row.detail = fmt.Sprintf("poster=%v provider=%s summary=%d chars",
					res.PosterPath != "", res.Provider, len(res.Summary))
				if opt.store != nil {
					if fresh, ok := opt.store.Get(c.ID); ok {
						if res.Summary != "" && fresh.Summary == "" {
							fresh.Summary = res.Summary
						}
						if res.PosterPath != "" && fresh.PosterSource != "local" {
							dest, _ := library.PlexArtPaths(fresh)
							if installed, cerr := library.CopyArtTo(res.PosterPath, dest); cerr == nil {
								fresh.PosterPath = installed
							} else {
								fresh.PosterPath = res.PosterPath
							}
							u := "/artwork/poster/" + fresh.ID
							fresh.PosterURL = &u
							fresh.PosterSource = res.Provider
						}
						opt.store.Upsert(fresh)
						row.detail += " → INSTALLED"
					}
				}
			}
			results[jc.idx] = row
		}
	}

	for i := 0; i < opt.workers; i++ {
		wg.Add(1)
		go worker()
	}
	for i := 0; i < limit; i++ {
		in <- wanted[i]
	}
	close(in)
	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].index < results[b].index })

	fmt.Println()
	for _, r := range results {
		status := r.status
		switch {
		case r.isMatch:
			status = "✓ MATCH"
		case status == "NO MATCH", status == "EMPTY RESULT":
			status = "· no match"
		case status == "ERROR":
			status = "! error"
		}
		line := fmt.Sprintf(" %-11s %s", status, r.title)
		if r.detail != "" {
			line += "  [" + r.detail + "]"
		}
		fmt.Println(line)
	}

	matched := 0
	for _, r := range results {
		if r.isMatch {
			matched++
		}
	}
	return matched
}

// ---- live catalog fetch ----

type wireItem struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Kind      string  `json:"kind"`
	Year      int     `json:"year"`
	Summary   string  `json:"summary"`
	PosterURL *string `json:"posterURL"`
	ShowTitle *string `json:"showTitle"`
}

func fetchCatalog(base, token string) []cand {
	url := strings.TrimRight(base, "/") + "/api/library"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		q := req.URL.Query()
		q.Set("token", token)
		req.URL.RawQuery = q.Encode()
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("fetch catalog: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Fatalf("fetch catalog: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Items []wireItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Fatalf("decode catalog: %v", err)
	}
	out := make([]cand, 0, len(payload.Items))
	for _, it := range payload.Items {
		if it.Summary != "" {
			continue // already enriched
		}
		out = append(out, cand{
			ID: it.ID, Title: it.Title, Kind: it.Kind, Year: it.Year,
			ShowTitle: derefStr(it.ShowTitle),
		})
	}
	return out
}
