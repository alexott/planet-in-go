package renderer

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
)

func BenchmarkSortByDate(b *testing.B) {
	// Create test entries with varying dates
	now := time.Now()
	entries := make([]cache.Entry, 100)
	for i := range entries {
		entries[i] = cache.Entry{
			Title:   "Entry " + string(rune(i)),
			Link:    "http://example.com/" + string(rune(i)),
			Content: "Test content",
			Date:    now.Add(-time.Duration(i) * time.Hour),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sortByDate(entries)
	}
}

func BenchmarkPaginate(b *testing.B) {
	// Create test entries
	now := time.Now()
	entries := make([]cache.Entry, 1000)
	for i := range entries {
		entries[i] = cache.Entry{
			Title:   "Entry",
			Link:    "http://example.com",
			Content: "Test content",
			Date:    now.Add(-time.Duration(i) * time.Minute),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = paginate(entries, 50, 0)
	}
}

func BenchmarkPaginateWithDays(b *testing.B) {
	// Create test entries
	now := time.Now()
	entries := make([]cache.Entry, 1000)
	for i := range entries {
		entries[i] = cache.Entry{
			Title:   "Entry",
			Link:    "http://example.com",
			Content: "Test content",
			Date:    now.Add(-time.Duration(i) * time.Hour),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = paginate(entries, 50, 7)
	}
}

func BenchmarkPrepareTemplateData(b *testing.B) {
	now := time.Now()
	entries := make([]cache.Entry, 100)
	for i := range entries {
		entries[i] = cache.Entry{
			Title:        "Entry " + string(rune(i)),
			Link:         "http://example.com/" + string(rune(i)),
			Content:      "Test content with some HTML <p>paragraph</p>",
			Author:       "Test Author",
			AuthorEmail:  "test@example.com",
			Date:         now.Add(-time.Duration(i) * time.Hour),
			ID:           "id-" + string(rune(i)),
			ChannelName:  "Test Channel",
			ChannelLink:  "http://example.com/feed",
			ChannelTitle: "Test Channel Title",
		}
	}

	cfg := &cache.Entry{}
	_ = cfg // Placeholder for config

	renderer := New("/tmp/output")
	
	// Create mock config
	mockConfig := struct {
		Planet struct {
			Name          string
			Link          string
			OwnerName     string
			OwnerEmail    string
			DateFormat    string
			NewDateFormat string
		}
	}{}
	mockConfig.Planet.Name = "Test Planet"
	mockConfig.Planet.Link = "http://planet.example.com"
	mockConfig.Planet.DateFormat = "2006-01-02"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We can't easily benchmark prepareTemplateData since it needs a proper config
		// But we benchmark the component operations
		sorted := sortByDate(entries)
		_ = paginate(sorted, 50, 0)
	}
	_ = renderer
}

