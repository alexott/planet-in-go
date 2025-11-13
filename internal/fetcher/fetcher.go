package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
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
	debug   bool
}

// NewSequential creates a new sequential fetcher
func NewSequential(timeoutSeconds int, cache *cache.Cache, debug bool) *SequentialFetcher {
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
		debug:   debug,
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

	// Read response body so we can optionally save raw data for debugging
	slog.Debug("reading response body", "url", feed.URL)
	bodyStart := time.Now()
	bodyBytes, err := io.ReadAll(resp.Body)
	bodyDuration := time.Since(bodyStart)
	if err != nil {
		result.Error = fmt.Errorf("read response body: %w", err)
		slog.Error("failed to read response body", "url", feed.URL, "error", err)
		return result
	}

	// If debug mode is enabled, save the raw response body as .xml in cache
	if f.debug {
		if err := f.cache.SaveRaw(feed.URL, bodyBytes); err != nil {
			slog.Warn("failed to save raw response body", "url", feed.URL, "error", err)
		} else {
			slog.Debug("saved raw response body", "url", feed.URL, "size", len(bodyBytes))
		}
	}

	// Parse feed from the bytes reader
	slog.Debug("parsing feed", "url", feed.URL)
	parseStart := time.Now()
	parsedFeed, err := f.parser.Parse(bytes.NewReader(bodyBytes))
	parseDuration := time.Since(parseStart)
	slog.Debug("response body read duration", "url", feed.URL, "duration", bodyDuration)

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

	// Extract channel-level metadata once (same for all entries)
	channelLanguage := feed.Language
	channelSubtitle := feed.Description
	channelURL := feedConfig.URL // Feed URL
	channelRights := feed.Copyright

	// Channel author
	channelAuthorName := ""
	channelAuthorEmail := ""
	if feed.Author != nil {
		channelAuthorName = feed.Author.Name
		channelAuthorEmail = feed.Author.Email
	}

	// Channel ID (prefer feed ID, fall back to feed link)
	channelID := feed.FeedLink
	if channelID == "" {
		channelID = feed.Link
	}

	// Channel updated time
	channelUpdated := time.Time{}
	if feed.UpdatedParsed != nil {
		channelUpdated = *feed.UpdatedParsed
	}

	for _, item := range feed.Items {
		// Get published date, fall back to updated, fall back to channel updated.
		// Do NOT default to time.Now() — that makes old items look like new ones
		// if the feed item lacks date metadata. Prefer leaving date zeroTime so
		// the renderer can decide how to display it.
		var date time.Time
		if item.PublishedParsed != nil {
			date = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			date = *item.UpdatedParsed
		} else if feed.UpdatedParsed != nil {
			// If the channel/feed has an updated timestamp, use it as a last-resort
			date = *feed.UpdatedParsed
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

			// Additional channel metadata
			ChannelLanguage:    channelLanguage,
			TitleLanguage:      channelLanguage, // Use channel language as default
			ContentLanguage:    channelLanguage, // Use channel language as default
			ChannelAuthorName:  channelAuthorName,
			ChannelAuthorEmail: channelAuthorEmail,
			ChannelSubtitle:    channelSubtitle,
			ChannelURL:         channelURL,
			ChannelID:          channelID,
			ChannelUpdated:     channelUpdated,
			ChannelRights:      channelRights,
		}

		entries = append(entries, entry)
	}

	return entries
}

// ParallelFetcher fetches feeds concurrently using a worker pool
type ParallelFetcher struct {
	client  *http.Client
	timeout time.Duration
	cache   *cache.Cache
	parser  *gofeed.Parser
	debug   bool
	workers int
}

// NewParallel creates a new parallel fetcher with specified number of workers
func NewParallel(timeoutSeconds int, cache *cache.Cache, debug bool, workers int) *ParallelFetcher {
	timeout := time.Duration(timeoutSeconds) * time.Second

	// Configure transport with aggressive timeouts to prevent hangs
	transport := &http.Transport{
		// Connection pool settings
		MaxIdleConns:        workers * 2,
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

	slog.Debug("initializing parallel fetcher",
		"timeout", timeout,
		"workers", workers,
		"dial_timeout", 10*time.Second,
		"tls_timeout", 10*time.Second)

	return &ParallelFetcher{
		client: &http.Client{
			Timeout:   timeout, // Overall request timeout
			Transport: transport,
		},
		timeout: timeout,
		cache:   cache,
		parser:  gofeed.NewParser(),
		debug:   debug,
		workers: workers,
	}
}

// FetchFeeds fetches all feeds in parallel using a worker pool
func (f *ParallelFetcher) FetchFeeds(ctx context.Context, feeds []config.FeedConfig) []FetchResult {
	numFeeds := len(feeds)
	if numFeeds == 0 {
		return []FetchResult{}
	}

	// Create channels for work distribution and result collection
	feedsChan := make(chan config.FeedConfig, numFeeds)
	resultsChan := make(chan FetchResult, numFeeds)

	// Use WaitGroup to track worker completion
	var wg sync.WaitGroup

	// Determine actual number of workers (don't spawn more than feeds)
	actualWorkers := f.workers
	if actualWorkers > numFeeds {
		actualWorkers = numFeeds
	}

	slog.Info("starting parallel fetch",
		"feeds", numFeeds,
		"workers", actualWorkers)

	// Start worker goroutines
	for i := 0; i < actualWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			slog.Debug("worker started", "worker_id", workerID)

			for feed := range feedsChan {
				slog.Debug("worker processing feed",
					"worker_id", workerID,
					"url", feed.URL)

				result := f.fetchOne(ctx, feed)
				resultsChan <- result
			}

			slog.Debug("worker finished", "worker_id", workerID)
		}(i)
	}

	// Send all feeds to the work channel
	go func() {
		for _, feed := range feeds {
			feedsChan <- feed
		}
		close(feedsChan)
	}()

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect all results
	results := make([]FetchResult, 0, numFeeds)
	for result := range resultsChan {
		results = append(results, result)
	}

	slog.Info("parallel fetch complete",
		"feeds", numFeeds,
		"workers", actualWorkers,
		"results", len(results))

	return results
}

