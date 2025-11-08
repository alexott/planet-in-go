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
