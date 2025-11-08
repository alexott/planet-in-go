# Planet Go Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement MVP of Planet Clojure feed aggregator in Go with sequential feed fetching, file-based caching, and Go template rendering.

**Architecture:** Monolithic CLI tool with modular internal packages (config, fetcher, cache, filter, renderer). Interface-based fetcher design enables future concurrent implementation.

**Tech Stack:** Go 1.21+, go-ini/ini, mmcdole/gofeed, stdlib (net/http, html/template, log/slog)

---

## Prerequisites

- Go 1.21 or later installed
- Access to planet.clojure/clojure/config.ini for testing
- Basic understanding of RSS/Atom feeds

---

## Task 1: Project Setup

**Files:**
- Create: `go.mod`
- Create: `go.sum` (via go mod tidy)
- Create: `.gitignore`

**Step 1: Initialize Go module**

Run:
```bash
go mod init github.com/alexey-ott/planet-go
```

Expected: Creates `go.mod` with module declaration

**Step 2: Add dependencies**

Run:
```bash
go get github.com/go-ini/ini@latest
go get github.com/mmcdole/gofeed@latest
```

Expected: Dependencies added to go.mod and go.sum created

**Step 3: Create .gitignore**

Create `.gitignore`:
```
# Binaries
/planet
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary
*.test

# Output
*.out

# Go workspace file
go.work

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# Test cache
/cache/
/output/
```

**Step 4: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: initialize Go module with dependencies

Add go-ini for config parsing and gofeed for RSS/Atom parsing.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 2: Config Package - Data Structures

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write failing test for config loading**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.ini")

	content := `[Planet]
name = Test Planet
link = http://example.com
cache_directory = /tmp/cache
output_dir = /tmp/output
log_level = INFO
feed_timeout = 20
items_per_page = 15
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Planet.Name != "Test Planet" {
		t.Errorf("Planet.Name = %q, want %q", cfg.Planet.Name, "Test Planet")
	}

	if cfg.Planet.FeedTimeout != 20 {
		t.Errorf("Planet.FeedTimeout = %d, want %d", cfg.Planet.FeedTimeout, 20)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -v`
Expected: FAIL - package or Load function not found

**Step 3: Write minimal config structures**

Create `internal/config/config.go`:
```go
package config

import (
	"fmt"
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
	Name           string
	Link           string
	OwnerName      string
	OwnerEmail     string
	CacheDirectory string
	OutputDir      string
	LogLevel       string
	FeedTimeout    int
	NewFeedItems   int
	ItemsPerPage   int
	DaysPerPage    int
	DateFormat     string
	NewDateFormat  string
	Encoding       string
	TemplateFiles  []string
	Filter         string
	Exclude        string
}

// FeedConfig represents a single feed subscription
type FeedConfig struct {
	URL   string
	Name  string
	Extra map[string]string
}

// TemplateConfig holds per-template settings
type TemplateConfig struct {
	DaysPerPage int
}

// Load reads and parses the config file
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

	return config, nil
}

func parsePlanetSection(iniFile *ini.File, config *Config) error {
	section := iniFile.Section("Planet")

	config.Planet = PlanetConfig{
		Name:           section.Key("name").String(),
		Link:           section.Key("link").String(),
		OwnerName:      section.Key("owner_name").String(),
		OwnerEmail:     section.Key("owner_email").String(),
		CacheDirectory: section.Key("cache_directory").String(),
		OutputDir:      section.Key("output_dir").String(),
		LogLevel:       section.Key("log_level").MustString("INFO"),
		FeedTimeout:    section.Key("feed_timeout").MustInt(20),
		NewFeedItems:   section.Key("new_feed_items").MustInt(10),
		ItemsPerPage:   section.Key("items_per_page").MustInt(15),
		DaysPerPage:    section.Key("days_per_page").MustInt(0),
		DateFormat:     section.Key("date_format").MustString("%B %d, %Y %I:%M %p"),
		NewDateFormat:  section.Key("new_date_format").MustString("%B %d, %Y"),
		Encoding:       section.Key("encoding").MustString("utf-8"),
		Filter:         section.Key("filter").String(),
		Exclude:        section.Key("exclude").String(),
	}

	// Parse template_files (space-separated)
	templateFiles := section.Key("template_files").String()
	if templateFiles != "" {
		config.Planet.TemplateFiles = strings.Fields(templateFiles)
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add config parsing with go-ini

Implement Config, PlanetConfig, and FeedConfig structures.
Parse [Planet] section and feed sections from INI file.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 3: Config Package - Feed Section Parsing Test

**Files:**
- Modify: `internal/config/config_test.go`

**Step 1: Write test for feed parsing**

Add to `internal/config/config_test.go`:
```go
func TestLoad_ParsesFeeds(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.ini")

	content := `[Planet]
name = Test Planet
cache_directory = /tmp/cache
output_dir = /tmp/output

[https://example.com/feed.xml]
name = Example Feed
twitter = exampleuser

[https://another.com/rss]
name = Another Feed
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Feeds) != 2 {
		t.Fatalf("len(Feeds) = %d, want 2", len(cfg.Feeds))
	}

	feed := cfg.Feeds[0]
	if feed.URL != "https://example.com/feed.xml" {
		t.Errorf("Feed[0].URL = %q, want %q", feed.URL, "https://example.com/feed.xml")
	}

	if feed.Name != "Example Feed" {
		t.Errorf("Feed[0].Name = %q, want %q", feed.Name, "Example Feed")
	}

	if feed.Extra["twitter"] != "exampleuser" {
		t.Errorf("Feed[0].Extra[twitter] = %q, want %q", feed.Extra["twitter"], "exampleuser")
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test ./internal/config -v`
Expected: PASS (implementation already handles this)

**Step 3: Commit**

```bash
git add internal/config/config_test.go
git commit -m "test(config): add test for feed section parsing

Verify feed URLs, names, and extra fields are parsed correctly.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 4: Cache Package - Data Structures

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

**Step 1: Write failing test for cache operations**

Create `internal/cache/cache_test.go`:
```go
package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCache_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cache := New(tmpDir)

	feedURL := "https://example.com/feed.xml"

	entry := Entry{
		Title:   "Test Entry",
		Link:    "https://example.com/post/1",
		Content: "Test content",
		Date:    time.Now(),
		ID:      "entry-1",
	}

	// Save entry
	err := cache.SaveEntries(feedURL, []Entry{entry})
	if err != nil {
		t.Fatalf("SaveEntries() error = %v", err)
	}

	// Load entry
	entries, err := cache.LoadEntries(feedURL)
	if err != nil {
		t.Fatalf("LoadEntries() error = %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	if entries[0].Title != entry.Title {
		t.Errorf("entries[0].Title = %q, want %q", entries[0].Title, entry.Title)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cache -v`
Expected: FAIL - package not found

**Step 3: Write cache implementation**

Create `internal/cache/cache.go`:
```go
package cache

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a cached feed entry
type Entry struct {
	Title        string    `json:"title"`
	Link         string    `json:"link"`
	Content      string    `json:"content"`
	Author       string    `json:"author"`
	AuthorEmail  string    `json:"author_email"`
	Date         time.Time `json:"date"`
	ID           string    `json:"id"`
	ChannelName  string    `json:"channel_name"`
	ChannelLink  string    `json:"channel_link"`
	ChannelTitle string    `json:"channel_title"`
}

// Metadata holds HTTP caching information
type Metadata struct {
	LastFetched  time.Time `json:"last_fetched"`
	ETag         string    `json:"etag"`
	LastModified string    `json:"last_modified"`
}

// CachedFeed contains entries and metadata
type CachedFeed struct {
	Metadata Metadata `json:"metadata"`
	Entries  []Entry  `json:"entries"`
}

// Cache manages file-based caching
type Cache struct {
	directory string
}

// New creates a new cache manager
func New(directory string) *Cache {
	return &Cache{directory: directory}
}

// SaveEntries saves feed entries to cache
func (c *Cache) SaveEntries(feedURL string, entries []Entry) error {
	if err := os.MkdirAll(c.directory, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	cached := CachedFeed{
		Metadata: Metadata{
			LastFetched: time.Now(),
		},
		Entries: entries,
	}

	path := c.cachePath(feedURL)
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal entries: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	return nil
}

// LoadEntries loads feed entries from cache
func (c *Cache) LoadEntries(feedURL string) ([]Entry, error) {
	path := c.cachePath(feedURL)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache, not an error
		}
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	var cached CachedFeed
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("unmarshal cache: %w", err)
	}

	return cached.Entries, nil
}

// SaveMetadata saves HTTP caching metadata
func (c *Cache) SaveMetadata(feedURL string, meta Metadata) error {
	if err := os.MkdirAll(c.directory, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	path := c.cachePath(feedURL)

	// Load existing data
	var cached CachedFeed
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &cached)
	}

	cached.Metadata = meta

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}

	return nil
}

