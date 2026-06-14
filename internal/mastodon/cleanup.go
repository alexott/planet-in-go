package mastodon

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/alexey-ott/planet-go/internal/cache"
	gomastodon "github.com/mattn/go-mastodon"
)

type CleanupOptions struct {
	WindowStart        time.Time
	WindowEndExclusive time.Time
	ArticleBefore      time.Time
	Apply              bool
}

type CleanupResult struct {
	Examined         int
	InWindow         int
	Matched          int
	Deleted          int
	SkippedTooNew    int
	SkippedNoURL     int
	SkippedNoArticle int
}

type cleanupCandidate struct {
	Status      *gomastodon.Status
	ArticleURL  string
	ArticleDate time.Time
}

type cleanupClient interface {
	GetAccountCurrentUser(ctx context.Context) (*gomastodon.Account, error)
	GetAccountStatuses(ctx context.Context, id gomastodon.ID, pg *gomastodon.Pagination) ([]*gomastodon.Status, error)
	DeleteStatus(ctx context.Context, id gomastodon.ID) error
}

type Cleaner struct {
	client       cleanupClient
	articleDates map[string]time.Time
	mastodonHost string
	limiter      *deletionLimiter
}

func NewCleaner(cacheDir string) (*Cleaner, error) {
	server := os.Getenv("MASTODON_SERVER")
	clientID := os.Getenv("MASTODON_CLIENT_ID")
	clientSecret := os.Getenv("MASTODON_CLIENT_SECRET")
	accessToken := os.Getenv("MASTODON_ACCESS_TOKEN")

	if server == "" || clientID == "" || clientSecret == "" || accessToken == "" {
		return nil, fmt.Errorf("missing Mastodon credentials in environment variables (MASTODON_SERVER, MASTODON_CLIENT_ID, MASTODON_CLIENT_SECRET, MASTODON_ACCESS_TOKEN)")
	}

	articleDates, err := loadArticleDateIndex(cacheDir)
	if err != nil {
		return nil, err
	}

	client := gomastodon.NewClient(&gomastodon.Config{
		Server:       server,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AccessToken:  accessToken,
	})

	parsedServer, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("parse Mastodon server URL: %w", err)
	}

	return &Cleaner{
		client:       client,
		articleDates: articleDates,
		mastodonHost: parsedServer.Host,
		limiter:      newDeletionLimiter(30, 30*time.Minute, time.Now, time.Sleep),
	}, nil
}

func selectCleanupCandidates(statuses []*gomastodon.Status, articleDates map[string]time.Time, mastodonHost string, opts CleanupOptions) ([]cleanupCandidate, CleanupResult) {
	result := CleanupResult{}
	candidates := make([]cleanupCandidate, 0)

	for _, status := range statuses {
		result.Examined++

		if status.CreatedAt.Before(opts.WindowStart) || !status.CreatedAt.Before(opts.WindowEndExclusive) {
			continue
		}

		result.InWindow++

		articleURL, ok := extractStatusURL(status.Content, mastodonHost)
		if !ok {
			result.SkippedNoURL++
			continue
		}

		articleDate, ok := articleDates[normalizeArticleURL(articleURL)]
		if !ok {
			result.SkippedNoArticle++
			continue
		}

		if !articleDate.Before(opts.ArticleBefore) {
			result.SkippedTooNew++
			continue
		}

		candidates = append(candidates, cleanupCandidate{
			Status:      status,
			ArticleURL:  articleURL,
			ArticleDate: articleDate,
		})
	}

	result.Matched = len(candidates)
	return candidates, result
}

func extractStatusURL(content string, mastodonHost string) (string, bool) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return "", false
	}

	var articleURL string
	doc.Find("a").Each(func(_ int, sel *goquery.Selection) {
		href, ok := sel.Attr("href")
		if !ok {
			return
		}

		parsed, err := url.Parse(href)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return
		}
		if strings.EqualFold(parsed.Host, mastodonHost) {
			return
		}

		articleURL = href
	})

	return articleURL, articleURL != ""
}

func normalizeArticleURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}

	parsed.Fragment = ""
	return parsed.String()
}

func (c *Cleaner) Cleanup(ctx context.Context, opts CleanupOptions) (CleanupResult, error) {
	account, err := c.client.GetAccountCurrentUser(ctx)
	if err != nil {
		return CleanupResult{}, fmt.Errorf("get current Mastodon account: %w", err)
	}

	statuses, err := c.fetchStatusesForCleanup(ctx, account.ID, opts.WindowStart)
	if err != nil {
		return CleanupResult{}, err
	}

	candidates, result := selectCleanupCandidates(statuses, c.articleDates, c.mastodonHost, opts)
	for _, candidate := range candidates {
		slog.Info("matched Mastodon status",
			"status_id", candidate.Status.ID,
			"created_at", candidate.Status.CreatedAt,
			"article_url", candidate.ArticleURL,
			"article_date", candidate.ArticleDate,
			"apply", opts.Apply)

		if !opts.Apply {
			continue
		}

		c.limiter.Wait()
		if err := c.client.DeleteStatus(ctx, candidate.Status.ID); err != nil {
			return result, fmt.Errorf("delete Mastodon status %s: %w", candidate.Status.ID, err)
		}
		result.Deleted++
	}

	return result, nil
}

func (c *Cleaner) fetchStatusesForCleanup(ctx context.Context, accountID gomastodon.ID, windowStart time.Time) ([]*gomastodon.Status, error) {
	allStatuses := make([]*gomastodon.Status, 0)
	pg := &gomastodon.Pagination{Limit: 40}

	for {
		page, err := c.client.GetAccountStatuses(ctx, accountID, pg)
		if err != nil {
			return nil, fmt.Errorf("get Mastodon statuses: %w", err)
		}
		if len(page) == 0 {
			return allStatuses, nil
		}

		allStatuses = append(allStatuses, page...)

		oldest := page[len(page)-1]
		if oldest.CreatedAt.Before(windowStart) {
			return allStatuses, nil
		}

		pg = &gomastodon.Pagination{
			Limit: 40,
			MaxID: oldest.ID,
		}
	}
}

func loadArticleDateIndex(cacheDir string) (map[string]time.Time, error) {
	entries, err := cache.New(cacheDir).LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load cached entries for Mastodon cleanup: %w", err)
	}

	index := make(map[string]time.Time, len(entries))
	for _, entry := range entries {
		index[normalizeArticleURL(entry.Link)] = entry.Date
	}

	return index, nil
}
