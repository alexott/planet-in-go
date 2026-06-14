package mastodon

import (
	"testing"
	"time"

	gomastodon "github.com/mattn/go-mastodon"
)

func TestSelectCleanupCandidatesAppliesIncidentWindowAndArticleCutoff(t *testing.T) {
	opts := CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}

	articleDates := map[string]time.Time{
		normalizeArticleURL("https://example.com/old"):   time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		normalizeArticleURL("https://example.com/new"):   time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC),
		normalizeArticleURL("https://example.com/older"): time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}

	statuses := []*gomastodon.Status{
		statusWithHTML("100", time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old">old</a></p>`),
		statusWithHTML("101", time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC), `<p><a href="https://example.com/new">new</a></p>`),
		statusWithHTML("102", time.Date(2026, 6, 4, 0, 1, 0, 0, time.UTC), `<p><a href="https://example.com/older">older</a></p>`),
	}

	candidates, stats := selectCleanupCandidates(statuses, articleDates, "fosstodon.org", opts)

	if stats.Examined != 3 {
		t.Fatalf("stats.Examined = %d, want 3", stats.Examined)
	}
	if stats.InWindow != 2 {
		t.Fatalf("stats.InWindow = %d, want 2", stats.InWindow)
	}
	if stats.SkippedTooNew != 1 {
		t.Fatalf("stats.SkippedTooNew = %d, want 1", stats.SkippedTooNew)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].Status.ID != "100" {
		t.Fatalf("candidate ID = %q, want %q", candidates[0].Status.ID, "100")
	}
	if candidates[0].ArticleURL != "https://example.com/old" {
		t.Fatalf("candidate URL = %q, want %q", candidates[0].ArticleURL, "https://example.com/old")
	}
}

func TestExtractStatusURLPrefersExternalArticleLinks(t *testing.T) {
	content := `<p><a href="https://fosstodon.org/tags/clojure">#clojure</a> <a href="https://fosstodon.org/@alexott/123">thread</a> <a href="https://example.com/post">article</a></p>`

	got, ok := extractStatusURL(content, "fosstodon.org")
	if !ok {
		t.Fatal("extractStatusURL() ok = false, want true")
	}
	if got != "https://example.com/post" {
		t.Fatalf("extractStatusURL() = %q, want %q", got, "https://example.com/post")
	}
}

func statusWithHTML(id string, created time.Time, html string) *gomastodon.Status {
	return &gomastodon.Status{
		ID:        gomastodon.ID(id),
		CreatedAt: created,
		Content:   html,
	}
}

func TestBuildArticleDateIndexNormalizesLinks(t *testing.T) {
	entries := []cachedArticle{
		{Link: "https://example.com/post", Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{Link: "https://example.com/post#section", Date: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)},
	}

	index := buildArticleDateIndex(entries)
	got, ok := index[normalizeArticleURL("https://example.com/post")]
	if !ok {
		t.Fatal("normalized link missing from index")
	}
	if !got.Equal(time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("normalized date = %s, want %s", got, time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC))
	}
}

type cachedArticle struct {
	Link string
	Date time.Time
}

func buildArticleDateIndex(entries []cachedArticle) map[string]time.Time {
	index := make(map[string]time.Time, len(entries))
	for _, entry := range entries {
		index[normalizeArticleURL(entry.Link)] = entry.Date
	}
	return index
}
