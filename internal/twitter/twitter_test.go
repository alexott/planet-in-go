package twitter

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

func TestFormatTweet(t *testing.T) {
	tests := []struct {
		name          string
		entry         cache.Entry
		twitterHandle string
		wantContains  []string
		maxLength     int
	}{
		{
			name: "simple tweet without attribution",
			entry: cache.Entry{
				Title: "Test Article Title",
				Link:  "https://example.com/article",
			},
			twitterHandle: "",
			wantContains: []string{
				"Test Article Title",
				"https://example.com/article",
			},
			maxLength: 280,
		},
		{
			name: "tweet with attribution",
			entry: cache.Entry{
				Title: "Opening wave 2022-06-16",
				Link:  "https://fpsd.codes/opening-wave-2022-06-16.html",
			},
			twitterHandle: "@focaskater",
			wantContains: []string{
				"Opening wave 2022-06-16",
				"(by @focaskater)",
				"https://fpsd.codes/opening-wave-2022-06-16.html",
			},
			maxLength: 280,
		},
		{
			name: "very long title gets truncated",
			entry: cache.Entry{
				Title: "This is an extremely long article title that should be truncated because it exceeds the maximum allowed length for a tweet when combined with the attribution and the link URL that we need to include in the final tweet text",
				Link:  "https://example.com/very-long-url-path-that-will-be-counted-as-23-chars",
			},
			twitterHandle: "@somehandle",
			wantContains: []string{
				"...",
				"(by @somehandle)",
				"https://example.com/very-long-url-path-that-will-be-counted-as-23-chars",
			},
			maxLength: 240,
		},
		{
			name: "title with newlines and special chars",
			entry: cache.Entry{
				Title: "Clojure Deref (June 23, 2023)",
				Link:  "https://clojure.org/news/2023/06/23/",
			},
			twitterHandle: "",
			wantContains: []string{
				"Clojure Deref (June 23, 2023)",
				"https://clojure.org/news/2023/06/23/",
			},
			maxLength: 280,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTweet(tt.entry, tt.twitterHandle)

			// Check that result contains expected strings
			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("formatTweet() result should contain %q, got: %s", want, result)
				}
			}

			// Check "Twitter length" - URLs are counted as 23 chars regardless of actual length
			twitterLength := calculateTwitterLength(result)
			if twitterLength > tt.maxLength {
				t.Errorf("formatTweet() result too long: %d Twitter chars (max %d)\nResult: %s", twitterLength, tt.maxLength, result)
			}

			// Check that it has two parts separated by double newline
			if !contains(result, "\n\n") {
				t.Errorf("formatTweet() should separate title and link with double newline, got: %s", result)
			}
		})
	}
}

func TestIsPosted(t *testing.T) {
	poster := &Poster{
		trackingFile: "test_tracking.json",
	}

	tracking := &TrackingData{
		Articles: []PostedArticle{
			{
				ID:       "article-1",
				Link:     "https://example.com/1",
				PostedAt: time.Now(),
			},
			{
				ID:       "article-2",
				Link:     "https://example.com/2",
				PostedAt: time.Now(),
			},
		},
	}

	tests := []struct {
		name    string
		entryID string
		want    bool
	}{
		{
			name:    "posted article",
			entryID: "article-1",
			want:    true,
		},
		{
			name:    "another posted article",
			entryID: "article-2",
			want:    true,
		},
		{
			name:    "new article",
			entryID: "article-3",
			want:    false,
		},
		{
			name:    "empty ID",
			entryID: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := poster.isPosted(tt.entryID, tracking)
			if got != tt.want {
				t.Errorf("isPosted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTwitterHandleExtraction(t *testing.T) {
	tests := []struct {
		name   string
		config config.FeedConfig
		want   string
	}{
		{
			name: "feed with twitter handle",
			config: config.FeedConfig{
				URL:  "https://example.com/feed.xml",
				Name: "Example Feed",
				Extra: map[string]string{
					"twitter": "@example",
				},
			},
			want: "@example",
		},
		{
			name: "feed without twitter handle",
			config: config.FeedConfig{
				URL:   "https://example.com/feed.xml",
				Name:  "Example Feed",
				Extra: map[string]string{},
			},
			want: "",
		},
		{
			name: "feed with other extra fields",
			config: config.FeedConfig{
				URL:  "https://example.com/feed.xml",
				Name: "Example Feed",
				Extra: map[string]string{
					"author": "John Doe",
					"filter": "golang",
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.TwitterHandle()
			if got != tt.want {
				t.Errorf("TwitterHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// calculateTwitterLength calculates the length as Twitter counts it
// Twitter counts URLs as 23 characters regardless of actual length
func calculateTwitterLength(text string) int {
	// Find URLs starting with http:// or https://
	length := 0
	i := 0
	for i < len(text) {
		if i+8 <= len(text) && text[i:i+8] == "https://" {
			// Found HTTPS URL, count as 23 chars
			length += 23
			// Skip to end of URL (find space or newline or end of string)
			i += 8
			for i < len(text) && text[i] != ' ' && text[i] != '\n' {
				i++
			}
		} else if i+7 <= len(text) && text[i:i+7] == "http://" {
			// Found HTTP URL, count as 23 chars
			length += 23
			// Skip to end of URL
			i += 7
			for i < len(text) && text[i] != ' ' && text[i] != '\n' {
				i++
			}
		} else {
			// Regular character, count as 1
			length++
			i++
		}
	}
	return length
}

