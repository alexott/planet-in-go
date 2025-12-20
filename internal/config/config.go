package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
)

// Config represents the complete planet configuration
type Config struct {
	Planet    PlanetConfig
	Feeds     []FeedConfig
	Templates map[string]TemplateConfig
}

// PlanetConfig holds global planet settings
type PlanetConfig struct {
	Name                string
	Link                string
	OwnerName           string
	OwnerEmail          string
	CacheDirectory      string
	OutputDir           string
	LogLevel            string
	FeedTimeout         int
	NewFeedItems        int
	ItemsPerPage        int
	DaysPerPage         int
	DateFormat          string
	NewDateFormat       string
	Encoding            string
	TemplateFiles       []string
	Filter              string
	Exclude             string
	PostToTwitter       bool
	TwitterTrackingFile string
	FetchMode           string // "parallel" or "sequential" (default: "parallel")
	ParallelWorkers     int    // Number of parallel workers (default: 10)
}

// FeedConfig represents a single feed subscription
type FeedConfig struct {
	URL   string
	Name  string
	Extra map[string]string
}

// TwitterHandle returns the Twitter handle for attribution, or empty string if not set
func (f *FeedConfig) TwitterHandle() string {
	if handle, ok := f.Extra["twitter"]; ok {
		return handle
	}
	return ""
}

// Filter returns the feed-level filter pattern, or empty string if not set
func (f *FeedConfig) Filter() string {
	if filter, ok := f.Extra["filter"]; ok {
		return filter
	}
	return ""
}

// Exclude returns the feed-level exclude pattern, or empty string if not set
func (f *FeedConfig) Exclude() string {
	if exclude, ok := f.Extra["exclude"]; ok {
		return exclude
	}
	return ""
}

// TemplateConfig holds per-template settings
type TemplateConfig struct {
	DaysPerPage int
}

// Load reads and parses the config file
// All relative paths in the config are resolved relative to the current working directory (project root),
// matching the Python version behavior
func Load(path string) (*Config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load ini file: %w", err)
	}

	config := &Config{
		Feeds:     make([]FeedConfig, 0),
		Templates: make(map[string]TemplateConfig),
	}

	// Parse [Planet] section
	if err := parsePlanetSection(cfg, config); err != nil {
		return nil, fmt.Errorf("parse planet section: %w", err)
	}

	// Parse feed sections
	if err := parseFeedSections(cfg, config); err != nil {
		return nil, fmt.Errorf("parse feed sections: %w", err)
	}

	// Resolve template-specific sections
	if err := parseTemplateSections(cfg, config); err != nil {
		return nil, fmt.Errorf("parse template sections: %w", err)
	}

	return config, nil
}

func parsePlanetSection(iniFile *ini.File, config *Config) error {
	section := iniFile.Section("Planet")

	// Read raw strftime-style formats from config and convert to Go layouts
	rawDate := section.Key("date_format").MustString("%B %d, %Y %I:%M %p")
	rawNewDate := section.Key("new_date_format").MustString("%B %d, %Y")

	// Get current working directory for resolving relative paths
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Read directory paths and resolve relative to CWD (project root)
	cacheDir := section.Key("cache_directory").String()
	if cacheDir != "" && !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(cwd, cacheDir)
	}

	outputDir := section.Key("output_dir").String()
	if outputDir != "" && !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(cwd, outputDir)
	}

	twitterTrackingFile := section.Key("twitter_tracking_file").MustString("twitter_posted.json")
	if !filepath.IsAbs(twitterTrackingFile) {
		twitterTrackingFile = filepath.Join(cwd, twitterTrackingFile)
	}

	config.Planet = PlanetConfig{
		Name:                section.Key("name").String(),
		Link:                section.Key("link").String(),
		OwnerName:           section.Key("owner_name").String(),
		OwnerEmail:          section.Key("owner_email").String(),
		CacheDirectory:      cacheDir,
		OutputDir:           outputDir,
		LogLevel:            section.Key("log_level").MustString("INFO"),
		FeedTimeout:         section.Key("feed_timeout").MustInt(20),
		NewFeedItems:        section.Key("new_feed_items").MustInt(10),
		ItemsPerPage:        section.Key("items_per_page").MustInt(15),
		DaysPerPage:         section.Key("days_per_page").MustInt(0),
		DateFormat:          strftimeToGoLayout(rawDate),
		NewDateFormat:       strftimeToGoLayout(rawNewDate),
		Encoding:            section.Key("encoding").MustString("utf-8"),
		Filter:              section.Key("filter").String(),
		Exclude:             section.Key("exclude").String(),
		PostToTwitter:       section.Key("post_to_twitter").MustBool(false),
		TwitterTrackingFile: twitterTrackingFile,
		FetchMode:           section.Key("fetch_mode").MustString("parallel"),
		ParallelWorkers:     section.Key("parallel_workers").MustInt(10),
	}

	// Parse template_files (space-separated) and resolve paths relative to CWD
	templateFiles := section.Key("template_files").String()
	if templateFiles != "" {
		rawTemplates := strings.Fields(templateFiles)
		config.Planet.TemplateFiles = make([]string, len(rawTemplates))
		for i, tmpl := range rawTemplates {
			if !filepath.IsAbs(tmpl) {
				config.Planet.TemplateFiles[i] = filepath.Join(cwd, tmpl)
			} else {
				config.Planet.TemplateFiles[i] = tmpl
			}
		}
	}

	return nil
}

func parseFeedSections(iniFile *ini.File, config *Config) error {
	for _, section := range iniFile.Sections() {
		name := section.Name()

		// Skip special sections
		if name == "DEFAULT" || name == "Planet" || name == "" {
			continue
		}

		// Check if it's a feed URL (starts with http)
		if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
			feed := FeedConfig{
				URL:   name,
				Name:  section.Key("name").String(),
				Extra: make(map[string]string),
			}

			// Collect extra fields
			for _, key := range section.Keys() {
				keyName := key.Name()
				if keyName != "name" {
					feed.Extra[keyName] = key.String()
				}
			}

			config.Feeds = append(config.Feeds, feed)
		}
	}

	return nil
}

func parseTemplateSections(iniFile *ini.File, config *Config) error {
	// Get current working directory for resolving relative paths
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	for _, section := range iniFile.Sections() {
		name := section.Name()

		// Skip special sections and feed URLs
		if name == "DEFAULT" || name == "Planet" || name == "" {
			continue
		}
		if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
			continue
		}

		// This is a template-specific section
		// Resolve the template name relative to CWD to match with loaded templates
		templateName := name
		if !filepath.IsAbs(templateName) {
			templateName = filepath.Join(cwd, templateName)
		}

		templateConfig := TemplateConfig{
			DaysPerPage: section.Key("days_per_page").MustInt(0),
		}

		config.Templates[templateName] = templateConfig
	}

	return nil
}
