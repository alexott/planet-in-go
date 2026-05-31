package mastodon

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

func TestFormatStatus(t *testing.T) {
	entry := cache.Entry{
		Title: "A very good blog post",
		Link:  "https://example.com/posts/1",
	}

	feed := config.FeedConfig{
		URL:  "https://example.com/feed.xml",
		Name: "Example Blog",
		Extra: map[string]string{
			"mastodon": "@example@fosstodon.org",
		},
	}

	result := formatStatus(entry, feed)

	if !strings.Contains(result, "(by @example@fosstodon.org)") {
		t.Fatalf("expected mastodon attribution, got %q", result)
	}
	if !strings.Contains(result, "\n\nhttps://example.com/posts/1") {
		t.Fatalf("expected link on second paragraph, got %q", result)
	}
	if mastodonLength(result) > 500 {
		t.Fatalf("status too long: %d", mastodonLength(result))
	}
}

func TestFormatStatusFallsBackToFeedName(t *testing.T) {
	entry := cache.Entry{Title: "Fallback author test", Link: "https://example.com/posts/2"}
	feed := config.FeedConfig{
		URL:   "https://example.com/feed.xml",
		Name:  "Fallback Blog",
		Extra: map[string]string{},
	}

	result := formatStatus(entry, feed)
	if !strings.Contains(result, "(by Fallback Blog)") {
		t.Fatalf("expected feed-name fallback, got %q", result)
	}
}

func TestSelectEntriesForFirstRunKeepsLatest50(t *testing.T) {
	entries := make([]cache.Entry, 60)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Title: "Post",
			Link:  "https://example.com/post",
			Date:  time.Unix(int64(i), 0),
		}
	}

	selected := selectEntriesToPost(entries, &TrackingData{}, 50)
	if len(selected) != 50 {
		t.Fatalf("len(selected) = %d, want 50", len(selected))
	}
	if selected[0].Date.After(selected[len(selected)-1].Date) {
		t.Fatal("expected selected entries to remain chronological")
	}
	if selected[0].ID != "entry-10" {
		t.Fatalf("first selected ID = %q, want %q", selected[0].ID, "entry-10")
	}
}

func TestSelectEntriesToPostFiltersOnlyWithinLatest50Window(t *testing.T) {
	entries := make([]cache.Entry, 60)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Title: "Post",
			Link:  "https://example.com/post",
			Date:  time.Unix(int64(i), 0),
		}
	}

	tracking := &TrackingData{
		Articles: []PostedArticle{
			{ID: "entry-55"},
			{ID: "entry-57"},
		},
	}

	selected := selectEntriesToPost(entries, tracking, 50)

	wantIDs := []string{
		"entry-10", "entry-11", "entry-12", "entry-13", "entry-14",
		"entry-15", "entry-16", "entry-17", "entry-18", "entry-19",
		"entry-20", "entry-21", "entry-22", "entry-23", "entry-24",
		"entry-25", "entry-26", "entry-27", "entry-28", "entry-29",
		"entry-30", "entry-31", "entry-32", "entry-33", "entry-34",
		"entry-35", "entry-36", "entry-37", "entry-38", "entry-39",
		"entry-40", "entry-41", "entry-42", "entry-43", "entry-44",
		"entry-45", "entry-46", "entry-47", "entry-48", "entry-49",
		"entry-50", "entry-51", "entry-52", "entry-53", "entry-54",
		"entry-56", "entry-58", "entry-59",
	}

	if len(selected) != len(wantIDs) {
		t.Fatalf("len(selected) = %d, want %d", len(selected), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if selected[i].ID != wantID {
			t.Fatalf("selected[%d].ID = %q, want %q", i, selected[i].ID, wantID)
		}
	}
}

func TestSelectEntriesToPostIgnoresOlderUntrackedEntriesOutsideLatest50Window(t *testing.T) {
	entries := make([]cache.Entry, 60)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Title: "Post",
			Link:  "https://example.com/post",
			Date:  time.Unix(int64(i), 0),
		}
	}

	tracking := &TrackingData{
		Articles: []PostedArticle{
			{ID: "entry-10"}, {ID: "entry-11"}, {ID: "entry-12"}, {ID: "entry-13"}, {ID: "entry-14"},
			{ID: "entry-15"}, {ID: "entry-16"}, {ID: "entry-17"}, {ID: "entry-18"}, {ID: "entry-19"},
			{ID: "entry-20"}, {ID: "entry-21"}, {ID: "entry-22"}, {ID: "entry-23"}, {ID: "entry-24"},
			{ID: "entry-25"}, {ID: "entry-26"}, {ID: "entry-27"}, {ID: "entry-28"}, {ID: "entry-29"},
			{ID: "entry-30"}, {ID: "entry-31"}, {ID: "entry-32"}, {ID: "entry-33"}, {ID: "entry-34"},
			{ID: "entry-35"}, {ID: "entry-36"}, {ID: "entry-37"}, {ID: "entry-38"}, {ID: "entry-39"},
			{ID: "entry-40"}, {ID: "entry-41"}, {ID: "entry-42"}, {ID: "entry-43"}, {ID: "entry-44"},
			{ID: "entry-45"}, {ID: "entry-46"}, {ID: "entry-47"}, {ID: "entry-48"}, {ID: "entry-49"},
			{ID: "entry-50"}, {ID: "entry-51"}, {ID: "entry-52"}, {ID: "entry-53"}, {ID: "entry-54"},
			{ID: "entry-55"}, {ID: "entry-56"}, {ID: "entry-57"}, {ID: "entry-58"}, {ID: "entry-59"},
		},
	}

	selected := selectEntriesToPost(entries, tracking, 50)
	if len(selected) != 0 {
		t.Fatalf("len(selected) = %d, want 0", len(selected))
	}
}

func TestNewPosterRequiresEnvironment(t *testing.T) {
	t.Setenv("MASTODON_SERVER", "")
	t.Setenv("MASTODON_CLIENT_ID", "")
	t.Setenv("MASTODON_CLIENT_SECRET", "")
	t.Setenv("MASTODON_ACCESS_TOKEN", "")

	_, err := NewPoster(filepath.Join(t.TempDir(), "mastodon.json"))
	if err == nil {
		t.Fatal("expected missing-env error")
	}
}

func mastodonLength(text string) int {
	length := 0
	for i := 0; i < len(text); {
		if strings.HasPrefix(text[i:], "https://") || strings.HasPrefix(text[i:], "http://") {
			length += 23
			for i < len(text) && text[i] != ' ' && text[i] != '\n' {
				i++
			}
			continue
		}
		length++
		i++
	}
	return length
}
