package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	"github.com/alexey-ott/planet-go/internal/fetcher"
	"github.com/alexey-ott/planet-go/internal/filter"
	"github.com/alexey-ott/planet-go/internal/renderer"
)

const version = "0.1.0"

func main() {
	configPath := flag.String("c", "config.ini", "path to config file")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("planet-go version %s\n", version)
		return
	}

	if err := run(*configPath); err != nil {
		slog.Error("failed to run", "error", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Setup logging
	logLevel := parseLogLevel(cfg.Planet.LogLevel)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("starting planet", "feeds", len(cfg.Feeds))

	// Ensure directories exist
	if err := os.MkdirAll(cfg.Planet.CacheDirectory, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	if err := os.MkdirAll(cfg.Planet.OutputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Initialize components
	cache := cache.New(cfg.Planet.CacheDirectory)
	fetcher := fetcher.NewSequential(cfg.Planet.FeedTimeout, cache)

	// Fetch feeds
	slog.Info("fetching feeds")
	ctx := context.Background()
	results := fetcher.FetchFeeds(ctx, cfg.Feeds)

	// Log results
	var successCount, errorCount int
	for _, result := range results {
		if result.Error != nil {
			errorCount++
			slog.Error("feed failed", "url", result.URL, "error", result.Error)
		} else {
			successCount++
			if result.Cached {
				slog.Debug("feed cached", "url", result.URL, "entries", len(result.Entries))
			} else {
				slog.Info("feed fetched", "url", result.URL, "entries", len(result.Entries))
			}
		}
	}

	slog.Info("fetch complete", "success", successCount, "errors", errorCount)

	// Load all cached entries
	entries, err := cache.LoadAll()
	if err != nil {
		return fmt.Errorf("load cached entries: %w", err)
	}

	slog.Info("loaded entries", "count", len(entries))

	// Apply filters
	filter, err := filter.New(cfg.Planet.Filter, cfg.Planet.Exclude)
	if err != nil {
		return fmt.Errorf("create filter: %w", err)
	}

	filtered := filter.Apply(entries)
	if len(filtered) != len(entries) {
		slog.Info("filtered entries", "before", len(entries), "after", len(filtered))
	}

	// Render templates
	renderer := renderer.New(cfg.Planet.OutputDir)

	slog.Info("rendering templates", "count", len(cfg.Planet.TemplateFiles))

	// Get config directory for template paths
	configDir := filepath.Dir(configPath)

	for _, tmplFile := range cfg.Planet.TemplateFiles {
		// Resolve template path relative to config file
		tmplPath := tmplFile
		if !filepath.IsAbs(tmplPath) {
			tmplPath = filepath.Join(configDir, tmplPath)
		}

		slog.Debug("rendering template", "file", tmplFile)
		if err := renderer.Render(tmplPath, filtered, cfg); err != nil {
			slog.Error("template failed", "file", tmplFile, "error", err)
		} else {
			slog.Info("template rendered", "file", tmplFile)
		}
	}

	slog.Info("done", "entries", len(filtered))
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