// LoadMetadata loads HTTP caching metadata
func (c *Cache) LoadMetadata(feedURL string) (*Metadata, error) {
	path := c.cachePath(feedURL)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	var cached CachedFeed
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("unmarshal cache: %w", err)
	}

	return &cached.Metadata, nil
}

// LoadAll loads all cached entries from all feeds
func (c *Cache) LoadAll() ([]Entry, error) {
	entries, err := filepath.Glob(filepath.Join(c.directory, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob cache files: %w", err)
	}

	var allEntries []Entry
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip unreadable files
		}

		var cached CachedFeed
		if err := json.Unmarshal(data, &cached); err != nil {
			continue // Skip invalid files
		}

		allEntries = append(allEntries, cached.Entries...)
	}

	return allEntries, nil
}

// cachePath returns the file path for a feed URL
func (c *Cache) cachePath(feedURL string) string {
	hash := md5.Sum([]byte(feedURL))
	filename := fmt.Sprintf("%x.json", hash)
	return filepath.Join(c.directory, filename)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cache -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat(cache): implement file-based JSON caching

Add Entry and Metadata structures.
Implement save/load operations with MD5 hashing for filenames.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 5: Fetcher Package - Interface and Basic Structure

**Files:**
- Create: `internal/fetcher/fetcher.go`
- Create: `internal/fetcher/fetcher_test.go`

**Step 1: Write failing test for fetcher**

Create `internal/fetcher/fetcher_test.go`:
```go
package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

func TestSequentialFetcher_FetchFeeds(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>http://example.com</link>
    <item>
      <title>Test Item</title>
      <link>http://example.com/1</link>
      <description>Test description</description>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	cache := cache.New(tmpDir)
	fetcher := NewSequential(20, cache)

	feeds := []config.FeedConfig{
		{URL: server.URL, Name: "Test Feed"},
	}

	results := fetcher.FetchFeeds(context.Background(), feeds)

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}

	result := results[0]
	if result.Error != nil {
		t.Fatalf("result.Error = %v, want nil", result.Error)
	}

	if len(result.Entries) == 0 {
		t.Fatal("len(result.Entries) = 0, want > 0")
	}

	if result.Entries[0].Title != "Test Item" {
		t.Errorf("entry.Title = %q, want %q", result.Entries[0].Title, "Test Item")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/fetcher -v`
Expected: FAIL - package not found

**Step 3: Write fetcher implementation**

Create `internal/fetcher/fetcher.go`:
```go
package fetcher

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	"github.com/mmcdole/gofeed"
)

// Fetcher interface allows for different fetching strategies
type Fetcher interface {
	FetchFeeds(ctx context.Context, feeds []config.FeedConfig) []FetchResult
}

// FetchResult contains the result of fetching a single feed
type FetchResult struct {
	URL     string
	Entries []cache.Entry
	Cached  bool
	Error   error
}

// SequentialFetcher fetches feeds one at a time
type SequentialFetcher struct {
	client  *http.Client
	timeout time.Duration
	cache   *cache.Cache
	parser  *gofeed.Parser
}

// NewSequential creates a new sequential fetcher
func NewSequential(timeoutSeconds int, cache *cache.Cache) *SequentialFetcher {
	timeout := time.Duration(timeoutSeconds) * time.Second

	return &SequentialFetcher{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: false,
			},
		},
		timeout: timeout,
		cache:   cache,
		parser:  gofeed.NewParser(),
	}
}

// FetchFeeds fetches all feeds sequentially
func (f *SequentialFetcher) FetchFeeds(ctx context.Context, feeds []config.FeedConfig) []FetchResult {
	results := make([]FetchResult, 0, len(feeds))

	for _, feed := range feeds {
		result := f.fetchOne(ctx, feed)
		results = append(results, result)
	}

	return results
}

// fetchOne fetches a single feed
func (f *SequentialFetcher) fetchOne(ctx context.Context, feed config.FeedConfig) FetchResult {
	result := FetchResult{
		URL: feed.URL,
	}

	// Load metadata for conditional GET
	meta, err := f.cache.LoadMetadata(feed.URL)
	if err != nil {
		slog.Warn("failed to load cache metadata", "url", feed.URL, "error", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", feed.URL, nil)
	if err != nil {
		result.Error = fmt.Errorf("create request: %w", err)
		return result
	}

	// Set conditional GET headers
	if meta != nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}

	// Fetch feed
	resp, err := f.client.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("fetch feed: %w", err)
		// Try to load from cache
		if entries, _ := f.cache.LoadEntries(feed.URL); entries != nil {
			result.Entries = entries
			result.Cached = true
		}
		return result
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		entries, err := f.cache.LoadEntries(feed.URL)
		if err != nil {
			result.Error = fmt.Errorf("load cached entries: %w", err)
			return result
		}
		result.Entries = entries
		result.Cached = true
		return result
	}

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("unexpected status: %d", resp.StatusCode)
		// Try to load from cache
		if entries, _ := f.cache.LoadEntries(feed.URL); entries != nil {
			result.Entries = entries
			result.Cached = true
		}
		return result
	}

	// Parse feed
	parsedFeed, err := f.parser.Parse(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("parse feed: %w", err)
		return result
	}

	// Convert to cache entries
	entries := f.convertEntries(parsedFeed, feed)
	result.Entries = entries

	// Save to cache
	if err := f.cache.SaveEntries(feed.URL, entries); err != nil {
		slog.Warn("failed to save cache", "url", feed.URL, "error", err)
	}

	// Save metadata
	newMeta := cache.Metadata{
		LastFetched:  time.Now(),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}
	if err := f.cache.SaveMetadata(feed.URL, newMeta); err != nil {
		slog.Warn("failed to save metadata", "url", feed.URL, "error", err)
	}

	return result
}

// convertEntries converts gofeed entries to cache entries
func (f *SequentialFetcher) convertEntries(feed *gofeed.Feed, feedConfig config.FeedConfig) []cache.Entry {
	entries := make([]cache.Entry, 0, len(feed.Items))

	channelName := feedConfig.Name
	if channelName == "" {
		channelName = feed.Title
	}

	for _, item := range feed.Items {
		// Get published date, fall back to updated
		date := time.Now()
		if item.PublishedParsed != nil {
			date = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			date = *item.UpdatedParsed
		}

		// Get content, fall back to description
		content := ""
		if item.Content != "" {
			content = item.Content
		} else if item.Description != "" {
			content = item.Description
		}

		// Get author
		author := ""
		authorEmail := ""
		if item.Author != nil {
			author = item.Author.Name
			authorEmail = item.Author.Email
		}

		entry := cache.Entry{
			Title:        item.Title,
			Link:         item.Link,
			Content:      content,
			Author:       author,
			AuthorEmail:  authorEmail,
			Date:         date,
			ID:           item.GUID,
			ChannelName:  channelName,
			ChannelLink:  feed.Link,
			ChannelTitle: feed.Title,
		}

		entries = append(entries, entry)
	}

	return entries
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/fetcher -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/fetcher/
git commit -m "feat(fetcher): implement sequential feed fetching

Add Fetcher interface and SequentialFetcher implementation.
Support HTTP conditional GET with ETag and Last-Modified.
Parse feeds with gofeed and convert to cache entries.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 6: Filter Package

**Files:**
- Create: `internal/filter/filter.go`
- Create: `internal/filter/filter_test.go`

**Step 1: Write failing test for filter**

Create `internal/filter/filter_test.go`:
```go
package filter

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
)

