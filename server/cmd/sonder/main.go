// Command sonder runs the TM Sonder media server (Go port).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/artwork"
	"tm-sonder/server/internal/audiobookopt"
	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/enrich"
	"tm-sonder/server/internal/httpapi"
	"tm-sonder/server/internal/library"
	"tm-sonder/server/internal/network"
	"tm-sonder/server/internal/probe"
	"tm-sonder/server/internal/proxy"
	"tm-sonder/server/internal/runtimecontrol"
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
	if handled, err := runAppCLI(os.Args[1:]); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}
	configPath := flag.String("config", "", "path to server config JSON")
	showVersion := flag.Bool("version", false, "print version and exit")
	importPlex := flag.String("import-plex", "", "one-time: import items from a Plex database.sqlite")
	exportData := flag.String("export-data", "", "one-time: export catalog and user data to a JSON bundle, then exit")
	importData := flag.String("import-data", "", "one-time: import a Sonder JSON bundle, then exit")
	importMode := flag.String("import-mode", "replace", "import mode: replace or merge")
	importPathMap := flag.String("import-path-map", "", "optional comma-separated exported-root=current-root mappings")
	enrichPass := flag.Bool("enrich", false, "one-time: fetch metadata for items missing summaries, then exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("sonder %s (%s)\n", version, httpapi.Build)
		return
	}
	if err := run(*configPath, *importPlex, *exportData, *importData, *importMode, *importPathMap, *enrichPass); err != nil {
		log.Fatal(err)
	}
}

