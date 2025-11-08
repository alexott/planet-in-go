package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// sanitizeURL converts a URL into a safe filename
// Example: https://go.dev/blog/feed.atom -> go.dev-blog-feed.atom
func sanitizeURL(url string) string {
	// Remove scheme (http://, https://, etc.)
	url = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`).ReplaceAllString(url, "")
	
	// Remove trailing slash
	url = strings.TrimSuffix(url, "/")
	
	// Replace invalid filename characters with dash
	// Keep alphanumeric, dots, and replace everything else with dash
	url = regexp.MustCompile(`[^a-zA-Z0-9._-]`).ReplaceAllString(url, "-")
	
	// Replace multiple consecutive dashes with single dash
	url = regexp.MustCompile(`-+`).ReplaceAllString(url, "-")
	
	// Trim leading/trailing dashes
	url = strings.Trim(url, "-")
	
	// Ensure filename is not too long (max 200 chars before .json)
	if len(url) > 200 {
		url = url[:200]
		url = strings.TrimSuffix(url, "-")
	}
	
	return url
}

// cachePath returns the file path for a feed URL
func (c *Cache) cachePath(feedURL string) string {
	filename := sanitizeURL(feedURL) + ".json"
	return filepath.Join(c.directory, filename)
}
