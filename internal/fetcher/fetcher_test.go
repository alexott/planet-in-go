package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	fetcher := NewSequential(20, cache, false)

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

func TestSequentialFetcher_ConditionalGET(t *testing.T) {
	etag := "test-etag-123"
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// Check for If-None-Match header
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", etag)
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
	fetcher := NewSequential(20, cache, false)

	feeds := []config.FeedConfig{
		{URL: server.URL, Name: "Test Feed"},
	}

	// First fetch
	results1 := fetcher.FetchFeeds(context.Background(), feeds)
	if len(results1) != 1 || results1[0].Error != nil {
		t.Fatalf("First fetch failed: %v", results1[0].Error)
	}

	if results1[0].Cached {
		t.Error("First fetch should not be cached")
	}

	// Second fetch (should use conditional GET)
	results2 := fetcher.FetchFeeds(context.Background(), feeds)
	if len(results2) != 1 || results2[0].Error != nil {
		t.Fatalf("Second fetch failed: %v", results2[0].Error)
	}

	if !results2[0].Cached {
		t.Error("Second fetch should be cached")
	}

	if requestCount != 2 {
		t.Errorf("requestCount = %d, want 2", requestCount)
	}
}

func TestParallelFetcher_FetchFeeds(t *testing.T) {
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
	fetcher := NewParallel(20, cache, false, 5)

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

func TestParallelFetcher_MultipleFeeds(t *testing.T) {
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
	fetcher := NewParallel(20, cache, false, 3)

	// Create 10 feeds to test parallel processing
	feeds := make([]config.FeedConfig, 10)
	for i := 0; i < 10; i++ {
		feeds[i] = config.FeedConfig{URL: server.URL, Name: "Test Feed"}
	}

	results := fetcher.FetchFeeds(context.Background(), feeds)

	if len(results) != 10 {
		t.Fatalf("len(results) = %d, want 10", len(results))
	}

	// Check all results are successful
	for i, result := range results {
		if result.Error != nil {
			t.Errorf("results[%d].Error = %v, want nil", i, result.Error)
		}
		if len(result.Entries) == 0 {
			t.Errorf("results[%d] has no entries", i)
		}
	}
}

func TestParallelFetcher_ConditionalGET(t *testing.T) {
	etag := "test-etag-123"
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// Check for If-None-Match header
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", etag)
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
	fetcher := NewParallel(20, cache, false, 5)

	feeds := []config.FeedConfig{
		{URL: server.URL, Name: "Test Feed"},
	}

	// First fetch
	results1 := fetcher.FetchFeeds(context.Background(), feeds)
	if len(results1) != 1 || results1[0].Error != nil {
		t.Fatalf("First fetch failed: %v", results1[0].Error)
	}

	if results1[0].Cached {
		t.Error("First fetch should not be cached")
	}

	// Second fetch (should use conditional GET)
	results2 := fetcher.FetchFeeds(context.Background(), feeds)
	if len(results2) != 1 || results2[0].Error != nil {
		t.Fatalf("Second fetch failed: %v", results2[0].Error)
	}

	if !results2[0].Cached {
		t.Error("Second fetch should be cached")
	}

	if requestCount != 2 {
		t.Errorf("requestCount = %d, want 2", requestCount)
	}
}