func run(configFlag, plexDB, exportPath, importPath, importMode, importPathMap string, enrichPass bool) error {
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
	interfaces, err := network.Enumerate()
	if err != nil {
		return fmt.Errorf("enumerate network interfaces: %w", err)
	}
	statePath := network.StatePath(cfg.DataDir)
	runtimeState, err := loadRuntimeState(statePath, cfg, interfaces)
	if err != nil {
		return err
	}
	// The legacy HTTP API uses cfg.Port in its discovery/settings payloads. Keep
	// that compatibility field synchronized with the persisted runtime state.
	cfg.Port = runtimeState.WebPort
	cfg.WebPort = runtimeState.WebPort
	cfg.APIPort = runtimeState.APIPort
	if err := network.SaveState(statePath, runtimeState); err != nil {
		return fmt.Errorf("save runtime state: %w", err)
	}
	caddyBinary := strings.TrimSpace(cfg.CaddyPath)
	if caddyBinary == "" {
		caddyBinary = "caddy"
	}
	caddyConfig := strings.TrimSpace(cfg.CaddyConfigPath)
	if caddyConfig == "" {
		caddyConfig = filepath.Join(cfg.DataDir, "Caddyfile")
	}
	caddy := proxy.Manager{
		Enabled:      runtimeState.CaddyEnabled,
		Binary:       caddyBinary,
		ConfigPath:   caddyConfig,
		LaunchdLabel: cfg.CaddyLaunchdLabel,
	}
	if runtimeState.CaddyEnabled {
		if err := caddy.Apply(context.Background(), runtimeState); err != nil {
			runtimeState.LastError = "Caddy startup: " + err.Error()
			logger.Printf("Caddy startup failed: %v", err)
			if saveErr := network.SaveState(statePath, runtimeState); saveErr != nil {
				logger.Printf("could not persist Caddy startup failure: %v", saveErr)
			}
		}
	}
	controller := &runtimecontrol.Controller{
		StatePath:    statePath,
		State:        runtimeState,
		Caddy:        caddy,
		LaunchdLabel: runtimecontrol.DefaultLaunchdLabel,
		PairingToken: cfg.PairingToken,
		InterfacesFn: network.Enumerate,
	}
	logConfigSummary(logger, cfg, runtimeState)

	store := library.New()
	snapshotPath := filepath.Join(cfg.DataDir, "library.json")
	progressPath := filepath.Join(cfg.DataDir, "progress.json")

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
		// Progress is written to a small sidecar on every heartbeat, so it can
		// be newer than the catalog snapshot. Overlay it last.
		if perr := store.LoadProgress(progressPath); perr == nil {
			logger.Printf("applied progress sidecar")
		} else if !os.IsNotExist(perr) {
			logger.Printf("progress sidecar unreadable (%v)", perr)
		}
	}
	if exportPath != "" && importPath != "" {
		return errors.New("-export-data and -import-data cannot be used together")
	}
	if importPath != "" {
		data, err := os.ReadFile(importPath)
		if err != nil {
			return fmt.Errorf("read data bundle: %w", err)
		}
		var bundle library.DataBundle
		if err := json.Unmarshal(data, &bundle); err != nil {
			return fmt.Errorf("parse data bundle: %w", err)
		}
		mappings, err := parsePathMappings(importPathMap)
		if err != nil {
			return err
		}
		remapped, err := library.RemapBundlePaths(&bundle, cfg.Libraries, mappings)
		if err != nil {
			return fmt.Errorf("prepare data bundle: %w", err)
		}
		result, err := store.ImportBundle(bundle, importMode)
		if err != nil {
			return fmt.Errorf("import data bundle: %w", err)
		}
		if err := store.Flush(snapshotPath); err != nil {
			return fmt.Errorf("save imported catalog: %w", err)
		}
		if err := store.SaveProgress(progressPath); err != nil {
			return fmt.Errorf("save imported progress: %w", err)
		}
		logger.Printf("data import: mode=%s items=%d lists=%d progress=%d paths-remapped=%d; no scan performed", result.Mode, result.Items, result.Lists, result.Progress, remapped)
		return nil
	}
	if exportPath != "" {
		bundle := store.ExportBundle(cfg.Libraries)
		data, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			return fmt.Errorf("encode data bundle: %w", err)
		}
		if err := writeJSONAtomic(exportPath, data); err != nil {
			return fmt.Errorf("write data bundle: %w", err)
		}
		logger.Printf("data export: %d item(s), %d list(s) to %s", len(bundle.Snapshot.Items), len(bundle.Snapshot.Lists), exportPath)
		return nil
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
		enriched := enrich.RunPass(context.Background(), logger, store,
			filepath.Join(cfg.DataDir, "metadata-cache"))
		logger.Printf("enrichment pass: updated %d item(s)", enriched)
		if err := store.Flush(snapshotPath); err != nil {
			logger.Printf("post-enrich snapshot save failed: %v", err)
		}
		return nil // one-time maintenance op: do not serve
	}

	scanner := library.NewScanner(store)
	scanner.SetSafeScan(cfg.SafeScan)
	if backfilled := scanner.BackfillStableKeys(cfg.Libraries); backfilled > 0 {
		logger.Printf("backfilled %d portable media identity(ies)", backfilled)
	}
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
		scanner.SetThumbWorkers(cfg.ThumbWorkers)
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
	srv.SetRuntimeControl(controller)
	srv.SetTrackRefresher(scanner)
	srv.SetConfigPath(path)
	srv.SetSnapshotPath(snapshotPath)
	var audiobookOptimizer *audiobookopt.Manager
	ffmpegForJobs, ffmpegErr := lookPath(cfg.FFmpegPath)
	ffprobeForJobs, ffprobeErr := lookPath(cfg.FFprobePath)
	var optimizerErr error
	if ffmpegErr != nil || ffprobeErr != nil {
		optimizerErr = fmt.Errorf("FFmpeg and FFprobe are required")
	} else {
		audiobookOptimizer, optimizerErr = audiobookopt.New(store, cfg.Libraries, cfg.DataDir, ffmpegForJobs, ffprobeForJobs)
	}
	if optimizerErr != nil {
		logger.Printf("audiobook optimization worker unavailable: %v", optimizerErr)
	} else {
		srv.SetAudiobookOptimizer(audiobookOptimizer)
		logger.Printf("audiobook optimization worker ready; local workspace=%s", filepath.Join(cfg.DataDir, "audiobook-optimization"))
	}
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
	srv.SetProgressSave(func() {
		store.SaveProgressDebounced(progressPath, library.DefaultSaveDelay)
	})

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(runtimeState.WebBindAddress, strconv.Itoa(runtimeState.WebPort)),
		Handler:           srv.WebHandler(net.JoinHostPort("127.0.0.1", strconv.Itoa(runtimeState.APIPort))),
		ReadHeaderTimeout: 10 * time.Second,
	}
	apiServer := &http.Server{
		Addr:              net.JoinHostPort(runtimeState.APIBindAddress, strconv.Itoa(runtimeState.APIPort)),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	apiSharesWebListener := httpServer.Addr == apiServer.Addr

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Snapshot the config values main needs after the server starts taking
	// requests: the server swaps in fresh config copies on settings updates,
	// so reading cfg directly here would race those writes.
	initialLibs := append([]config.Library(nil), cfg.Libraries...)
	allowLAN := cfg.AllowLAN

	// Build the browser payload from the loaded snapshot independently of the
	// NAS walk. The HTTP handler serves this warm copy while the scan runs.
	go func() {
		started := time.Now()
		srv.WarmLibraryCache()
		logger.Printf("library cache warmed in %s", time.Since(started).Round(time.Millisecond))
	}()

	// Initial scan in the background so the port opens immediately.
	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		started := time.Now()
		res, err := scanner.ScanAll(initialLibs)
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

	errCh := make(chan error, 2)
	go func() {
		logger.Printf("listening on %s", httpServer.Addr)
		errCh <- httpServer.ListenAndServe()
	}()
	if !apiSharesWebListener {
		go func() {
			logger.Printf("private API listening on %s", apiServer.Addr)
			errCh <- apiServer.ListenAndServe()
		}()
	}

	// Bonjour advertisement (LAN only; pointless otherwise).
	var advertiser *zeroconf.Advertiser
	if allowLAN && runtimeState.WebBindAddress != "127.0.0.1" {
		if a, err := zeroconf.Start("TM Sonder", runtimeState.WebPort); err != nil {
			logger.Printf("bonjour advertisement unavailable: %v", err)
		} else {
			advertiser = a
			logger.Printf("advertising _tmsonder._tcp on port %d", runtimeState.WebPort)
		}
	}

	select {
	case <-ctx.Done():
		logger.Printf("shutting down...")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			stop()
			scanner.Shutdown()
			tm.StopAll()
			if audiobookOptimizer != nil {
				_ = audiobookOptimizer.Close(context.Background())
			}
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if advertiser != nil {
		advertiser.Stop()
	}
	if audiobookOptimizer != nil {
		if err := audiobookOptimizer.Close(shutdownCtx); err != nil {
			logger.Printf("audiobook optimization worker shutdown: %v", err)
		}
	}
	// Cancel in-flight scan/probe work, then let it unwind before the final
	// snapshot so we never persist a half-applied scan.
	scanner.Shutdown()
	select {
	case <-scanDone:
	case <-time.After(3 * time.Second):
		logger.Printf("initial scan did not stop within 3s; continuing shutdown")
	}
	_ = httpServer.Shutdown(shutdownCtx)
	if !apiSharesWebListener {
		_ = apiServer.Shutdown(shutdownCtx)
	}
	tm.StopAll()
	if err := store.Flush(snapshotPath); err != nil {
		logger.Printf("final snapshot save failed: %v", err)
	} else {
		logger.Printf("snapshot saved to %s", snapshotPath)
	}
	logger.Printf("bye")
	return nil
}

func parsePathMappings(raw string) (map[string]string, error) {
	mappings := map[string]string{}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		from, to, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
			return nil, fmt.Errorf("invalid -import-path-map entry %q; expected exported-root=current-root", entry)
		}
		mappings[strings.TrimSpace(from)] = strings.TrimSpace(to)
	}
	return mappings, nil
}

