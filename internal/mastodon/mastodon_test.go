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
