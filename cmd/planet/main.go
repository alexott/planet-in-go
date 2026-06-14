package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	"github.com/alexey-ott/planet-go/internal/fetcher"
	"github.com/alexey-ott/planet-go/internal/filter"
	mastodonposter "github.com/alexey-ott/planet-go/internal/mastodon"
	"github.com/alexey-ott/planet-go/internal/renderer"
	"github.com/alexey-ott/planet-go/internal/twitter"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		// No subcommand provided, default to "run"
		runCommand(os.Args)
		return
	}

	// Check for subcommands
	switch os.Args[1] {
	case "run":
		runCommand(os.Args[1:])
	case "fetch":
		fetchCommand(os.Args[1:])
	case "render":
		renderCommand(os.Args[1:])
	case "post":
		postCommand(os.Args[1:])
	case "cleanup-mastodon":
		cleanupMastodonCommand(os.Args[1:])
	case "version":
		versionCommand()
	case "-version", "--version":
		versionCommand()
	case "-h", "-help", "--help":
		printUsage()
	default:
		// If it starts with a dash, treat as flag for default "run" command
		if strings.HasPrefix(os.Args[1], "-") {
			runCommand(os.Args)
		} else {
			fmt.Fprintf(os.Stderr, "Error: unknown command %q\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Planet Go - Feed Aggregator

Usage:
  planet [command] [options]

Commands:
  run      Fetch feeds, render templates, and post to enabled networks (default)
  fetch    Fetch feeds and update cache only (no posting)
  render   Render templates from cache only (no posting)
  post     Post new articles to enabled networks from cache
  cleanup-mastodon  Dry-run or delete incident-era Mastodon posts
  version  Show version information

Options:
  -c string
        path to config file (default "config.ini")
  -debug
        enable debug logging (overrides config log_level)

Examples:
  planet -c config.ini                # Run (fetch + render + post) with config
  planet run -c config.ini -debug     # Run with debug logging
  planet fetch -c config.ini          # Only fetch and cache feeds (no posting)
  planet render -c config.ini         # Only render from cache (no posting)
  planet post -c config.ini           # Only post to enabled networks from cache
  planet version                      # Show version

For more information, visit: https://github.com/alexey-ott/planet-go
`)
}

func versionCommand() {
	fmt.Printf("planet-go version %s\n", version)
}

func runCommand(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")

	fs.Parse(args[1:])

	if err := runFetchAndRender(*configPath, *debugMode); err != nil {
		slog.Error("failed to run", "error", err)
		os.Exit(1)
	}
}

func fetchCommand(args []string) {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")

	fs.Parse(args[1:])

	if err := runFetch(*configPath, *debugMode); err != nil {
		slog.Error("failed to fetch", "error", err)
		os.Exit(1)
	}
}

func renderCommand(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")

	fs.Parse(args[1:])

	if err := runRender(*configPath, *debugMode); err != nil {
		slog.Error("failed to render", "error", err)
		os.Exit(1)
	}
}

// Common setup function
func setupLogging(cfg *config.Config, debugMode bool) {
	logLevel := parseLogLevel(cfg.Planet.LogLevel)
	if debugMode {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if debugMode {
		slog.Debug("debug mode enabled")
	}
}

// Common config loading function
func loadConfig(configPath string, debugMode bool) (*config.Config, error) {
	// Check if config file exists
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s (absolute path: %s)", configPath, absPath)
	}

	// Load configuration
	fmt.Printf("Loading configuration from: %s\n", absPath)
	slog.Info("loading configuration", "path", configPath, "absolute_path", absPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Setup logging after config is loaded
	setupLogging(cfg, debugMode)

	slog.Debug("configuration loaded successfully",
		"feeds_count", len(cfg.Feeds),
		"template_files", len(cfg.Planet.TemplateFiles))

	return cfg, nil
}

// fetchFeeds fetches all feeds and returns timing info
func fetchFeeds(cfg *config.Config, debugMode bool) (successCount, cachedCount, errorCount int, duration time.Duration, err error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cfg.Planet.CacheDirectory, 0755); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("create cache directory: %w", err)
	}

	// Initialize components
	cacheInstance := cache.New(cfg.Planet.CacheDirectory)

	// Select fetcher based on configuration
	var fetcherInstance fetcher.Fetcher
	if cfg.Planet.FetchMode == "sequential" {
		slog.Debug("initializing sequential fetcher",
			"cache_dir", cfg.Planet.CacheDirectory,
			"timeout", cfg.Planet.FeedTimeout)
		fetcherInstance = fetcher.NewSequential(cfg.Planet.FeedTimeout, cacheInstance, debugMode)
	} else {
		// Default to parallel mode
		slog.Debug("initializing parallel fetcher",
			"cache_dir", cfg.Planet.CacheDirectory,
			"timeout", cfg.Planet.FeedTimeout,
			"workers", cfg.Planet.ParallelWorkers)
		fetcherInstance = fetcher.NewParallel(cfg.Planet.FeedTimeout, cacheInstance, debugMode, cfg.Planet.ParallelWorkers)
	}

	// Log first few feeds at INFO level
	feedsToShow := 3
	if len(cfg.Feeds) < feedsToShow {
		feedsToShow = len(cfg.Feeds)
	}
	for i := 0; i < feedsToShow; i++ {
		feed := cfg.Feeds[i]
		slog.Info("feed",
			"index", i+1,
			"url", feed.URL,
			"name", feed.Name)
	}
	if len(cfg.Feeds) > feedsToShow {
		slog.Info("... and more feeds", "total", len(cfg.Feeds))
	}

	// Log all feeds in debug mode
	for i, feed := range cfg.Feeds {
		slog.Debug("feed configuration",
			"index", i+1,
			"url", feed.URL,
			"name", feed.Name)
	}

	// Fetch feeds
	slog.Info("fetching feeds", "count", len(cfg.Feeds))
	ctx := context.Background()
	fetchStart := time.Now()
	results := fetcherInstance.FetchFeeds(ctx, cfg.Feeds)
	duration = time.Since(fetchStart)

	// Process results
	for _, result := range results {
		if result.Error != nil {
			errorCount++
			slog.Error("feed failed", "url", result.URL, "error", result.Error)
		} else {
			successCount++
			if result.Cached {
				cachedCount++
				slog.Debug("feed cached", "url", result.URL, "entries", len(result.Entries))
			} else {
				slog.Info("feed fetched", "url", result.URL, "entries", len(result.Entries))
			}
		}
	}

	slog.Info("fetch complete",
		"success", successCount,
		"cached", cachedCount,
		"errors", errorCount,
		"duration", duration)

	return successCount, cachedCount, errorCount, duration, nil
}

// loadAndFilterEntries loads all cached entries and applies per-feed filters
func loadAndFilterEntries(cfg *config.Config) ([]cache.Entry, error) {
	cacheInstance := cache.New(cfg.Planet.CacheDirectory)

	// Load all cached entries
	slog.Debug("loading all cached entries")
	loadStart := time.Now()
	entries, err := cacheInstance.LoadAll()
	loadDuration := time.Since(loadStart)

	if err != nil {
		return nil, fmt.Errorf("load cached entries: %w", err)
	}

	if len(entries) == 0 {
		slog.Warn("no cached entries found")
		return entries, nil
	}

	slog.Info("loaded entries",
		"count", len(entries),
		"duration", loadDuration)

	// Apply per-feed filters
	slog.Debug("applying per-feed filters",
		"total_entries", len(entries),
		"global_include", cfg.Planet.Filter,
		"global_exclude", cfg.Planet.Exclude,
		"feeds_count", len(cfg.Feeds))

	filterStart := time.Now()
	filtered, err := filter.ApplyPerFeed(entries, cfg.Feeds, cfg.Planet.Filter, cfg.Planet.Exclude)
	if err != nil {
		return nil, fmt.Errorf("apply filters: %w", err)
	}
	filterDuration := time.Since(filterStart)

	if len(filtered) != len(entries) {
		slog.Info("filtered entries",
			"before", len(entries),
			"after", len(filtered),
			"removed", len(entries)-len(filtered),
			"duration", filterDuration)
	} else {
		slog.Debug("no entries filtered",
			"count", len(entries),
			"duration", filterDuration)
	}

	return filtered, nil
}

// limitEntries returns the most recent N entries, sorted by date (newest first)
// If maxEntries is 0 or negative, all entries are returned
func limitEntries(entries []cache.Entry, maxEntries int) []cache.Entry {
	if maxEntries <= 0 || len(entries) <= maxEntries {
		return sortEntriesByDate(entries)
	}

	sorted := sortEntriesByDate(entries)
	limited := sorted[:maxEntries]

	slog.Debug("limited entries",
		"before", len(entries),
		"after", len(limited),
		"max", maxEntries)

	return limited
}

// sortEntriesByDate sorts entries by date, newest first
func sortEntriesByDate(entries []cache.Entry) []cache.Entry {
	sorted := make([]cache.Entry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.After(sorted[j].Date)
	})

	return sorted
}

// renderTemplates renders all configured templates
func renderTemplates(cfg *config.Config, entries []cache.Entry, configPath string) (int, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(cfg.Planet.OutputDir, 0755); err != nil {
		return 0, fmt.Errorf("create output directory: %w", err)
	}

	rendererInstance := renderer.New(cfg.Planet.OutputDir)

	// Copy static files - look for static directory in same location as first template
	if len(cfg.Planet.TemplateFiles) > 0 {
		// Get directory of first template file
		templateDir := filepath.Dir(cfg.Planet.TemplateFiles[0])
		staticSourceDir := filepath.Join(templateDir, "static")

		slog.Debug("checking for static directory", "path", staticSourceDir)

		if err := rendererInstance.CopyStaticFiles(staticSourceDir); err != nil {
			slog.Warn("failed to copy static files (non-fatal)", "error", err)
		} else if _, err := os.Stat(staticSourceDir); err == nil {
			slog.Info("static files copied", "from", staticSourceDir, "to", filepath.Join(cfg.Planet.OutputDir, "static"))
		}
	}

	slog.Info("rendering templates", "count", len(cfg.Planet.TemplateFiles))

	renderStart := time.Now()
	successTemplates := 0

	for i, tmplPath := range cfg.Planet.TemplateFiles {
		// Template paths are already resolved to absolute paths by config loading
		slog.Debug("rendering template",
			"index", i+1,
			"path", tmplPath)

		tmplStart := time.Now()
		if err := rendererInstance.Render(tmplPath, entries, cfg); err != nil {
			slog.Error("template failed",
				"path", tmplPath,
				"error", err,
				"duration", time.Since(tmplStart))
		} else {
			successTemplates++
			slog.Info("template rendered",
				"path", tmplPath,
				"duration", time.Since(tmplStart))
		}
	}
	renderDuration := time.Since(renderStart)

	slog.Info("render complete",
		"entries", len(entries),
		"templates", successTemplates,
		"duration", renderDuration)

	return successTemplates, nil
}

// runFetchAndRender implements the "run" command - fetch and render
func runFetchAndRender(configPath string, debugMode bool) error {
	startTime := time.Now()
	cfg, err := loadConfig(configPath, debugMode)
	if err != nil {
		return err
	}

	slog.Info("starting planet (run: fetch + render + post)",
		"version", version,
		"feeds", len(cfg.Feeds))

	// Run fetch
	if err := doFetch(cfg, debugMode); err != nil {
		return err
	}

	// Run render
	filtered, err := doRender(cfg, configPath)
	if err != nil {
		return err
	}

	totalDuration := time.Since(startTime)
	slog.Info("planet run complete",
		"entries", len(filtered),
		"total_duration", totalDuration)

	twitterEnabled, mastodonEnabled := postingTargets(cfg)

	if twitterEnabled {
		slog.Info("Twitter posting enabled, posting new articles")
		if err := postToTwitter(cfg, filtered); err != nil {
			// Log error but don't fail the entire command
			slog.Error("Twitter posting failed", "error", err)
		}
	} else {
		slog.Debug("Twitter posting disabled in configuration")
	}

	if mastodonEnabled {
		slog.Info("Mastodon posting enabled, posting new articles")
		if err := postToMastodon(cfg, filtered); err != nil {
			slog.Error("Mastodon posting failed", "error", err)
		}
	} else {
		slog.Debug("Mastodon posting disabled in configuration")
	}

	return nil
}

// doFetch performs the fetch operation (internal, used by commands)
func doFetch(cfg *config.Config, debugMode bool) error {
	successCount, cachedCount, errorCount, _, err := fetchFeeds(cfg, debugMode)
	if err != nil {
		return fmt.Errorf("fetch feeds: %w", err)
	}

	slog.Info("fetch command complete",
		"success", successCount,
		"cached", cachedCount,
		"errors", errorCount)

	return nil
}

// runFetch implements the "fetch" command - fetch feeds and update cache only
func runFetch(configPath string, debugMode bool) error {
	cfg, err := loadConfig(configPath, debugMode)
	if err != nil {
		return err
	}

	slog.Info("starting planet (fetch only)",
		"version", version,
		"feeds", len(cfg.Feeds))

	return doFetch(cfg, debugMode)
}

// doRender performs the render operation (internal, used by commands)
// Returns the filtered entries for use by calling code (e.g., Twitter posting)
func doRender(cfg *config.Config, configPath string) ([]cache.Entry, error) {
	// Load and filter entries
	filtered, err := loadAndFilterEntries(cfg)
	if err != nil {
		return nil, fmt.Errorf("load and filter entries: %w", err)
	}

	if len(filtered) == 0 {
		slog.Warn("no cached entries found - run 'planet fetch' first")
		return filtered, nil
	}

	// Render templates
	successTemplates, err := renderTemplates(cfg, filtered, configPath)
	if err != nil {
		return nil, fmt.Errorf("render templates: %w", err)
	}

	slog.Info("render command complete",
		"entries", len(filtered),
		"templates", successTemplates)

	return filtered, nil
}

// runRender implements the "render" command - render templates from cache only
func runRender(configPath string, debugMode bool) error {
	cfg, err := loadConfig(configPath, debugMode)
	if err != nil {
		return err
	}

	slog.Info("starting planet (render only)",
		"version", version,
		"templates", len(cfg.Planet.TemplateFiles))

	_, err = doRender(cfg, configPath)
	return err
}

// postCommand implements the "post" command - post to enabled networks from cache only
func postCommand(args []string) {
	fs := flag.NewFlagSet("post", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")

	fs.Parse(args[1:])

	if err := runPost(*configPath, *debugMode); err != nil {
		slog.Error("failed to post to enabled networks", "error", err)
		os.Exit(1)
	}
}

// runPost implements the "post" command - post to enabled networks from cache only
func runPost(configPath string, debugMode bool) error {
	cfg, err := loadConfig(configPath, debugMode)
	if err != nil {
		return err
	}

	slog.Info("starting planet (post to enabled networks)",
		"version", version)

	twitterEnabled, mastodonEnabled := postingTargets(cfg)
	if !twitterEnabled && !mastodonEnabled {
		slog.Warn("posting is disabled in configuration")
		fmt.Println("Posting is disabled. Enable one or both in your config.ini:")
		fmt.Println("  [Planet]")
		fmt.Println("  post_to_twitter = true")
		fmt.Println("  post_to_mastodon = true")
		return nil
	}

	// Load and filter entries
	filtered, err := loadAndFilterEntries(cfg)
	if err != nil {
		return fmt.Errorf("load and filter entries: %w", err)
	}

	if len(filtered) == 0 {
		slog.Warn("no cached entries found - run 'planet fetch' first")
		fmt.Println("No cached entries found. Run 'planet fetch' first to cache articles.")
		return nil
	}

	postStart := time.Now()
	var postErrs []error

	if twitterEnabled {
		twitterEntries := prepareTwitterPostEntries(filtered)
		slog.Debug("limited entries for Twitter posting", "count", len(twitterEntries), "max", 10)
		slog.Info("posting to Twitter", "entries", len(twitterEntries))
		if err := postToTwitter(cfg, twitterEntries); err != nil {
			postErrs = append(postErrs, fmt.Errorf("post to Twitter: %w", err))
		}
	}

	if mastodonEnabled {
		mastodonEntries := prepareMastodonPostEntries(filtered)
		slog.Info("posting to Mastodon", "entries", len(mastodonEntries))
		if err := postToMastodon(cfg, mastodonEntries); err != nil {
			postErrs = append(postErrs, fmt.Errorf("post to Mastodon: %w", err))
		}
	}

	postDuration := time.Since(postStart)

	slog.Info("post command complete",
		"entries", len(filtered),
		"duration", postDuration)

	return errors.Join(postErrs...)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARNING", "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type articlePoster interface {
	PostNewArticles(entries []cache.Entry, feedConfigs []config.FeedConfig, maxInitial int) error
}

type mastodonCleanupRunner interface {
	Cleanup(ctx context.Context, opts mastodonposter.CleanupOptions) (mastodonposter.CleanupResult, error)
}

var newTwitterPoster = func(trackingFile string) (articlePoster, error) {
	return twitter.NewPoster(trackingFile)
}

var newMastodonPoster = func(trackingFile string) (articlePoster, error) {
	return mastodonposter.NewPoster(trackingFile)
}

var newMastodonCleaner = func(cacheDir string) (mastodonCleanupRunner, error) {
	return mastodonposter.NewCleaner(cacheDir)
}

func postingTargets(cfg *config.Config) (bool, bool) {
	return cfg.Planet.PostToTwitter, cfg.Planet.PostToMastodon
}

func resolveTrackingFile(cacheDir, trackingFile string) string {
	if filepath.IsAbs(trackingFile) {
		return trackingFile
	}
	return filepath.Join(cacheDir, trackingFile)
}

func prepareTwitterPostEntries(entries []cache.Entry) []cache.Entry {
	return limitEntries(entries, 10)
}

func prepareMastodonPostEntries(entries []cache.Entry) []cache.Entry {
	return sortEntriesByDate(entries)
}

func defaultCleanupMastodonOptions() mastodonposter.CleanupOptions {
	return mastodonposter.CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
}

func cleanupMastodonCommand(args []string) {
	fs := flag.NewFlagSet("cleanup-mastodon", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")
	opts, err := parseCleanupMastodonFlags(fs, args[1:])
	if err != nil {
		slog.Error("failed to parse cleanup-mastodon flags", "error", err)
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath, *debugMode)
	if err != nil {
		slog.Error("failed to load config for Mastodon cleanup", "error", err)
		os.Exit(1)
	}

	if err := runCleanupMastodon(cfg, opts); err != nil {
		slog.Error("failed to cleanup Mastodon statuses", "error", err)
		os.Exit(1)
	}
}

func parseCleanupMastodonFlags(fs *flag.FlagSet, args []string) (mastodonposter.CleanupOptions, error) {
	opts := defaultCleanupMastodonOptions()
	applyMode := fs.Bool("apply", false, "actually delete matching statuses")
	fromValue := fs.String("from", opts.WindowStart.Format(time.RFC3339), "incident window start in RFC3339")
	toValue := fs.String("to", opts.WindowEndExclusive.Format(time.RFC3339), "incident window end (exclusive) in RFC3339")
	articleBeforeValue := fs.String("article-before", opts.ArticleBefore.Format(time.RFC3339), "delete only if article date is before this RFC3339 timestamp")

	if err := fs.Parse(args); err != nil {
		return mastodonposter.CleanupOptions{}, err
	}

	var err error
	opts.WindowStart, err = time.Parse(time.RFC3339, *fromValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -from: %w", err)
	}
	opts.WindowEndExclusive, err = time.Parse(time.RFC3339, *toValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -to: %w", err)
	}
	opts.ArticleBefore, err = time.Parse(time.RFC3339, *articleBeforeValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -article-before: %w", err)
	}
	opts.Apply = *applyMode

	return opts, nil
}

func runCleanupMastodon(cfg *config.Config, opts mastodonposter.CleanupOptions) error {
	cleaner, err := newMastodonCleaner(cfg.Planet.CacheDirectory)
	if err != nil {
		return fmt.Errorf("create Mastodon cleaner: %w", err)
	}

	result, err := cleaner.Cleanup(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("cleanup Mastodon statuses: %w", err)
	}

	slog.Info("Mastodon cleanup complete",
		"examined", result.Examined,
		"in_window", result.InWindow,
		"matched", result.Matched,
		"deleted", result.Deleted,
		"apply", opts.Apply)

	return nil
}

// postToTwitter posts new articles to Twitter.
func postToTwitter(cfg *config.Config, entries []cache.Entry) error {
	trackingFile := resolveTrackingFile(cfg.Planet.CacheDirectory, cfg.Planet.TwitterTrackingFile)

	slog.Info("Initializing Twitter poster", "tracking_file", trackingFile)

	poster, err := newTwitterPoster(trackingFile)
	if err != nil {
		return fmt.Errorf("create Twitter poster: %w", err)
	}

	// Post new articles (max 5 on first run)
	maxInitial := 5
	if err := poster.PostNewArticles(entries, cfg.Feeds, maxInitial); err != nil {
		return fmt.Errorf("post to Twitter: %w", err)
	}

	return nil
}

// postToMastodon posts new articles to Mastodon.
func postToMastodon(cfg *config.Config, entries []cache.Entry) error {
	trackingFile := resolveTrackingFile(cfg.Planet.CacheDirectory, cfg.Planet.MastodonTrackingFile)

	slog.Info("Initializing Mastodon poster", "tracking_file", trackingFile)

	poster, err := newMastodonPoster(trackingFile)
	if err != nil {
		return fmt.Errorf("create Mastodon poster: %w", err)
	}

	const maxInitial = 50
	if err := poster.PostNewArticles(entries, cfg.Feeds, maxInitial); err != nil {
		return fmt.Errorf("post to Mastodon: %w", err)
	}

	return nil
}
