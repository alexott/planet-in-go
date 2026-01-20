package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/michimani/gotwi"
	"github.com/michimani/gotwi/tweet/managetweet"
	"github.com/michimani/gotwi/tweet/managetweet/types"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

// PostedArticle tracks articles that have been posted to Twitter
type PostedArticle struct {
	ID        string    `json:"id"`
	Link      string    `json:"link"`
	Title     string    `json:"title"`
	PostedAt  time.Time `json:"posted_at"`
	TweetID   string    `json:"tweet_id,omitempty"`
	TweetText string    `json:"tweet_text,omitempty"`
}

// TrackingData stores all posted articles
type TrackingData struct {
	Articles []PostedArticle `json:"articles"`
}

// Poster handles posting articles to Twitter
type Poster struct {
	client       *gotwi.Client
	trackingFile string
}

// NewPoster creates a new Twitter poster with OAuth 1.0a authentication
func NewPoster(trackingFile string) (*Poster, error) {
	apiKey := os.Getenv("TWITTER_API_KEY")
	apiKeySecret := os.Getenv("TWITTER_API_KEY_SECRET")
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	accessTokenSecret := os.Getenv("TWITTER_ACCESS_TOKEN_SECRET")

	if apiKey == "" || apiKeySecret == "" || accessToken == "" || accessTokenSecret == "" {
		return nil, fmt.Errorf("missing Twitter credentials in environment variables (TWITTER_API_KEY, TWITTER_API_KEY_SECRET, TWITTER_ACCESS_TOKEN, TWITTER_ACCESS_TOKEN_SECRET)")
	}

	slog.Debug("Initializing Twitter client",
		"api_key_set", apiKey != "",
		"access_token_set", accessToken != "")

	in := &gotwi.NewClientInput{
		AuthenticationMethod: gotwi.AuthenMethodOAuth1UserContext,
		OAuthToken:           accessToken,
		OAuthTokenSecret:     accessTokenSecret,
		APIKey:               apiKey,
		APIKeySecret:         apiKeySecret,
	}

	client, err := gotwi.NewClient(in)
	if err != nil {
		return nil, fmt.Errorf("create Twitter client: %w", err)
	}

	slog.Info("Twitter client initialized successfully")

	return &Poster{
		client:       client,
		trackingFile: trackingFile,
	}, nil
}

// loadTracking reads the tracking file
func (p *Poster) loadTracking() (*TrackingData, error) {
	data := &TrackingData{
		Articles: make([]PostedArticle, 0),
	}

	if _, err := os.Stat(p.trackingFile); os.IsNotExist(err) {
		slog.Info("Tracking file does not exist, will be created", "path", p.trackingFile)
		return data, nil
	}

	content, err := os.ReadFile(p.trackingFile)
	if err != nil {
		return nil, fmt.Errorf("read tracking file: %w", err)
	}

	if len(content) == 0 {
		slog.Info("Tracking file is empty, starting fresh")
		return data, nil
	}

	if err := json.Unmarshal(content, data); err != nil {
		return nil, fmt.Errorf("unmarshal tracking data: %w", err)
	}

	slog.Info("Loaded tracking data", "posted_articles", len(data.Articles))
	return data, nil
}

// saveTracking writes the tracking file
func (p *Poster) saveTracking(data *TrackingData) error {
	// Ensure directory exists
	dir := filepath.Dir(p.trackingFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create tracking directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking data: %w", err)
	}

	if err := os.WriteFile(p.trackingFile, content, 0644); err != nil {
		return fmt.Errorf("write tracking file: %w", err)
	}

	slog.Info("Saved tracking data", "path", p.trackingFile, "total_articles", len(data.Articles))
	return nil
}

// isPosted checks if an article has already been posted
func (p *Poster) isPosted(entryID string, tracking *TrackingData) bool {
	for _, article := range tracking.Articles {
		if article.ID == entryID {
			return true
		}
	}
	return false
}

// formatTweet creates the tweet text according to the specification
func formatTweet(entry cache.Entry, twitterHandle string) string {
	// Start with the title
	title := entry.Title

	// Build attribution
	attribution := ""
	if twitterHandle != "" {
		// Handle multiple Twitter handles separated by commas
		handles := strings.Split(twitterHandle, ",")
		formattedHandles := make([]string, 0, len(handles))
		for _, handle := range handles {
			handle = strings.TrimSpace(handle)
			if handle != "" {
				formattedHandles = append(formattedHandles, "@"+handle)
			}
		}
		if len(formattedHandles) > 0 {
			attribution = fmt.Sprintf(" (by %s)", strings.Join(formattedHandles, ", "))
		}
	}

	// Add link
	link := entry.Link

	// Calculate total length (Twitter counts URLs as 23 chars regardless of actual length)
	urlLength := 23
	// Format is: "title + attribution + \n\n + url"
	totalLength := len(title) + len(attribution) + 2 + urlLength // +2 for double newline

	// Twitter limit is 280, but we want to keep it under 240 for safety and readability
	maxLength := 240

	// If too long, shorten the title
	if totalLength > maxLength {
		// Calculate how much space we have for the title
		availableForTitle := maxLength - len(attribution) - urlLength - 2 // -2 for double newline
		if availableForTitle < 20 {
			// If we can't fit at least 20 chars of title, remove attribution
			attribution = ""
			availableForTitle = maxLength - urlLength - 2
		}

		if len(title) > availableForTitle {
			// Truncate title and add ellipsis
			title = title[:availableForTitle-3] + "..."
		}
	}

	// Format the tweet
	if attribution != "" {
		return fmt.Sprintf("%s%s\n\n%s", title, attribution, link)
	}
	return fmt.Sprintf("%s\n\n%s", title, link)
}

