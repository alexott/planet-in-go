package filter

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
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

func TestFilter_Apply_NoFilter(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Entry 1", Content: "Content 1", Date: time.Now()},
		{Title: "Entry 2", Content: "Content 2", Date: time.Now()},
	}

	filter, err := New("", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
}

func TestFilter_Apply_BothFilters(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Good Clojure Post", Content: "Learn Clojure", Date: time.Now()},
		{Title: "Bad Clojure Spam", Content: "Buy Clojure now!", Date: time.Now()},
		{Title: "Good Python Post", Content: "Learn Python", Date: time.Now()},
	}

	filter, err := New("Clojure", "Spam")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}

	if filtered[0].Title != "Good Clojure Post" {
		t.Errorf("filtered[0].Title = %q, want %q", filtered[0].Title, "Good Clojure Post")
	}
}

func TestApplyPerFeed_FeedLevelFilter(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Clojure Post", Content: "Learn Clojure", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Python Post", Content: "Learn Python", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Go Post", Content: "Learn Go", ChannelURL: "https://blog2.com/feed", Date: time.Now()},
		{Title: "Rust Post", Content: "Learn Rust", ChannelURL: "https://blog2.com/feed", Date: time.Now()},
	}

	feedConfigs := []config.FeedConfig{
		{
			URL:  "https://blog1.com/feed",
			Name: "Blog 1",
			Extra: map[string]string{
				"filter": "Clojure",
			},
		},
		{
			URL:  "https://blog2.com/feed",
			Name: "Blog 2",
			Extra: map[string]string{
				"filter": "Go",
			},
		},
	}

	filtered, err := ApplyPerFeed(entries, feedConfigs, "", "")
	if err != nil {
		t.Fatalf("ApplyPerFeed() error = %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	// Should have "Clojure Post" and "Go Post"
	titles := make(map[string]bool)
	for _, entry := range filtered {
		titles[entry.Title] = true
	}

	if !titles["Clojure Post"] || !titles["Go Post"] {
		t.Errorf("wrong entries filtered: got %v", titles)
	}
}

func TestApplyPerFeed_GlobalFilter(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Clojure Post", Content: "Learn Clojure", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Python Post", Content: "Learn Python", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Clojure Tips", Content: "Advanced tips", ChannelURL: "https://blog2.com/feed", Date: time.Now()},
	}

	feedConfigs := []config.FeedConfig{
		{
			URL:   "https://blog1.com/feed",
			Name:  "Blog 1",
			Extra: map[string]string{},
		},
		{
			URL:   "https://blog2.com/feed",
			Name:  "Blog 2",
			Extra: map[string]string{},
		},
	}

	filtered, err := ApplyPerFeed(entries, feedConfigs, "Clojure", "")
	if err != nil {
		t.Fatalf("ApplyPerFeed() error = %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	// Should have both Clojure posts
	for _, entry := range filtered {
		if entry.Title != "Clojure Post" && entry.Title != "Clojure Tips" {
			t.Errorf("unexpected entry: %q", entry.Title)
		}
	}
}

func TestApplyPerFeed_FeedOverridesGlobal(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Clojure Post", Content: "Learn Clojure", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Python Post", Content: "Learn Python", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Go Post", Content: "Learn Go", ChannelURL: "https://blog2.com/feed", Date: time.Now()},
	}

	feedConfigs := []config.FeedConfig{
		{
			URL:  "https://blog1.com/feed",
			Name: "Blog 1",
			Extra: map[string]string{
				"filter": "Python", // Feed-level filter overrides global
			},
		},
		{
			URL:   "https://blog2.com/feed",
			Name:  "Blog 2",
			Extra: map[string]string{}, // Uses global filter
		},
	}

	filtered, err := ApplyPerFeed(entries, feedConfigs, "Clojure", "")
	if err != nil {
		t.Fatalf("ApplyPerFeed() error = %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}

	// Blog1 should use "Python" filter (feed-level), Blog2 should use "Clojure" filter (global)
	// Only "Python Post" should match
	if filtered[0].Title != "Python Post" {
		t.Errorf("filtered[0].Title = %q, want %q", filtered[0].Title, "Python Post")
	}
}

func TestApplyPerFeed_NoFilterForFeed(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Any Post", Content: "Any content", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Another Post", Content: "More content", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
	}

	feedConfigs := []config.FeedConfig{
		{
			URL:   "https://blog1.com/feed",
			Name:  "Blog 1",
			Extra: map[string]string{},
		},
	}

	// No global or feed-level filters
	filtered, err := ApplyPerFeed(entries, feedConfigs, "", "")
	if err != nil {
		t.Fatalf("ApplyPerFeed() error = %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2 (all entries should pass)", len(filtered))
	}
}

func TestApplyPerFeed_ExcludePattern(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Good Post", Content: "Quality content", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
		{Title: "Spam Post", Content: "Buy now!", ChannelURL: "https://blog1.com/feed", Date: time.Now()},
	}

	feedConfigs := []config.FeedConfig{
		{
			URL:  "https://blog1.com/feed",
			Name: "Blog 1",
			Extra: map[string]string{
				"exclude": "Spam",
			},
		},
	}

	filtered, err := ApplyPerFeed(entries, feedConfigs, "", "")
	if err != nil {
		t.Fatalf("ApplyPerFeed() error = %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}

	if filtered[0].Title != "Good Post" {
		t.Errorf("filtered[0].Title = %q, want %q", filtered[0].Title, "Good Post")
	}
}
