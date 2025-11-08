package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	"github.com/alexey-ott/planet-go/internal/fetcher"
	"github.com/alexey-ott/planet-go/internal/filter"
	"github.com/alexey-ott/planet-go/internal/renderer"
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
  run      Fetch feeds and render templates (default)
  fetch    Fetch feeds and update cache only
  render   Render templates from cache only
  version  Show version information

Options:
  -c string
        path to config file (default "config.ini")
  -debug
        enable debug logging (overrides config log_level)

Examples:
  planet -c config.ini                # Run (fetch + render) with config
  planet run -c config.ini -debug     # Run with debug logging
  planet fetch -c config.ini          # Only fetch and cache feeds
  planet render -c config.ini         # Only render from cache
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

// runFetchAndRender implements the "run" command - fetch and render
func runFetchAndRender(configPath string, debugMode bool) error {
	cfg, err := loadConfig(configPath, debugMode)
	if err != nil {
		return err
	}

	slog.Info("starting planet (run: fetch + render)",
		"version", version,
		"feeds", len(cfg.Feeds))

	// Ensure directories exist
	slog.Debug("creating directories",
		"cache_dir", cfg.Planet.CacheDirectory,
		"output_dir", cfg.Planet.OutputDir)
	
	if err := os.MkdirAll(cfg.Planet.CacheDirectory, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	if err := os.MkdirAll(cfg.Planet.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Initialize components
	slog.Debug("initializing components",
		"cache_dir", cfg.Planet.CacheDirectory,
		"timeout", cfg.Planet.FeedTimeout)
	
	cache := cache.New(cfg.Planet.CacheDirectory)
	fetcher := fetcher.NewSequential(cfg.Planet.FeedTimeout, cache)

	// Fetch feeds
	slog.Info("fetching feeds", "count", len(cfg.Feeds))
	
	// Show first few feeds at INFO level so user can verify correct config
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
	
	ctx := context.Background()
	fetchStart := time.Now()
	results := fetcher.FetchFeeds(ctx, cfg.Feeds)
	fetchDuration := time.Since(fetchStart)

	// Log results
	var successCount, errorCount, cachedCount int
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
		"duration", fetchDuration)

	// Load all cached entries
	slog.Debug("loading all cached entries")
	loadStart := time.Now()
	entries, err := cache.LoadAll()
	loadDuration := time.Since(loadStart)
	
	if err != nil {
		return fmt.Errorf("load cached entries: %w", err)
	}

	slog.Info("loaded entries",
		"count", len(entries),
		"duration", loadDuration)

	// Apply filters
	slog.Debug("creating filter",
		"include_pattern", cfg.Planet.Filter,
		"exclude_pattern", cfg.Planet.Exclude)
	
	filter, err := filter.New(cfg.Planet.Filter, cfg.Planet.Exclude)
	if err != nil {
		return fmt.Errorf("create filter: %w", err)
	}

	slog.Debug("applying filters")
	filterStart := time.Now()
	filtered := filter.Apply(entries)
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

	// Render templates
	renderer := renderer.New(cfg.Planet.OutputDir)

	slog.Info("rendering templates", "count", len(cfg.Planet.TemplateFiles))

	// Get config directory for template paths
	configDir := filepath.Dir(configPath)
	slog.Debug("config directory", "path", configDir)

	renderStart := time.Now()
	successTemplates := 0
	for i, tmplFile := range cfg.Planet.TemplateFiles {
		// Resolve template path relative to config file
		tmplPath := tmplFile
		if !filepath.IsAbs(tmplPath) {
			tmplPath = filepath.Join(configDir, tmplPath)
		}

		slog.Debug("rendering template",
			"index", i+1,
			"file", tmplFile,
			"path", tmplPath)
		
		tmplStart := time.Now()
		if err := renderer.Render(tmplPath, filtered, cfg); err != nil {
			slog.Error("template failed",
				"file", tmplFile,
				"error", err,
				"duration", time.Since(tmplStart))
		} else {
			successTemplates++
			slog.Info("template rendered",
				"file", tmplFile,
				"duration", time.Since(tmplStart))
		}
	}
	renderDuration := time.Since(renderStart)

	totalDuration := time.Since(fetchStart)
	slog.Info("planet run complete",
		"entries", len(filtered),
		"templates", successTemplates,
		"total_duration", totalDuration,
		"fetch_duration", fetchDuration,
		"render_duration", renderDuration)
	
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

	// Ensure cache directory exists
	slog.Debug("creating cache directory", "cache_dir", cfg.Planet.CacheDirectory)
	
	if err := os.MkdirAll(cfg.Planet.CacheDirectory, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	// Initialize components
	slog.Debug("initializing components",
		"cache_dir", cfg.Planet.CacheDirectory,
		"timeout", cfg.Planet.FeedTimeout)
	
	cache := cache.New(cfg.Planet.CacheDirectory)
	fetcher := fetcher.NewSequential(cfg.Planet.FeedTimeout, cache)

	// Fetch feeds
	slog.Info("fetching feeds", "count", len(cfg.Feeds))
	
	// Show first few feeds at INFO level
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
	
	ctx := context.Background()
	fetchStart := time.Now()
	results := fetcher.FetchFeeds(ctx, cfg.Feeds)
	fetchDuration := time.Since(fetchStart)

	// Log results
	var successCount, errorCount, cachedCount int
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
		"duration", fetchDuration)

	return nil
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

	// Ensure output directory exists
	slog.Debug("creating output directory", "output_dir", cfg.Planet.OutputDir)
	
	if err := os.MkdirAll(cfg.Planet.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Initialize cache
	cache := cache.New(cfg.Planet.CacheDirectory)

	// Load all cached entries
	slog.Debug("loading all cached entries")
	loadStart := time.Now()
	entries, err := cache.LoadAll()
	loadDuration := time.Since(loadStart)
	
	if err != nil {
		return fmt.Errorf("load cached entries: %w", err)
	}

	if len(entries) == 0 {
		slog.Warn("no cached entries found - run 'planet fetch' first")
		return nil
	}

	slog.Info("loaded entries",
		"count", len(entries),
		"duration", loadDuration)

	// Apply filters
	slog.Debug("creating filter",
		"include_pattern", cfg.Planet.Filter,
		"exclude_pattern", cfg.Planet.Exclude)
	
	filter, err := filter.New(cfg.Planet.Filter, cfg.Planet.Exclude)
	if err != nil {
		return fmt.Errorf("create filter: %w", err)
	}

	slog.Debug("applying filters")
	filterStart := time.Now()
	filtered := filter.Apply(entries)
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

	// Render templates
	renderer := renderer.New(cfg.Planet.OutputDir)

	slog.Info("rendering templates", "count", len(cfg.Planet.TemplateFiles))

	// Get config directory for template paths
	configDir := filepath.Dir(configPath)
	slog.Debug("config directory", "path", configDir)

	renderStart := time.Now()
	successTemplates := 0
	for i, tmplFile := range cfg.Planet.TemplateFiles {
		// Resolve template path relative to config file
		tmplPath := tmplFile
		if !filepath.IsAbs(tmplPath) {
			tmplPath = filepath.Join(configDir, tmplPath)
		}

		slog.Debug("rendering template",
			"index", i+1,
			"file", tmplFile,
			"path", tmplPath)
		
		tmplStart := time.Now()
		if err := renderer.Render(tmplPath, filtered, cfg); err != nil {
			slog.Error("template failed",
				"file", tmplFile,
				"error", err,
				"duration", time.Since(tmplStart))
		} else {
			successTemplates++
			slog.Info("template rendered",
				"file", tmplFile,
				"duration", time.Since(tmplStart))
		}
	}
	renderDuration := time.Since(renderStart)

	slog.Info("render complete",
		"entries", len(filtered),
		"templates", successTemplates,
		"duration", renderDuration)
	
	return nil
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
