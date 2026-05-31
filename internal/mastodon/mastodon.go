package mastodon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gomastodon "github.com/mattn/go-mastodon"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

// PostedArticle tracks articles that have been posted to Mastodon.
type PostedArticle struct {
	ID                 string    `json:"id"`
	Link               string    `json:"link"`
	Title              string    `json:"title"`
	PostedAt           time.Time `json:"posted_at"`
	ArticleDate        time.Time `json:"article_date,omitempty"`
	MastodonStatusID   string    `json:"mastodon_status_id,omitempty"`
	MastodonStatusText string    `json:"mastodon_status_text,omitempty"`
}

// TrackingData stores all posted articles.
type TrackingData struct {
	Articles []PostedArticle `json:"articles"`
}

// Poster handles posting articles to Mastodon.
type Poster struct {
	client       *gomastodon.Client
	trackingFile string
}

// NewPoster creates a new Mastodon poster using token-based authentication.
func NewPoster(trackingFile string) (*Poster, error) {
	server := os.Getenv("MASTODON_SERVER")
	clientID := os.Getenv("MASTODON_CLIENT_ID")
	clientSecret := os.Getenv("MASTODON_CLIENT_SECRET")
	accessToken := os.Getenv("MASTODON_ACCESS_TOKEN")

	if server == "" || clientID == "" || clientSecret == "" || accessToken == "" {
		return nil, fmt.Errorf("missing Mastodon credentials in environment variables (MASTODON_SERVER, MASTODON_CLIENT_ID, MASTODON_CLIENT_SECRET, MASTODON_ACCESS_TOKEN)")
	}

	client := gomastodon.NewClient(&gomastodon.Config{
		Server:       server,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  accessToken,
	})

	return &Poster{
		client:       client,
		trackingFile: trackingFile,
	}, nil
}

func (p *Poster) loadTracking() (*TrackingData, error) {
	data := &TrackingData{
		Articles: make([]PostedArticle, 0),
	}

	if _, err := os.Stat(p.trackingFile); os.IsNotExist(err) {
		return data, nil
	}

	content, err := os.ReadFile(p.trackingFile)
	if err != nil {
		return nil, fmt.Errorf("read tracking file: %w", err)
	}
	if len(content) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(content, data); err != nil {
		return nil, fmt.Errorf("unmarshal tracking data: %w", err)
	}
	return data, nil
}

func (p *Poster) saveTracking(data *TrackingData) error {
	if err := os.MkdirAll(filepath.Dir(p.trackingFile), 0755); err != nil {
		return fmt.Errorf("create tracking directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tracking data: %w", err)
	}
	if err := os.WriteFile(p.trackingFile, content, 0644); err != nil {
		return fmt.Errorf("write tracking file: %w", err)
	}
	return nil
}

func (p *Poster) isPosted(entryID string, tracking *TrackingData) bool {
	for _, article := range tracking.Articles {
		if article.ID == entryID {
			return true
		}
	}
	return false
}

func authorAttribution(feed config.FeedConfig) string {
	if handle := strings.TrimSpace(feed.MastodonHandle()); handle != "" {
		return fmt.Sprintf(" (by %s)", handle)
	}
	if name := strings.TrimSpace(feed.Name); name != "" {
		return fmt.Sprintf(" (by %s)", name)
	}
	return ""
}

func formatStatus(entry cache.Entry, feed config.FeedConfig) string {
	title := entry.Title
	attribution := authorAttribution(feed)

	const urlLength = 23
	const maxLength = 500

	if mastodonLengthForBudget(title+attribution+"\n\n"+entry.Link) > maxLength {
		availableForTitle := maxLength - len(attribution) - urlLength - 2
		if availableForTitle < 20 {
			attribution = ""
			availableForTitle = maxLength - urlLength - 2
		}
		if len(title) > availableForTitle && availableForTitle > 3 {
			title = title[:availableForTitle-3] + "..."
		}
	}

	if attribution != "" {
		return fmt.Sprintf("%s%s\n\n%s", title, attribution, entry.Link)
	}
	return fmt.Sprintf("%s\n\n%s", title, entry.Link)
}

func mastodonLengthForBudget(text string) int {
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

func selectEntriesToPost(entries []cache.Entry, tracking *TrackingData, maxInitial int) []cache.Entry {
	sorted := make([]cache.Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	window := sorted
	if len(window) > maxInitial {
		window = window[len(window)-maxInitial:]
	}

	newEntries := make([]cache.Entry, 0, len(window))
	for _, entry := range window {
		if !(&Poster{}).isPosted(entry.ID, tracking) {
			newEntries = append(newEntries, entry)
		}
	}

	return newEntries
}

// PostNewArticles posts new articles to Mastodon.
func (p *Poster) PostNewArticles(entries []cache.Entry, feedConfigs []config.FeedConfig, maxInitial int) error {
	tracking, err := p.loadTracking()
	if err != nil {
		return fmt.Errorf("load tracking: %w", err)
	}

	feedConfigMap := make(map[string]config.FeedConfig, len(feedConfigs))
	for _, feed := range feedConfigs {
		feedConfigMap[feed.URL] = feed
	}

	toPost := selectEntriesToPost(entries, tracking, maxInitial)
	for _, entry := range toPost {
		feed := feedConfigMap[entry.ChannelURL]
		statusText := formatStatus(entry, feed)

		status, err := p.client.PostStatus(context.Background(), &gomastodon.Toot{
			Status:     statusText,
			Visibility: gomastodon.VisibilityPublic,
		})
		if err != nil {
			return fmt.Errorf("post status for %q: %w", entry.Title, err)
		}

		tracking.Articles = append(tracking.Articles, PostedArticle{
			ID:                 entry.ID,
			Link:               entry.Link,
			Title:              entry.Title,
			PostedAt:           time.Now(),
			ArticleDate:        entry.Date,
			MastodonStatusID:   string(status.ID),
			MastodonStatusText: statusText,
		})
	}

	return p.saveTracking(tracking)
}