// fetchOne fetches a single feed (used by ParallelFetcher)
// This is mostly the same as SequentialFetcher.fetchOne but we need a separate copy
// to maintain independence between the two implementations
func (f *ParallelFetcher) fetchOne(ctx context.Context, feed config.FeedConfig) FetchResult {
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

	// Read response body so we can optionally save raw data for debugging
	slog.Debug("reading response body", "url", feed.URL)
	bodyStart := time.Now()
	bodyBytes, err := io.ReadAll(resp.Body)
	bodyDuration := time.Since(bodyStart)
	if err != nil {
		result.Error = fmt.Errorf("read response body: %w", err)
		slog.Error("failed to read response body", "url", feed.URL, "error", err)
		return result
	}

	// If debug mode is enabled, save the raw response body as .xml in cache
	if f.debug {
		if err := f.cache.SaveRaw(feed.URL, bodyBytes); err != nil {
			slog.Warn("failed to save raw response body", "url", feed.URL, "error", err)
		} else {
			slog.Debug("saved raw response body", "url", feed.URL, "size", len(bodyBytes))
		}
	}

	// Parse feed from the bytes reader
	slog.Debug("parsing feed", "url", feed.URL)
	parseStart := time.Now()
	parsedFeed, err := f.parser.Parse(bytes.NewReader(bodyBytes))
	parseDuration := time.Since(parseStart)
	slog.Debug("response body read duration", "url", feed.URL, "duration", bodyDuration)

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

// convertEntries converts gofeed entries to cache entries (used by ParallelFetcher)
func (f *ParallelFetcher) convertEntries(feed *gofeed.Feed, feedConfig config.FeedConfig) []cache.Entry {
	entries := make([]cache.Entry, 0, len(feed.Items))

	channelName := feedConfig.Name
	if channelName == "" {
		channelName = feed.Title
	}

	// Extract channel-level metadata once (same for all entries)
	channelLanguage := feed.Language
	channelSubtitle := feed.Description
	channelURL := feedConfig.URL // Feed URL
	channelRights := feed.Copyright

	// Channel author
	channelAuthorName := ""
	channelAuthorEmail := ""
	if feed.Author != nil {
		channelAuthorName = feed.Author.Name
		channelAuthorEmail = feed.Author.Email
	}

	// Channel ID (prefer feed ID, fall back to feed link)
	channelID := feed.FeedLink
	if channelID == "" {
		channelID = feed.Link
	}

	// Channel updated time
	channelUpdated := time.Time{}
	if feed.UpdatedParsed != nil {
		channelUpdated = *feed.UpdatedParsed
	}

	for _, item := range feed.Items {
		// Get published date, fall back to updated, fall back to channel updated.
		// Do NOT default to time.Now() — that makes old items look like new ones
		// if the feed item lacks date metadata. Prefer leaving date zeroTime so
		// the renderer can decide how to display it.
		var date time.Time
		if item.PublishedParsed != nil {
			date = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			date = *item.UpdatedParsed
		} else if feed.UpdatedParsed != nil {
			// If the channel/feed has an updated timestamp, use it as a last-resort
			date = *feed.UpdatedParsed
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

			// Additional channel metadata
			ChannelLanguage:    channelLanguage,
			TitleLanguage:      channelLanguage, // Use channel language as default
			ContentLanguage:    channelLanguage, // Use channel language as default
			ChannelAuthorName:  channelAuthorName,
			ChannelAuthorEmail: channelAuthorEmail,
			ChannelSubtitle:    channelSubtitle,
			ChannelURL:         channelURL,
			ChannelID:          channelID,
			ChannelUpdated:     channelUpdated,
			ChannelRights:      channelRights,
		}

		entries = append(entries, entry)
	}

	return entries
}