// PostNewArticles posts new articles to Twitter
func (p *Poster) PostNewArticles(entries []cache.Entry, feedConfigs []config.FeedConfig, maxInitial int) error {
	slog.Info("Starting Twitter posting process", "total_entries", len(entries))

	// Load tracking data
	tracking, err := p.loadTracking()
	if err != nil {
		return fmt.Errorf("load tracking: %w", err)
	}

	isFirstRun := len(tracking.Articles) == 0

	// Create a map of feed URL to config for quick lookup
	feedConfigMap := make(map[string]config.FeedConfig)
	for _, fc := range feedConfigs {
		feedConfigMap[fc.URL] = fc
	}

	// Sort entries by date (oldest first) so we post in chronological order
	sortedEntries := make([]cache.Entry, len(entries))
	copy(sortedEntries, entries)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].Date.Before(sortedEntries[j].Date)
	})

	// Find new articles
	newArticles := make([]cache.Entry, 0)
	for _, entry := range sortedEntries {
		if !p.isPosted(entry.ID, tracking) {
			newArticles = append(newArticles, entry)
		}
	}

	slog.Info("Found new articles", "count", len(newArticles), "is_first_run", isFirstRun)

	// Limit to maxInitial on first run
	articlesToPost := newArticles
	if isFirstRun && len(newArticles) > maxInitial {
		// Take the most recent maxInitial articles
		articlesToPost = newArticles[len(newArticles)-maxInitial:]
		slog.Info("First run: limiting to most recent articles", "count", maxInitial)
	}

	if len(articlesToPost) == 0 {
		slog.Info("No new articles to post")
		return nil
	}

	// Post each article
	posted := 0
	for _, entry := range articlesToPost {
		// Get Twitter handle for this feed
		twitterHandle := ""
		if feedConfig, ok := feedConfigMap[entry.ChannelURL]; ok {
			twitterHandle = feedConfig.TwitterHandle()
		}

		// Format tweet
		tweetText := formatTweet(entry, twitterHandle)

		slog.Info("Posting to Twitter",
			"title", entry.Title,
			"link", entry.Link,
			"twitter_handle", twitterHandle,
			"tweet_length", len(tweetText))

		slog.Debug("Tweet content", "text", tweetText)

		// Post to Twitter
		tweetID, err := p.postTweet(tweetText)
		if err != nil {
			slog.Error("Failed to post tweet",
				"error", err,
				"title", entry.Title,
				"link", entry.Link)
			// Continue with other articles even if one fails
			continue
		}

		// Record the posted article
		article := PostedArticle{
			ID:        entry.ID,
			Link:      entry.Link,
			Title:     entry.Title,
			PostedAt:  time.Now(),
			TweetID:   tweetID,
			TweetText: tweetText,
		}
		tracking.Articles = append(tracking.Articles, article)
		posted++

		slog.Info("Successfully posted to Twitter",
			"title", entry.Title,
			"tweet_id", tweetID)

		// Save after each successful post to avoid losing progress
		if err := p.saveTracking(tracking); err != nil {
			slog.Error("Failed to save tracking after post", "error", err)
		}

		// Add a small delay between posts to avoid rate limiting
		time.Sleep(2 * time.Second)
	}

	slog.Info("Twitter posting complete", "posted", posted, "failed", len(articlesToPost)-posted)
	return nil
}

// postTweet sends a tweet and returns the tweet ID
func (p *Poster) postTweet(text string) (string, error) {
	input := &types.CreateInput{
		Text: gotwi.String(text),
	}

	res, err := managetweet.Create(context.Background(), p.client, input)
	if err != nil {
		// Try to extract more error details
		if gotwiErr, ok := err.(*gotwi.GotwiError); ok {
			var details strings.Builder
			details.WriteString(fmt.Sprintf("Twitter API error: %s", gotwiErr.Error()))
			if gotwiErr.OnAPI {
				details.WriteString(fmt.Sprintf(" (Status: %d, Title: %s)", gotwiErr.StatusCode, gotwiErr.Title))
				for _, apiErr := range gotwiErr.APIErrors {
					details.WriteString(fmt.Sprintf(", %s", apiErr.Message))
				}
			}
			return "", fmt.Errorf("%s", details.String())
		}
		return "", fmt.Errorf("post tweet: %w", err)
	}

	if res.Data.ID == nil {
		return "", fmt.Errorf("no tweet ID in response")
	}

	return *res.Data.ID, nil
}
