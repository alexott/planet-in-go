package fetcher

import (
	"context"
	"fmt"
	"log/slog"
	"net"
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

	// Configure transport with aggressive timeouts to prevent hangs
	transport := &http.Transport{
		// Connection pool settings
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,

		// Timeout settings to prevent hangs
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second, // Max time to establish connection
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout:   10 * time.Second, // Max time for TLS handshake
		ResponseHeaderTimeout: timeout,          // Max time to read response headers
		ExpectContinueTimeout: 1 * time.Second,  // Max time to wait for 100-continue

		DisableCompression: false,
		ForceAttemptHTTP2:  true,
	}

	slog.Debug("initializing sequential fetcher",
		"timeout", timeout,
		"dial_timeout", 10*time.Second,
		"tls_timeout", 10*time.Second)

	return &SequentialFetcher{
		client: &http.Client{
			Timeout:   timeout, // Overall request timeout
			Transport: transport,
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
	startTime := time.Now()
	result := FetchResult{
		URL: feed.URL,
	}

	slog.Debug("starting feed fetch",
		"url", feed.URL,
		"name", feed.Name)

	// Load metadata for conditional GET
	meta, err := f.cache.LoadMetadata(feed.URL)
	if err != nil {
		slog.Warn("failed to load cache metadata", "url", feed.URL, "error", err)
	} else if meta != nil {
		slog.Debug("loaded cache metadata",
			"url", feed.URL,
			"etag", meta.ETag,
			"last_modified", meta.LastModified,
			"last_fetched", meta.LastFetched)
	}

	// Create request
	slog.Debug("creating HTTP request", "url", feed.URL)
	req, err := http.NewRequestWithContext(ctx, "GET", feed.URL, nil)
	if err != nil {
		result.Error = fmt.Errorf("create request: %w", err)
		slog.Error("failed to create request", "url", feed.URL, "error", err)
		return result
	}

	// Set User-Agent
	req.Header.Set("User-Agent", "Planet-Go/0.1.0 (Feed Aggregator)")

	// Set conditional GET headers
	if meta != nil {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
			slog.Debug("set conditional GET header", "url", feed.URL, "etag", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
			slog.Debug("set conditional GET header", "url", feed.URL, "last_modified", meta.LastModified)
		}
	}

	// Fetch feed
	slog.Debug("sending HTTP request", "url", feed.URL, "timeout", f.timeout)
	fetchStart := time.Now()
	resp, err := f.client.Do(req)
	fetchDuration := time.Since(fetchStart)

	if err != nil {
		result.Error = fmt.Errorf("fetch feed: %w", err)
		slog.Error("HTTP request failed",
			"url", feed.URL,
			"error", err,
			"duration", fetchDuration)

		// Try to load from cache
		slog.Debug("attempting to load from cache", "url", feed.URL)
		if entries, _ := f.cache.LoadEntries(feed.URL); entries != nil {
			result.Entries = entries
			result.Cached = true
			slog.Info("using cached entries after fetch failure",
				"url", feed.URL,
				"entries", len(entries))
		}
		return result
	}
	defer resp.Body.Close()

	slog.Debug("received HTTP response",
		"url", feed.URL,
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"duration", fetchDuration)

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		slog.Debug("feed not modified, using cache", "url", feed.URL)
		entries, err := f.cache.LoadEntries(feed.URL)
		if err != nil {
			result.Error = fmt.Errorf("load cached entries: %w", err)
			slog.Error("failed to load cached entries", "url", feed.URL, "error", err)
			return result
		}
		result.Entries = entries
		result.Cached = true
		slog.Info("feed cached (304 Not Modified)",
			"url", feed.URL,
			"entries", len(entries),
			"duration", time.Since(startTime))
		return result
	}

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("unexpected status: %d", resp.StatusCode)
		slog.Error("unexpected HTTP status",
			"url", feed.URL,
			"status", resp.StatusCode,
			"status_text", resp.Status)

		// Try to load from cache
		slog.Debug("attempting to load from cache", "url", feed.URL)
		if entries, _ := f.cache.LoadEntries(feed.URL); entries != nil {
			result.Entries = entries
			result.Cached = true
			slog.Info("using cached entries after HTTP error",
				"url", feed.URL,
				"entries", len(entries))
		}
		return result
	}

	// Parse feed
	slog.Debug("parsing feed", "url", feed.URL)
	parseStart := time.Now()
	parsedFeed, err := f.parser.Parse(resp.Body)
	parseDuration := time.Since(parseStart)

	if err != nil {
		result.Error = fmt.Errorf("parse feed: %w", err)
		slog.Error("failed to parse feed",
			"url", feed.URL,
			"error", err,
			"parse_duration", parseDuration)
		return result
	}

	slog.Debug("feed parsed successfully",
		"url", feed.URL,
		"title", parsedFeed.Title,
		"items", len(parsedFeed.Items),
		"parse_duration", parseDuration)

	// Convert to cache entries
	entries := f.convertEntries(parsedFeed, feed)
	result.Entries = entries

	// Save to cache
	slog.Debug("saving to cache", "url", feed.URL, "entries", len(entries))
	if err := f.cache.SaveEntries(feed.URL, entries); err != nil {
		slog.Warn("failed to save cache", "url", feed.URL, "error", err)
	} else {
		slog.Debug("cache saved", "url", feed.URL)
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

	totalDuration := time.Since(startTime)
	slog.Info("feed fetched successfully",
		"url", feed.URL,
		"entries", len(entries),
		"duration", totalDuration,
		"fetch_time", fetchDuration,
		"parse_time", parseDuration)

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