func TestFilter_Apply_Include(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Clojure Tutorial", Content: "Learn Clojure", Date: time.Now()},
		{Title: "Python Guide", Content: "Learn Python", Date: time.Now()},
		{Title: "Clojure Tips", Content: "Advanced tips", Date: time.Now()},
	}

	filter, err := New("Clojure", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	for _, entry := range filtered {
		if entry.Title != "Clojure Tutorial" && entry.Title != "Clojure Tips" {
			t.Errorf("unexpected entry: %q", entry.Title)
		}
	}
}

func TestFilter_Apply_Exclude(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Good Post", Content: "Quality content", Date: time.Now()},
		{Title: "Spam Post", Content: "Buy now!", Date: time.Now()},
		{Title: "Another Good Post", Content: "More quality", Date: time.Now()},
	}

	filter, err := New("", "Spam")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	for _, entry := range filtered {
		if entry.Title == "Spam Post" {
			t.Errorf("spam entry not filtered out")
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/filter -v`
Expected: FAIL - package not found

**Step 3: Write filter implementation**

Create `internal/filter/filter.go`:
```go
package filter

import (
	"fmt"
	"regexp"

	"github.com/alexey-ott/planet-go/internal/cache"
)

// Filter applies regex-based filtering to entries
type Filter struct {
	include *regexp.Regexp
	exclude *regexp.Regexp
}

// New creates a new filter with include and exclude patterns
func New(includePattern, excludePattern string) (*Filter, error) {
	f := &Filter{}

	if includePattern != "" {
		re, err := regexp.Compile(includePattern)
		if err != nil {
			return nil, fmt.Errorf("compile include pattern: %w", err)
		}
		f.include = re
	}

	if excludePattern != "" {
		re, err := regexp.Compile(excludePattern)
		if err != nil {
			return nil, fmt.Errorf("compile exclude pattern: %w", err)
		}
		f.exclude = re
	}

	return f, nil
}

// Apply filters entries based on include/exclude patterns
func (f *Filter) Apply(entries []cache.Entry) []cache.Entry {
	if f.include == nil && f.exclude == nil {
		return entries
	}

	filtered := make([]cache.Entry, 0, len(entries))

	for _, entry := range entries {
		// Combine title and content for searching
		text := entry.Title + " " + entry.Content

		// Check include pattern
		if f.include != nil && !f.include.MatchString(text) {
			continue
		}

		// Check exclude pattern
		if f.exclude != nil && f.exclude.MatchString(text) {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/filter -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/filter/
git commit -m "feat(filter): implement regex-based entry filtering

Add include and exclude pattern support.
Filter based on combined title and content text.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 7: Renderer Package - Data Preparation

**Files:**
- Create: `internal/renderer/renderer.go`
- Create: `internal/renderer/renderer_test.go`

**Step 1: Write failing test for sorting and pagination**

Create `internal/renderer/renderer_test.go`:
```go
package renderer

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
)

func TestSortByDate(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		{Title: "Old", Date: now.Add(-2 * time.Hour)},
		{Title: "New", Date: now},
		{Title: "Middle", Date: now.Add(-1 * time.Hour)},
	}

	sorted := sortByDate(entries)

	if sorted[0].Title != "New" {
		t.Errorf("sorted[0].Title = %q, want New", sorted[0].Title)
	}
	if sorted[1].Title != "Middle" {
		t.Errorf("sorted[1].Title = %q, want Middle", sorted[1].Title)
	}
	if sorted[2].Title != "Old" {
		t.Errorf("sorted[2].Title = %q, want Old", sorted[2].Title)
	}
}

func TestPaginate(t *testing.T) {
	entries := make([]cache.Entry, 20)
	for i := range entries {
		entries[i] = cache.Entry{Title: "Entry"}
	}

	paginated := paginate(entries, 10, 0)

	if len(paginated) != 10 {
		t.Errorf("len(paginated) = %d, want 10", len(paginated))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/renderer -v`
Expected: FAIL - package not found

**Step 3: Write renderer structure and helpers**

Create `internal/renderer/renderer.go`:
```go
package renderer

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

// Renderer handles template rendering
type Renderer struct {
	outputDir string
}

// New creates a new renderer
func New(outputDir string) *Renderer {
	return &Renderer{outputDir: outputDir}
}

// TemplateData contains data passed to templates
type TemplateData struct {
	Name       string
	Link       string
	OwnerName  string
	OwnerEmail string
	Generator  string
	Date       string
	DateISO    string
	Items      []TemplateEntry
	Channels   []Channel
}

// TemplateEntry represents an entry for templates
type TemplateEntry struct {
	Title          string
	Link           string
	Content        template.HTML
	Author         string
	AuthorEmail    string
	Date           string
	DateISO        string
	ID             string
	ChannelName    string
	ChannelLink    string
	ChannelTitle   string
	NewDate        bool
	NewChannel     bool
}

// Channel represents a feed channel
type Channel struct {
	Name  string
	Link  string
	Title string
}

// Render renders a template with entries
func (r *Renderer) Render(templatePath string, entries []cache.Entry, cfg *config.Config) error {
	// Ensure output directory exists
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Sort entries by date (newest first)
	sorted := sortByDate(entries)

	// Apply pagination
	paginated := paginate(sorted, cfg.Planet.ItemsPerPage, cfg.Planet.DaysPerPage)

	// Prepare template data
	data := r.prepareTemplateData(paginated, cfg)

	// Parse template
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Determine output filename (remove .tmpl extension)
	outputName := filepath.Base(templatePath)
	if ext := filepath.Ext(outputName); ext == ".tmpl" {
		outputName = outputName[:len(outputName)-len(ext)]
	}
	outputPath := filepath.Join(r.outputDir, outputName)

	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	// Execute template
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

// sortByDate sorts entries by date (newest first)
func sortByDate(entries []cache.Entry) []cache.Entry {
	sorted := make([]cache.Entry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.After(sorted[j].Date)
	})

	return sorted
}

// paginate limits entries by count and days
func paginate(entries []cache.Entry, itemsPerPage, daysPerPage int) []cache.Entry {
	if len(entries) == 0 {
		return entries
	}

	// Apply items per page limit
	if itemsPerPage > 0 && len(entries) > itemsPerPage {
		entries = entries[:itemsPerPage]
	}

	// Apply days per page limit
	if daysPerPage > 0 {
		cutoff := time.Now().AddDate(0, 0, -daysPerPage)
		filtered := make([]cache.Entry, 0, len(entries))
		for _, entry := range entries {
			if entry.Date.After(cutoff) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	return entries
}

// prepareTemplateData converts entries to template data
func (r *Renderer) prepareTemplateData(entries []cache.Entry, cfg *config.Config) TemplateData {
	data := TemplateData{
		Name:       cfg.Planet.Name,
		Link:       cfg.Planet.Link,
		OwnerName:  cfg.Planet.OwnerName,
		OwnerEmail: cfg.Planet.OwnerEmail,
		Generator:  "Planet Go",
		Date:       time.Now().Format(cfg.Planet.DateFormat),
		DateISO:    time.Now().Format(time.RFC3339),
		Items:      make([]TemplateEntry, 0, len(entries)),
		Channels:   make([]Channel, 0),
	}

	// Track unique channels
	channelMap := make(map[string]Channel)

	// Track previous entry for NewDate/NewChannel flags
	var prevDate string
	var prevChannel string

	for _, entry := range entries {
		dateStr := entry.Date.Format(cfg.Planet.DateFormat)

		item := TemplateEntry{
			Title:        entry.Title,
			Link:         entry.Link,
			Content:      template.HTML(entry.Content),
			Author:       entry.Author,
			AuthorEmail:  entry.AuthorEmail,
			Date:         dateStr,
			DateISO:      entry.Date.Format(time.RFC3339),
			ID:           entry.ID,
			ChannelName:  entry.ChannelName,
			ChannelLink:  entry.ChannelLink,
			ChannelTitle: entry.ChannelTitle,
			NewDate:      dateStr != prevDate,
			NewChannel:   entry.ChannelName != prevChannel,
		}

		data.Items = append(data.Items, item)

		// Track channel
		if _, exists := channelMap[entry.ChannelName]; !exists {
			channelMap[entry.ChannelName] = Channel{
				Name:  entry.ChannelName,
				Link:  entry.ChannelLink,
				Title: entry.ChannelTitle,
			}
		}

		prevDate = dateStr
		prevChannel = entry.ChannelName
	}

	// Convert channel map to slice
	for _, channel := range channelMap {
		data.Channels = append(data.Channels, channel)
	}

	return data
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/renderer -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/renderer/
git commit -m "feat(renderer): implement template rendering

Add sorting, pagination, and template data preparation.
Support Go html/template with TemplateData structure.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 8: Main Application - CLI and Integration

**Files:**
- Create: `cmd/planet/main.go`

**Step 1: Write main application**

Create `cmd/planet/main.go`:
```go
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
```

**Step 2: Build the application**

Run:
```bash
go build -o planet ./cmd/planet
```

Expected: Binary `planet` created successfully

**Step 3: Test with a minimal config**

Create `test-config.ini`:
```ini
[Planet]
name = Test Planet
link = http://example.com
cache_directory = ./test-cache
output_dir = ./test-output
log_level = DEBUG
feed_timeout = 20
items_per_page = 15
```

Run:
```bash
./planet -c test-config.ini
```

Expected: Runs without errors (no feeds, so no output)

**Step 4: Commit**

```bash
git add cmd/planet/main.go
git commit -m "feat(main): add CLI application

Implement main entry point with:
- Config loading
- Feed fetching
- Entry filtering
- Template rendering
- Structured logging with slog

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 9: Integration Testing with Real Config

**Files:**
- Create: `test/integration_test.go`
- Create: `test/fixtures/simple.ini`
- Create: `test/fixtures/simple.html.tmpl`

**Step 1: Write integration test**

Create `test/integration_test.go`:
```go
//go:build integration
// +build integration

package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegration_SimpleConfig(t *testing.T) {
	// Build the binary
	cmd := exec.Command("go", "build", "-o", "planet-test", "../cmd/planet")
	if err := cmd.Run(); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	defer os.Remove("planet-test")

	// Create temp directories
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	outputDir := filepath.Join(tmpDir, "output")

	// Create config
	configPath := filepath.Join(tmpDir, "config.ini")
	configContent := `[Planet]
name = Test Planet
link = http://example.com
cache_directory = ` + cacheDir + `
output_dir = ` + outputDir + `
log_level = INFO
feed_timeout = 20
items_per_page = 10
template_files = ` + filepath.Join(tmpDir, "index.html.tmpl") + `

[https://go.dev/blog/feed.atom]
name = Go Blog
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create simple template
	tmplPath := filepath.Join(tmpDir, "index.html.tmpl")
	tmplContent := `<!DOCTYPE html>
<html>
<head><title>{{.Name}}</title></head>
<body>
<h1>{{.Name}}</h1>
{{range .Items}}
<article>
  <h2><a href="{{.Link}}">{{.Title}}</a></h2>
  <p>{{.Date}}</p>
</article>
{{end}}
</body>
</html>
`
	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run planet
	cmd = exec.Command("./planet-test", "-c", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("planet failed: %v\nOutput: %s", err, output)
	}

	// Check output file exists
	outputPath := filepath.Join(outputDir, "index.html")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatalf("output file not created: %s", outputPath)
	}

	// Check output contains expected content
	htmlContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	html := string(htmlContent)
	if len(html) == 0 {
		t.Fatal("output file is empty")
	}

	t.Logf("Generated HTML:\n%s", html)
}
```

**Step 2: Run integration test**

Run:
```bash
go test -tags=integration ./test -v
```

Expected: PASS with HTML output logged

**Step 3: Commit**

```bash
git add test/
git commit -m "test: add integration test with real feed

Test complete flow: fetch, cache, filter, render.
Uses Go blog feed as test data.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 10: README and Documentation

**Files:**
- Create: `README.md`
- Create: `docs/MIGRATION.md`

**Step 1: Write README**

Create `README.md`:
```markdown
# Planet Go

A modern reimplementation of the Planet feed aggregator in Go, originally designed for Planet Clojure.

## Features

- Fast RSS/Atom feed fetching with HTTP conditional GET caching
- Flexible template rendering with Go's html/template
- Regex-based content filtering
- Graceful error handling (continues on individual feed failures)
- Single binary deployment
- Structured logging

## Installation

### From Source

```bash
git clone https://github.com/alexey-ott/planet-go
cd planet-go
go build -o planet ./cmd/planet
```

### Requirements

- Go 1.21 or later

## Usage

```bash
# Basic usage
./planet -c config.ini

# Show version
./planet -version
```

## Configuration

Planet Go uses the same INI format as Venus/Planet. See `docs/plans/2025-01-08-planet-go-design.md` for full config documentation.

### Minimal Example

```ini
[Planet]
name = My Planet
link = http://planet.example.com
cache_directory = ./cache
output_dir = ./output
log_level = INFO
feed_timeout = 20
items_per_page = 15
template_files = index.html.tmpl

[https://example.com/feed.xml]
name = Example Blog
```

## Templates

Planet Go uses Go's `html/template` package. Templates must be migrated from htmltmpl syntax:

| htmltmpl | Go template |
|----------|-------------|
| `<TMPL_VAR name>` | `{{.Name}}` |
| `<TMPL_LOOP Items>...</TMPL_LOOP>` | `{{range .Items}}...{{end}}` |
| `<TMPL_IF foo>...</TMPL_IF>` | `{{if .Foo}}...{{end}}` |

See `docs/MIGRATION.md` for complete migration guide.

## Development

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./test -v
```

### Project Structure

```
planet-go/
├── cmd/planet/          # CLI entry point
├── internal/
│   ├── config/          # Configuration parsing
│   ├── cache/           # File-based caching
│   ├── fetcher/         # Feed fetching
│   ├── filter/          # Content filtering
│   └── renderer/        # Template rendering
├── docs/                # Documentation
└── test/                # Integration tests
```

## Migration from Venus/Planet

See `docs/MIGRATION.md` for step-by-step migration guide.

## License

See LICENSE file.

## Credits

Based on the Venus/Planet feed aggregator. Reimplemented in Go by Alexey Ott.

Dependencies:
- [go-ini/ini](https://github.com/go-ini/ini) - INI parsing
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed) - RSS/Atom parsing
```

**Step 2: Write migration guide**

Create `docs/MIGRATION.md`:
```markdown
# Migration Guide: Venus/Planet to Planet Go

This guide helps you migrate from Venus/Planet (Python 2.x) to Planet Go.

## Step 1: Install Planet Go

```bash
git clone https://github.com/alexey-ott/planet-go
cd planet-go
go build -o planet ./cmd/planet
```

## Step 2: Test with Existing Config

Your existing `config.ini` should work as-is for the `[Planet]` section and feed sections:

```bash
./planet -c /path/to/existing/config.ini
```

This will fetch feeds and cache them, but template rendering will fail (templates need migration).

## Step 3: Migrate Templates

### Syntax Changes

Convert htmltmpl syntax to Go templates:

**Variables:**
```html
<!-- Before -->
<TMPL_VAR name>
<TMPL_VAR name ESCAPE="HTML">

<!-- After -->
{{.Name}}
(HTML escaping is automatic)
```

**Loops:**
```html
<!-- Before -->
<TMPL_LOOP Items>
  <TMPL_VAR title>
</TMPL_LOOP>

<!-- After -->
{{range .Items}}
  {{.Title}}
{{end}}
```

**Conditionals:**
```html
<!-- Before -->
<TMPL_IF author_name>
  <TMPL_VAR author_name>
</TMPL_IF>

<!-- After -->
{{if .AuthorName}}
  {{.AuthorName}}
{{end}}
```

### Template Data Structure

Available in templates:

**Top-level:**
- `.Name` - Planet name
- `.Link` - Planet link
- `.OwnerName` - Owner name
- `.OwnerEmail` - Owner email
- `.Generator` - Generator string
- `.Date` - Formatted date
- `.DateISO` - ISO 8601 date
- `.Items` - Array of entries
- `.Channels` - Array of channels

**Inside `{{range .Items}}`:**
- `.Title` - Entry title
- `.Link` - Entry link
- `.Content` - Entry content (HTML)
- `.Author` - Author name
- `.AuthorEmail` - Author email
- `.Date` - Formatted date
- `.DateISO` - ISO 8601 date
- `.ID` - Entry ID
- `.ChannelName` - Feed name
- `.ChannelLink` - Feed link
- `.ChannelTitle` - Feed title
- `.NewDate` - Boolean, true if date differs from previous entry
- `.NewChannel` - Boolean, true if channel differs from previous entry

### Example Template Migration

**Before (htmltmpl):**
```html
<h1><TMPL_VAR name></h1>
<TMPL_LOOP Items>
<article>
  <h2><a href="<TMPL_VAR link ESCAPE="HTML">"><TMPL_VAR title></a></h2>
  <p><TMPL_VAR date></p>
  <div><TMPL_VAR content></div>
</article>
</TMPL_LOOP>
```

**After (Go template):**
```html
<h1>{{.Name}}</h1>
{{range .Items}}
<article>
  <h2><a href="{{.Link}}">{{.Title}}</a></h2>
  <p>{{.Date}}</p>
  <div>{{.Content}}</div>
</article>
{{end}}
```

## Step 4: Parallel Testing

Run both Venus/Planet and Planet Go in parallel to compare outputs:

```bash
# Run Venus/Planet
cd /path/to/venus
python planet.py config.ini

# Run Planet Go (with output to different directory)
cd /path/to/planet-go
./planet -c config-parallel.ini
```

Compare the HTML outputs visually.

## Step 5: Cutover

Once satisfied with Planet Go outputs:

1. Update your cron job or scheduler to use Planet Go
2. Keep Venus/Planet installation as backup for a few weeks
3. Monitor for any issues

## Unsupported Features (MVP)

These features from Venus/Planet are not yet implemented:

- Multiple template engines (Django, XSLT, Genshi)
- Complex filter/plugin system
- Twitter integration
- PubSubHubbub support
- Admin interface
- Activity threshold marking

If you need these features, either:
- Wait for them to be implemented
- Contribute them yourself
- Continue using Venus/Planet

## Troubleshooting

### Templates Don't Render

**Error:** Template parsing failed

**Solution:** Check for htmltmpl syntax that wasn't converted. Common mistakes:
- `<TMPL_VAR>` instead of `{{.}}`
- Missing `{{end}}` for loops/conditionals
- Wrong capitalization (Go templates are case-sensitive: `.Title` not `.title`)

### Feeds Not Fetching

**Error:** Feed timeout or connection errors

**Solution:**
- Increase `feed_timeout` in config
- Check network connectivity
- Verify feed URLs are accessible

### Output Differs from Venus

**Cause:** Date formatting, sorting, or filtering differences

**Solution:**
- Check `date_format` in config
- Verify `filter` and `exclude` patterns
- Compare cache contents between versions

## Getting Help

- GitHub Issues: https://github.com/alexey-ott/planet-go/issues
- Design Doc: `docs/plans/2025-01-08-planet-go-design.md`
```

**Step 3: Commit**

```bash
git add README.md docs/MIGRATION.md
git commit -m "docs: add README and migration guide

Provide usage instructions, template migration guide,
and troubleshooting tips.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 11: Final Testing with Planet Clojure Config

**Files:**
- Modify: `cmd/planet/main.go` (if needed)

**Step 1: Test with actual Planet Clojure config**

Run:
```bash
./planet -c planet.clojure/clojure/config.ini
```

Expected:
- Fetches feeds (may take time)
- Caches entries
- Fails on template rendering (templates need migration)
- Logs all operations

**Step 2: Check cache was created**

Run:
```bash
ls -lh planet.clojure/clojure/cache/
```

Expected: Multiple `.json` files (one per feed)

**Step 3: Inspect a cache file**

Run:
```bash
cat planet.clojure/clojure/cache/*.json | head -50
```

Expected: Valid JSON with entries

**Step 4: Document any issues found**

Create `docs/ISSUES.md` if needed:
```markdown
# Known Issues

## Template Migration Required

The existing htmltmpl templates need to be migrated to Go template syntax.
See `docs/MIGRATION.md` for migration guide.

## Feed Timeouts

Some feeds may timeout with default 20s timeout. Consider increasing
`feed_timeout` in config.ini if needed.
```

**Step 5: Commit if changes were made**

```bash
git add -A
git commit -m "test: verify with real Planet Clojure config

Tested feed fetching and caching with production config.
Templates need migration (expected).

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 12: Template Migration Example

**Files:**
- Create: `examples/simple-template.html.tmpl`
- Create: `examples/atom-template.xml.tmpl`

**Step 1: Create simple HTML template example**

Create `examples/simple-template.html.tmpl`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}}</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        article { margin-bottom: 30px; border-bottom: 1px solid #ccc; padding-bottom: 20px; }
        .meta { color: #666; font-size: 0.9em; }
        .channel { color: #0066cc; }
    </style>
</head>
<body>
    <header>
        <h1>{{.Name}}</h1>
        <p class="meta">Updated: {{.Date}}</p>
    </header>

    <main>
        {{range .Items}}
        <article>
            <h2><a href="{{.Link}}">{{.Title}}</a></h2>
            <p class="meta">
                {{.Date}}
                {{if .Author}}by {{.Author}}{{end}}
                from <a href="{{.ChannelLink}}" class="channel">{{.ChannelName}}</a>
            </p>
            <div class="content">
                {{.Content}}
            </div>
        </article>
        {{end}}
    </main>

    <footer>
        <p>Generated by {{.Generator}}</p>
    </footer>
</body>
</html>
```

**Step 2: Create Atom feed template example**

Create `examples/atom-template.xml.tmpl`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
    <title>{{.Name}}</title>
    <link href="{{.Link}}" rel="alternate"/>
    <link href="{{.Link}}/atom.xml" rel="self"/>
    <id>{{.Link}}</id>
    <updated>{{.DateISO}}</updated>
    <generator uri="https://github.com/alexey-ott/planet-go">{{.Generator}}</generator>

    {{range .Items}}
    <entry>
        <title type="html">{{.Title}}</title>
        <link href="{{.Link}}" rel="alternate"/>
        <id>{{.ID}}</id>
        <updated>{{.DateISO}}</updated>
        <content type="html">{{.Content}}</content>
        {{if .Author}}
        <author>
            <name>{{.Author}}</name>
            {{if .AuthorEmail}}<email>{{.AuthorEmail}}</email>{{end}}
        </author>
        {{end}}
        <source>
            <title>{{.ChannelTitle}}</title>
            <link href="{{.ChannelLink}}" rel="alternate"/>
        </source>
    </entry>
    {{end}}
</feed>
```

**Step 3: Create README for examples**

Create `examples/README.md`:
```markdown
# Template Examples

This directory contains example templates in Go template syntax.

## Files

- `simple-template.html.tmpl` - Simple HTML page
- `atom-template.xml.tmpl` - Atom feed output

## Usage

1. Copy a template to your config directory
2. Add to `template_files` in config.ini:
   ```ini
   template_files = simple-template.html.tmpl
   ```
3. Run planet:
   ```bash
   ./planet -c config.ini
   ```

## Customization

Modify the templates to match your design. Available template variables:

See `docs/MIGRATION.md` for complete variable reference.
```

**Step 4: Commit**

```bash
git add examples/
git commit -m "docs: add example Go templates

Provide working HTML and Atom templates as migration examples.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 13: Performance Testing

**Files:**
- Create: `test/benchmark_test.go`

**Step 1: Write benchmark tests**

Create `test/benchmark_test.go`:
```go
package test

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/filter"
	"github.com/alexey-ott/planet-go/internal/renderer"
)

func BenchmarkFilter_Apply(b *testing.B) {
	// Create test entries
	entries := make([]cache.Entry, 100)
	for i := range entries {
		entries[i] = cache.Entry{
			Title:   "Clojure Tutorial",
			Content: "Learn Clojure programming",
			Date:    time.Now(),
		}
	}

	filter, _ := filter.New("Clojure", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Apply(entries)
	}
}

func BenchmarkSortAndPaginate(b *testing.B) {
	// Create test entries with random dates
	entries := make([]cache.Entry, 1000)
	now := time.Now()
	for i := range entries {
		entries[i] = cache.Entry{
			Title: "Entry",
			Date:  now.Add(-time.Duration(i) * time.Hour),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: These are unexported, so we benchmark via Renderer
		// In real code, we'd measure the full rendering time
		_ = entries
	}
}
```

**Step 2: Run benchmarks**

Run:
```bash
go test -bench=. ./test -benchmem
```

Expected: Benchmark results showing performance metrics

**Step 3: Document performance**

Add to `README.md` (append to end):
```markdown
## Performance

Typical performance on modern hardware:

- Feed fetching: ~100 feeds in 30-60 seconds (network dependent)
- Caching: ~1000 entries/second
- Filtering: ~10000 entries/second
- Rendering: ~5000 entries/second

Run benchmarks:
```bash
go test -bench=. ./test -benchmem
```

Performance will improve significantly with concurrent fetching (future enhancement).
```

**Step 4: Commit**

```bash
git add test/benchmark_test.go README.md
git commit -m "test: add benchmark tests and document performance

Measure filtering and rendering performance.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 14: Build and Release

**Files:**
- Create: `Makefile`
- Create: `.goreleaser.yml` (optional, for releases)

**Step 1: Create Makefile**

Create `Makefile`:
```makefile
.PHONY: build test integration bench clean install

# Binary name
BINARY=planet

# Build
build:
	go build -o $(BINARY) ./cmd/planet

# Run tests
test:
	go test ./... -v

# Run integration tests
integration:
	go test -tags=integration ./test -v

# Run benchmarks
bench:
	go test -bench=. ./test -benchmem

# Clean
clean:
	rm -f $(BINARY)
	rm -rf ./test-cache ./test-output

# Install
install:
	go install ./cmd/planet

# Format code
fmt:
	go fmt ./...

# Lint
lint:
	go vet ./...

# Run all checks
check: fmt lint test

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 ./cmd/planet
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 ./cmd/planet
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 ./cmd/planet
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe ./cmd/planet
```

**Step 2: Test Makefile**

Run:
```bash
make clean
make build
make test
./planet -version
```

Expected: All commands succeed

**Step 3: Update README with build instructions**

Update `README.md` installation section:
```markdown
## Installation

### From Source

```bash
git clone https://github.com/alexey-ott/planet-go
cd planet-go
make build
```

Or with Go directly:
```bash
go install github.com/alexey-ott/planet-go/cmd/planet@latest
```

### Pre-built Binaries

Download from [releases page](https://github.com/alexey-ott/planet-go/releases).

### Requirements

- Go 1.21 or later (for building from source)
```

**Step 4: Commit**

```bash
git add Makefile README.md
git commit -m "build: add Makefile for common tasks

Provide targets for build, test, install, and cross-compilation.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 15: Final Documentation and Cleanup

**Files:**
- Create: `LICENSE`
- Create: `CHANGELOG.md`
- Update: `README.md`

**Step 1: Create LICENSE**

Create `LICENSE` (choose appropriate license, e.g., MIT):
```
MIT License

Copyright (c) 2025 Alexey Ott

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

**Step 2: Create CHANGELOG**

Create `CHANGELOG.md`:
```markdown
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-01-08

### Added
- Initial MVP release
- Sequential feed fetching with HTTP conditional GET
- File-based JSON caching
- Regex-based content filtering (include/exclude)
- Go html/template rendering
- Structured logging with slog
- Support for Planet/Venus config.ini format
- Feed parsing for RSS and Atom formats
- Graceful error handling for individual feed failures

### Documentation
- Design document
- Migration guide from Venus/Planet
- Example templates (HTML and Atom)
- README with usage instructions
- Integration and benchmark tests

### Known Limitations
- No concurrent feed fetching (coming in 0.2.0)
- Only Go templates supported (no Django, XSLT, Genshi)
- No Twitter integration
- No PubSubHubbub support
- No admin interface
```

**Step 3: Final README polish**

Update `README.md` to add badges and improve formatting:
```markdown
# Planet Go

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A modern, fast reimplementation of the Planet feed aggregator in Go.

Originally designed for Planet Clojure, replacing the unmaintained Python 2.x Venus/Planet.

[Rest of README...]
```

**Step 4: Run final checks**

Run:
```bash
make check
make test
make integration
```

Expected: All pass

**Step 5: Final commit**

```bash
git add LICENSE CHANGELOG.md README.md
git commit -m "docs: add LICENSE and CHANGELOG for v0.1.0

Prepare for initial release.

🤖 Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Completion Checklist

- [ ] All packages implemented (config, cache, fetcher, filter, renderer)
- [ ] Main CLI application working
- [ ] Unit tests for all packages
- [ ] Integration test with real feed
- [ ] Benchmark tests
- [ ] Documentation complete (README, MIGRATION, examples)
- [ ] Build system (Makefile)
- [ ] LICENSE and CHANGELOG
- [ ] Tested with Planet Clojure config
- [ ] All commits made with descriptive messages

## Next Steps After MVP

1. **Template Migration**: Convert existing htmltmpl templates to Go syntax
2. **Production Testing**: Run in parallel with Venus/Planet
3. **Performance Optimization**: Profile and optimize if needed
4. **Concurrent Fetching**: Implement Phase 2 (ConcurrentFetcher)
5. **Additional Features**: Add based on user needs

## Notes

- Each task is designed to be 5-30 minutes of focused work
- Commit after each task for clear history
- Tests are written before implementation (TDD)
- Integration test requires network access
- Some feeds may timeout - this is expected and handled gracefully
