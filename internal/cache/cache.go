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