// runAppCLI implements the operational commands that are useful when the
// server is installed as a per-user LaunchAgent. It returns handled=false for
// the existing flag-based server startup path.
func runAppCLI(args []string) (handled bool, err error) {
	if len(args) == 0 || args[0] != "app" {
		return false, nil
	}
	args = args[1:]
	if len(args) == 0 {
		return true, errors.New("usage: sonder app interfaces|status|exposure|proxy status|restart <mode-or-interface>")
	}
	configPath := ""
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--config":
			if i+1 >= len(args) {
				return true, errors.New("--config requires a path")
			}
			configPath = args[i+1]
			i++
		default:
			filtered = append(filtered, args[i])
		}
	}
	args = filtered

	load := func() (*config.Config, network.RuntimeState, *runtimecontrol.Controller, error) {
		path := configPath
		if path == "" {
			path, _ = config.ResolvePath()
		}
		var cfg *config.Config
		var loadErr error
		if path == "" || !fileExists(path) {
			cfg = ptrConfig(config.Default())
			if path != "" && configPath != "" {
				return nil, network.RuntimeState{}, nil, fmt.Errorf("config file not found: %s", path)
			}
		} else {
			cfg, loadErr = config.Load(path)
			if loadErr != nil {
				return nil, network.RuntimeState{}, nil, loadErr
			}
		}
		interfaces, enumErr := network.Enumerate()
		if enumErr != nil {
			return nil, network.RuntimeState{}, nil, enumErr
		}
		statePath := network.StatePath(cfg.DataDir)
		state, stateErr := network.LoadState(statePath)
		if stateErr != nil {
			if !os.IsNotExist(stateErr) {
				return nil, network.RuntimeState{}, nil, stateErr
			}
			state = network.DefaultState(cfg.WebPort, cfg.APIPort, cfg.AllowLAN, interfaces)
		} else {
			network.Normalize(&state, cfg.WebPort, cfg.APIPort)
			if resolveErr := network.ResolveAndApply(&state, interfaces); resolveErr != nil {
				return nil, network.RuntimeState{}, nil, resolveErr
			}
		}
		network.Normalize(&state, cfg.WebPort, cfg.APIPort)
		caddyBinary := cfg.CaddyPath
		if caddyBinary == "" {
			caddyBinary = "caddy"
		}
		caddyConfig := cfg.CaddyConfigPath
		if caddyConfig == "" {
			caddyConfig = filepath.Join(cfg.DataDir, "Caddyfile")
		}
		controller := &runtimecontrol.Controller{
			StatePath: statePath, State: state,
			Caddy:        proxy.Manager{Enabled: state.CaddyEnabled, Binary: caddyBinary, ConfigPath: caddyConfig, LaunchdLabel: cfg.CaddyLaunchdLabel},
			LaunchdLabel: runtimecontrol.DefaultLaunchdLabel,
			PairingToken: cfg.PairingToken,
			InterfacesFn: network.Enumerate,
		}
		return cfg, state, controller, nil
	}

	switch args[0] {
	case "interfaces":
		items, enumErr := network.Enumerate()
		if enumErr != nil {
			return true, enumErr
		}
		if jsonOutput {
			return true, json.NewEncoder(os.Stdout).Encode(items)
		}
		for _, item := range items {
			fmt.Printf("%-8s %-18s %-16s %s\n", item.ID, item.Type, item.IPv4, item.Name)
		}
		return true, nil
	case "status":
		cfg, state, controller, loadErr := load()
		_ = cfg
		_ = state
		if loadErr != nil {
			return true, loadErr
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		status, statusErr := controller.Status(ctx)
		if statusErr != nil {
			return true, statusErr
		}
		payload := map[string]any{"version": httpapi.Version, "build": httpapi.Build, "status": status}
		if jsonOutput {
			data, marshalErr := json.MarshalIndent(payload, "", "  ")
			if marshalErr != nil {
				return true, marshalErr
			}
			fmt.Println(string(data))
			return true, nil
		}
		printHumanStatus(payload)
		return true, nil
	case "proxy":
		if len(args) < 2 {
			return true, errors.New("usage: sonder app proxy status|enable|disable")
		}
		_, state, controller, loadErr := load()
		if loadErr != nil {
			return true, loadErr
		}
		if args[1] == "enable" || args[1] == "disable" {
			enabled := args[1] == "enable"
			domain := cliOption(args[2:], "--domain")
			health := cliOption(args[2:], "--health")
			bind := cliOption(args[2:], "--bind")
			result, configureErr := controller.ConfigureProxy(context.Background(), enabled, domain, health, bind)
			if configureErr != nil {
				return true, configureErr
			}
			if jsonOutput {
				data, marshalErr := json.MarshalIndent(result, "", "  ")
				if marshalErr != nil {
					return true, marshalErr
				}
				fmt.Println(string(data))
			} else {
				fmt.Printf("Caddy %s\n", map[bool]string{true: "enabled", false: "disabled"}[enabled])
			}
			return true, nil
		}
		if args[1] != "status" {
			return true, errors.New("usage: sonder app proxy status|enable|disable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status := controller.Caddy.Status(ctx, state)
		fmt.Printf("enabled: %v\nconfigured: %v\ndomain: %s\nupstream: %s\nlistening: %v\npublic healthy: %v\n",
			status.Enabled, status.Configured, status.Domain, status.Upstream, status.Listening, status.PublicHealthy)
		if status.Error != "" {
			fmt.Printf("error: %s\n", status.Error)
		}
		fmt.Println("external prerequisites: DNS, router/firewall TCP 80/443 forwarding, and any WireGuard routes")
		return true, nil
	case "exposure", "ports":
		_, state, controller, loadErr := load()
		if loadErr != nil {
			return true, loadErr
		}
		modeValue := cliOption(args[1:], "--mode")
		mode := state.SelectedMode
		interfaceID := state.SelectedInterface
		if modeValue != "" {
			mode, interfaceID = cliBinding(modeValue)
		}
		if requestedInterface := cliOption(args[1:], "--interface"); requestedInterface != "" {
			interfaceID = requestedInterface
			if modeValue == "" {
				mode = network.ModeExact
			}
		}
		webPort, parseErr := cliIntOption(args[1:], "--web-port")
		if parseErr != nil {
			return true, parseErr
		}
		apiPort, parseErr := cliIntOption(args[1:], "--api-port")
		if parseErr != nil {
			return true, parseErr
		}
		if webPort == 0 && apiPort == 0 && modeValue == "" && interfaceID == state.SelectedInterface {
			return true, errors.New("provide --web-port, --api-port, --mode, or --interface")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, switchErr := controller.SwitchExposure(ctx, mode, interfaceID, webPort, apiPort)
		if switchErr != nil {
			return true, switchErr
		}
		if jsonOutput {
			data, marshalErr := json.MarshalIndent(result, "", "  ")
			if marshalErr != nil {
				return true, marshalErr
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("exposure active: mode=%s interface=%s web=%s:%d api=%s:%d\n",
				result.SelectedMode, result.SelectedInterface, result.WebBindAddress, result.WebPort,
				result.APIBindAddress, result.APIPort)
		}
		return true, nil
	case "restart":
		if len(args) != 2 {
			return true, errors.New("usage: sonder app restart wifi|ethernet|vpn|loopback|public|<interface-id>")
		}
		_, _, controller, loadErr := load()
		if loadErr != nil {
			return true, loadErr
		}
		mode, interfaceID := cliBinding(args[1])
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, switchErr := controller.Switch(ctx, mode, interfaceID)
		if switchErr != nil {
			return true, switchErr
		}
		fmt.Printf("binding active: mode=%s interface=%s web=%s:%d api=%s:%d\n",
			result.SelectedMode, result.SelectedInterface, result.WebBindAddress, result.WebPort,
			result.APIBindAddress, result.APIPort)
		return true, nil
	default:
		return true, fmt.Errorf("unknown app command %q", args[0])
	}
}

func cliOption(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return strings.TrimSpace(args[i+1])
		}
	}
	return ""
}

func cliIntOption(args []string, name string) (int, error) {
	raw := cliOption(args, name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}

func ptrConfig(cfg config.Config) *config.Config { return &cfg }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func cliBinding(value string) (network.BindingMode, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "loopback", "local", "localhost":
		return network.ModeLoopback, ""
	case "wifi", "wi-fi", "lan", "wifi/lan":
		return network.ModeWiFiLAN, ""
	case "ethernet", "wired", "usb", "thunderbolt":
		return network.ModeEthernet, ""
	case "vpn", "wireguard", "utun":
		return network.ModeVPN, ""
	case "public", "all":
		return network.ModePublic, ""
	case "interface", "exact":
		return network.ModeExact, ""
	default:
		return network.ModeExact, value
	}
}

func printHumanStatus(payload map[string]any) {
	status, _ := payload["status"].(runtimecontrol.Status)
	state := status.State
	fmt.Printf("version: %v (%v)\nweb: %s:%d healthy=%v\napi: %s:%d healthy=%v\nmode: %s interface=%s ipv4=%s\nuptime: %ds\ncaddy: enabled=%v listening=%v publicHealthy=%v\n",
		payload["version"], payload["build"], state.WebBindAddress, state.WebPort, status.WebHealthy,
		state.APIBindAddress, state.APIPort, status.APIHealthy, state.SelectedMode, state.SelectedInterface,
		state.SelectedIPv4, status.UptimeSeconds, status.Caddy.Enabled, status.Caddy.Listening, status.Caddy.PublicHealthy)
	if status.LastError != "" {
		fmt.Printf("last error: %s\n", status.LastError)
	}
}

func writeJSONAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".sonder-export-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func logConfigSummary(logger *log.Logger, cfg *config.Config, state network.RuntimeState) {
	tokenState := "none"
	if cfg.PairingToken != "" {
		tokenState = "configured (never logged)"
	}
	logger.Printf("config: web=%s:%d api=%s:%d mode=%s interface=%s dataDir=%s allowLAN=%v safeScan=%v token=%s theme=%s hwaccel=%s maxTranscode=%d libraries=%d caddy=%v",
		state.WebBindAddress, state.WebPort, state.APIBindAddress, state.APIPort,
		state.SelectedMode, state.SelectedInterface, cfg.DataDir, cfg.AllowLAN, cfg.SafeScan,
		tokenState, cfg.ThemePreset, cfg.Transcode.HWAccel, cfg.Transcode.MaxConcurrent,
		len(cfg.Libraries), state.CaddyEnabled)
	for _, lib := range cfg.Libraries {
		logger.Printf("  library %q kind=%s path=%s", lib.Name, lib.Kind, lib.Path)
	}
}

func loadRuntimeState(path string, cfg *config.Config, interfaces []network.Interface) (network.RuntimeState, error) {
	state, err := network.LoadState(path)
	if err != nil && !os.IsNotExist(err) {
		return network.RuntimeState{}, fmt.Errorf("load runtime state: %w", err)
	}
	if os.IsNotExist(err) {
		state = network.DefaultState(cfg.WebPort, cfg.APIPort, cfg.AllowLAN, interfaces)
	} else {
		network.Normalize(&state, cfg.WebPort, cfg.APIPort)
		if resolveErr := network.ResolveAndApply(&state, interfaces); resolveErr != nil {
			// A laptop may resume with a different adapter order or without its
			// VPN. Do not bind a stale address; fail safe to loopback and retain
			// the reason in runtime state for the menu bar/CLI.
			previous := state.SelectedInterface
			state = network.DefaultState(cfg.WebPort, cfg.APIPort, false, interfaces)
			state.LastError = fmt.Sprintf("saved interface %q is unavailable: %v; fell back to loopback", previous, resolveErr)
		}
	}
	network.Normalize(&state, cfg.WebPort, cfg.APIPort)
	if state.WebPort <= 0 {
		state.WebPort = cfg.WebPort
	}
	if state.APIPort <= 0 {
		state.APIPort = cfg.APIPort
	}
	if state.APIBindAddress == "" {
		state.APIBindAddress = "127.0.0.1"
	}
	if state.CaddyBindAddress == "" {
		state.CaddyBindAddress = "0.0.0.0"
	}
	state.ProcessStartTime = time.Now().UTC()
	return state, nil
}

// generateToken returns a 128-bit hex pairing token.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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
