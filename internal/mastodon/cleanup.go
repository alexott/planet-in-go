package mastodon

import (
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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
