// Command sonder runs the TM Sonder media server (Go port).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/artwork"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/httpapi"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/probe"
	"tm-sonder/server/internal/transcode"
	"tm-sonder/server/internal/zeroconf"
)

var (
	version   = httpapi.Version
	swiftLibs = []string{
		filepath.Join("Library", "Application Support", "TM Sonder", "library.json"),
	}
)

func main() {
	configPath := flag.String("config", "", "path to server config JSON")
	showVersion := flag.Bool("version", false, "print version and exit")
	importPlex := flag.String("import-plex", "", "one-time: import items from a Plex database.sqlite")
	enrichPass := flag.Bool("enrich", false, "one-time: fetch metadata for items missing summaries, then exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sonder %s (%s)\n", version, httpapi.Build)
		return
	}
	if err := run(*configPath, *importPlex, *enrichPass); err != nil {
		log.Fatal(err)
	}
}

func run(configFlag, plexDB string, enrichPass bool) error {
	logger := log.New(os.Stderr, "sonder ", log.LstdFlags)

	path := configFlag
	if path == "" {
		p, _ := config.ResolvePath()
		path = p
	}
	if path == "" {
		return errors.New("no config path resolvable; pass -config")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := config.WriteTemplate(path); err != nil {
			return fmt.Errorf("generate default config: %w", err)
		}
		logger.Printf("generated default config at %s — edit libraries before heavy use", path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("dataDir: %w", err)
	}
	// API.md contract: enabling LAN auto-generates a pairing token, persisted
	// so restarts keep the same secret.
	if cfg.AllowLAN && cfg.PairingToken == "" {
		tokenFile := filepath.Join(cfg.DataDir, "pairing-token")
		if data, err := os.ReadFile(tokenFile); err == nil && len(data) >= 16 {
			cfg.PairingToken = strings.TrimSpace(string(data))
		} else {
			tok, genErr := generateToken()
			if genErr != nil {
				return fmt.Errorf("generate pairing token: %w", genErr)
			}
			cfg.PairingToken = tok
			if werr := os.WriteFile(tokenFile, []byte(tok+"\n"), 0o600); werr != nil {
				logger.Printf("could not persist pairing token: %v", werr)
			}
		}
		logger.Printf("LAN enabled with generated pairing token (full value in %s)",
			filepath.Join(cfg.DataDir, "pairing-token"))
	}
	logConfigSummary(logger, cfg)

	store := library.New()
	snapshotPath := filepath.Join(cfg.DataDir, "library.json")

	// Startup state: Go snapshot first, else one-time Swift migration.
	if err := store.Load(snapshotPath); err != nil {
		logger.Printf("no usable snapshot (%v)", err)
		for _, rel := range swiftLibs {
			home, _ := os.UserHomeDir()
			candidate := filepath.Join(home, rel)
			if _, statErr := os.Stat(candidate); statErr != nil {
				continue
			}
			res, importErr := library.ImportSwiftLibraryJSON(store, candidate)
			if importErr != nil {
				logger.Printf("swift import from %s failed: %v", candidate, importErr)
				continue
			}
			logger.Printf("imported %d items from macOS app snapshot %s (skipped %d no-file, %d missing)",
				res.Items, candidate, res.SkippedNoFile, res.MissingOnDisk)
			break
		}
	} else {
		logger.Printf("loaded snapshot: %d items, generation %d", store.Count(), store.Generation())
	}

	// One-time Plex migration when requested.
	if plexDB != "" {
		res, err := library.ImportPlexLibrary(store, plexDB, 0)
		if err != nil {
			return fmt.Errorf("plex import: %w", err)
		}
		logger.Printf("plex import: %d movie(s), %d show(s), %d episode(s); skipped %d missing files",
			res.Movies, res.Shows, res.Episodes, res.Skipped)
		if err := store.Flush(snapshotPath); err != nil {
			logger.Printf("post-import snapshot save failed: %v", err)
		}
		return nil // one-time maintenance op: do not serve
	}

	// One-time metadata enrichment pass.
	if enrichPass {
		enriched := httpapi.RunEnrichmentPass(logger, store, filepath.Join(cfg.DataDir, "metadata-cache"))
		logger.Printf("enrichment pass: updated %d item(s)", enriched)
		if err := store.Flush(snapshotPath); err != nil {
			logger.Printf("post-enrich snapshot save failed: %v", err)
		}
		return nil // one-time maintenance op: do not serve
	}

	scanner := library.NewScanner(store)
	if _, err := lookPath(cfg.FFprobePath); err == nil {
		probeWorkers := cfg.ProbeWorkers
		if probeWorkers <= 0 {
			probeWorkers = runtime.NumCPU() / 2
		}
		if probeWorkers < 2 {
			probeWorkers = 2
		}
		logger.Printf("probe workers: %d", probeWorkers)
		scanner.SetProber(probe.NewCache(cfg.FFprobePath), probeWorkers)
		if ffmpegPath, ferr := lookPath(cfg.FFmpegPath); ferr == nil {
			gen := &artwork.Generator{FFmpegPath: ffmpegPath, OutDir: filepath.Join(cfg.DataDir, "artwork")}
			scanner.SetThumbnailDir(gen.OutDir)
			scanner.SetThumbnailGen(func(ctx context.Context, itemID, videoPath string, dur float64) (string, error) {
				return gen.Generate(ctx, videoPath, itemID, dur)
			})
		} else {
			logger.Printf("ffmpeg not found at %q — poster generation disabled", cfg.FFmpegPath)
		}
	} else {
		logger.Printf("ffprobe not found at %q — track probing disabled", cfg.FFprobePath)
	}

	tm := transcode.NewManager(transcode.Config{
		FFmpegPath:    cfg.FFmpegPath,
		HWAccel:       cfg.Transcode.HWAccel,
		Preset:        cfg.Transcode.Preset,
		MaxConcurrent: cfg.Transcode.MaxConcurrent,
	})

	srv := httpapi.New(cfg, store, scanner, tm)
	srv.SetTrackRefresher(scanner)
	srv.SetConfigPath(path)
	// Populate the /api/library mediaDirectories table from config.
	dirs := make([]api.MediaDirectory, 0, len(cfg.Libraries))
	for _, l := range cfg.Libraries {
		dirs = append(dirs, api.MediaDirectory{ID: l.ID, Name: l.Name, Kind: l.Kind, LibraryID: l.ID})
	}
	if len(dirs) > 0 {
		store.SetDirectories(dirs)
	}
	if _, err := lookPath(cfg.FFprobePath); err == nil {
		srv.SetChapterProvider(probe.NewChapterSource(
			func(id string) (string, time.Time, bool) {
				it, ok := store.Get(id)
				if !ok {
					return "", time.Time{}, false
				}
				return it.FilePath, it.ModTime, true
			}, cfg.FFprobePath))
	}
	srv.SetAutoSave(func() {
		store.SaveDebounced(snapshotPath, library.DefaultSaveDelay)
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initial scan in the background so the port opens immediately.
	go func() {
		started := time.Now()
		res, err := scanner.ScanAll(cfg.Libraries)
		if err != nil && !errors.Is(err, library.ErrScanInProgress) {
			logger.Printf("initial scan failed: %v", err)
			return
		}
		logger.Printf("scan complete in %s: added=%d updated=%d removed=%d skipped=%d",
			time.Since(started).Round(time.Millisecond), res.Added, res.Updated, res.Removed, res.Skipped)
		if err := store.Flush(snapshotPath); err != nil {
			logger.Printf("snapshot save failed: %v", err)
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", httpServer.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	// Bonjour advertisement (LAN only; pointless otherwise).
	var advertiser *zeroconf.Advertiser
	if cfg.AllowLAN {
		if a, err := zeroconf.Start("TM Sonder", cfg.Port); err != nil {
			logger.Printf("bonjour advertisement unavailable: %v", err)
		} else {
			advertiser = a
			logger.Printf("advertising _tmsonder._tcp on port %d", cfg.Port)
		}
	}

	select {
	case <-ctx.Done():
		logger.Printf("shutting down...")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			stop()
			tm.StopAll()
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if advertiser != nil {
		advertiser.Stop()
	}
	_ = httpServer.Shutdown(shutdownCtx)
	tm.StopAll()
	if err := store.Flush(snapshotPath); err != nil {
		logger.Printf("final snapshot save failed: %v", err)
	} else {
		logger.Printf("snapshot saved to %s", snapshotPath)
	}
	logger.Printf("bye")
	return nil
}

func logConfigSummary(logger *log.Logger, cfg *config.Config) {
	tokenState := "none"
	if cfg.PairingToken != "" {
		tokenState = "configured (never logged)"
	}
	logger.Printf("config: port=%d dataDir=%s allowLAN=%v token=%s theme=%s hwaccel=%s maxTranscode=%d libraries=%d",
		cfg.Port, cfg.DataDir, cfg.AllowLAN, tokenState, cfg.ThemePreset,
		cfg.Transcode.HWAccel, cfg.Transcode.MaxConcurrent, len(cfg.Libraries))
	for _, lib := range cfg.Libraries {
		logger.Printf("  library %q kind=%s path=%s", lib.Name, lib.Kind, lib.Path)
	}
}

// generateToken returns a 128-bit hex pairing token.// generateToken returns a 128-bit hex pairing token.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lookPath resolves a binary name or path without importing os/exec here.
func lookPath(p string) (string, error) {
	if p == "" {
		return "", os.ErrNotExist
	}
	if filepath.Base(p) == p {
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			full := filepath.Join(dir, p)
			if info, err := os.Stat(full); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				return full, nil
			}
		}
		return "", os.ErrNotExist
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", os.ErrNotExist
	}
	return p, nil
}
