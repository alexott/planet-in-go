package cache

import (
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

func TestCache_LoadAll(t *testing.T) {
	tmpDir := t.TempDir()
	cache := New(tmpDir)

	// Save entries from multiple feeds
	feed1 := "https://example.com/feed1.xml"
	feed2 := "https://example.com/feed2.xml"

	entries1 := []Entry{
		{Title: "Entry 1", Link: "http://example.com/1", Date: time.Now(), ID: "1"},
		{Title: "Entry 2", Link: "http://example.com/2", Date: time.Now(), ID: "2"},
	}

	entries2 := []Entry{
		{Title: "Entry 3", Link: "http://example.com/3", Date: time.Now(), ID: "3"},
	}

	if err := cache.SaveEntries(feed1, entries1); err != nil {
		t.Fatal(err)
	}

	if err := cache.SaveEntries(feed2, entries2); err != nil {
		t.Fatal(err)
	}

	// Load all entries
	allEntries, err := cache.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if len(allEntries) != 3 {
		t.Fatalf("len(allEntries) = %d, want 3", len(allEntries))
	}
}

func TestCache_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	cache := New(tmpDir)

	feedURL := "https://example.com/feed.xml"

	meta := Metadata{
		LastFetched:  time.Now(),
		ETag:         "test-etag",
		LastModified: "Mon, 01 Jan 2024 00:00:00 GMT",
	}

	// Save metadata
	err := cache.SaveMetadata(feedURL, meta)
	if err != nil {
		t.Fatalf("SaveMetadata() error = %v", err)
	}

	// Load metadata
	loadedMeta, err := cache.LoadMetadata(feedURL)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}

	if loadedMeta == nil {
		t.Fatal("LoadMetadata() returned nil")
	}

	if loadedMeta.ETag != meta.ETag {
		t.Errorf("ETag = %q, want %q", loadedMeta.ETag, meta.ETag)
	}

	if loadedMeta.LastModified != meta.LastModified {
		t.Errorf("LastModified = %q, want %q", loadedMeta.LastModified, meta.LastModified)
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "simple https URL",
			url:  "https://go.dev/blog/feed.atom",
			want: "go.dev-blog-feed.atom",
		},
		{
			name: "http URL with path",
			url:  "http://example.com/rss/feed.xml",
			want: "example.com-rss-feed.xml",
		},
		{
			name: "URL with query params",
			url:  "https://example.com/feed?format=rss&lang=en",
			want: "example.com-feed-format-rss-lang-en",
		},
		{
			name: "URL with special characters",
			url:  "https://example.com/blog/2024/01/my-post!@#$%^&*()",
			want: "example.com-blog-2024-01-my-post",
		},
		{
			name: "URL with trailing slash",
			url:  "https://example.com/feed/",
			want: "example.com-feed",
		},
		{
			name: "URL with multiple slashes",
			url:  "https://example.com/path/to/feed",
			want: "example.com-path-to-feed",
		},
		{
			name: "URL with port",
			url:  "https://example.com:8080/feed.xml",
			want: "example.com-8080-feed.xml",
		},
		{
			name: "subdomain",
			url:  "https://blog.example.com/feed.xml",
			want: "blog.example.com-feed.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeURL(tt.url)
			if got != tt.want {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
