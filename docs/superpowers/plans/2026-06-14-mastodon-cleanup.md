# Mastodon Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe `planet cleanup-mastodon` command that dry-runs by default, deletes only June 1-3 incident posts whose linked articles are older than May 1, and paces delete calls to stay within Mastodon’s 30-per-30-minute limit.

**Architecture:** Keep the normal posting path unchanged and add a separate cleanup flow under `internal/mastodon`. Build the cleanup path around small, testable units: candidate selection from fetched statuses, article-date lookup from the existing local cache, and a deletion limiter that throttles real delete calls. Wire that flow into a new CLI subcommand in `cmd/planet/main.go`.

**Tech Stack:** Go, `testing`, `github.com/mattn/go-mastodon`, existing `internal/cache` package, `github.com/PuerkitoBio/goquery`

---

## File map

- Modify: `cmd/planet/main.go` — add `cleanup-mastodon` command wiring, usage text, runtime integration, and override-flag parsing.
- Modify: `cmd/planet/main_test.go` — add tests for the new cleanup command defaults, override flags, and runner wiring.
- Create: `internal/mastodon/cleanup.go` — cleanup types, cache-backed article date index, Mastodon status filtering, and dry-run/apply execution.
- Create: `internal/mastodon/cleanup_test.go` — tests for status selection, URL extraction, dry-run behavior, and delete behavior.
- Create: `internal/mastodon/rate_limit.go` — rolling-window limiter for 30 deletes per 30 minutes.
- Create: `internal/mastodon/rate_limit_test.go` — pacing tests for the limiter.
- Modify: `go.mod` — promote `github.com/PuerkitoBio/goquery` to a direct dependency if `go test` updates the module metadata.
- Modify: `QUICKSTART.md` — document the cleanup command and its safe default behavior.

### Task 1: Build cleanup selection around cached article dates

**Files:**
- Create: `internal/mastodon/cleanup.go`
- Create: `internal/mastodon/cleanup_test.go`
- Test: `internal/mastodon/cleanup_test.go`

- [ ] **Step 1: Write the failing tests**

Add this new test file:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./internal/mastodon -run 'TestSelectCleanupCandidates|TestExtractStatusURL|TestBuildArticleDateIndex' -count=1
```

Expected: FAIL with undefined identifiers such as `CleanupOptions`, `selectCleanupCandidates`, `extractStatusURL`, and `normalizeArticleURL`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/mastodon/cleanup.go` with this initial implementation:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
go test ./internal/mastodon -run 'TestSelectCleanupCandidates|TestExtractStatusURL|TestBuildArticleDateIndex' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/mastodon/cleanup.go internal/mastodon/cleanup_test.go
git commit -m "feat: add Mastodon cleanup candidate selection" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 2: Add the delete limiter and cleanup execution flow

**Files:**
- Modify: `internal/mastodon/cleanup.go`
- Create: `internal/mastodon/rate_limit.go`
- Create: `internal/mastodon/rate_limit_test.go`
- Modify: `internal/mastodon/cleanup_test.go`
- Test: `internal/mastodon/rate_limit_test.go`
- Test: `internal/mastodon/cleanup_test.go`

- [ ] **Step 1: Write the failing tests**

Append these tests:

```go
func TestDeletionLimiterSleepsBeforeThirtyFirstDelete(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	var slept []time.Duration

	limiter := newDeletionLimiter(
		30,
		30*time.Minute,
		func() time.Time { return now },
		func(d time.Duration) {
			slept = append(slept, d)
			now = now.Add(d)
		},
	)

	for i := 0; i < 30; i++ {
		limiter.Wait()
		now = now.Add(10 * time.Second)
	}

	limiter.Wait()

	if len(slept) != 1 {
		t.Fatalf("len(slept) = %d, want 1", len(slept))
	}
	if slept[0] != 25*time.Minute {
		t.Fatalf("slept[0] = %s, want %s", slept[0], 25*time.Minute)
	}
}
```

And append this cleanup execution test:

```go
type fakeCleanupClient struct {
	pages    [][]*gomastodon.Status
	calls    int
	deleted  []gomastodon.ID
}

func (f *fakeCleanupClient) GetAccountCurrentUser(_ context.Context) (*gomastodon.Account, error) {
	return &gomastodon.Account{ID: "acct-1"}, nil
}

func (f *fakeCleanupClient) GetAccountStatuses(_ context.Context, _ gomastodon.ID, _ *gomastodon.Pagination) ([]*gomastodon.Status, error) {
	if f.calls >= len(f.pages) {
		return nil, nil
	}
	page := f.pages[f.calls]
	f.calls++
	return page, nil
}

func (f *fakeCleanupClient) DeleteStatus(_ context.Context, id gomastodon.ID) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestCleanerPaginatesTimelineUntilItReachesOlderStatuses(t *testing.T) {
	client := &fakeCleanupClient{
		pages: [][]*gomastodon.Status{
			{
				statusWithHTML("100", time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old-1">old-1</a></p>`),
			},
			{
				statusWithHTML("101", time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old-2">old-2</a></p>`),
				statusWithHTML("102", time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old-3">old-3</a></p>`),
			},
		},
	}

	cleaner := &Cleaner{
		client: client,
		articleDates: map[string]time.Time{
			normalizeArticleURL("https://example.com/old-1"): time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			normalizeArticleURL("https://example.com/old-2"): time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
			normalizeArticleURL("https://example.com/old-3"): time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		},
		mastodonHost: "fosstodon.org",
		limiter:      newDeletionLimiter(30, 30*time.Minute, time.Now, time.Sleep),
	}

	result, err := cleaner.Cleanup(context.Background(), CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Apply:              false,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.Matched != 2 {
		t.Fatalf("result.Matched = %d, want 2", result.Matched)
	}
	if client.calls != 2 {
		t.Fatalf("GetAccountStatuses calls = %d, want 2", client.calls)
	}
}

func TestCleanerDryRunDoesNotDeleteStatuses(t *testing.T) {
	client := &fakeCleanupClient{
		pages: [][]*gomastodon.Status{
			{
				statusWithHTML("100", time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old">old</a></p>`),
			},
		},
	}

	cleaner := &Cleaner{
		client: client,
		articleDates: map[string]time.Time{
			normalizeArticleURL("https://example.com/old"): time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		mastodonHost: "fosstodon.org",
		limiter:      newDeletionLimiter(30, 30*time.Minute, time.Now, time.Sleep),
	}

	result, err := cleaner.Cleanup(context.Background(), CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Apply:              false,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.Matched != 1 {
		t.Fatalf("result.Matched = %d, want 1", result.Matched)
	}
	if len(client.deleted) != 0 {
		t.Fatalf("len(client.deleted) = %d, want 0", len(client.deleted))
	}
}

func TestCleanerApplyDeletesMatchedStatuses(t *testing.T) {
	client := &fakeCleanupClient{
		pages: [][]*gomastodon.Status{
			{
				statusWithHTML("100", time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC), `<p><a href="https://example.com/old">old</a></p>`),
			},
		},
	}

	cleaner := &Cleaner{
		client: client,
		articleDates: map[string]time.Time{
			normalizeArticleURL("https://example.com/old"): time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		mastodonHost: "fosstodon.org",
		limiter:      newDeletionLimiter(30, 30*time.Minute, time.Now, time.Sleep),
	}

	result, err := cleaner.Cleanup(context.Background(), CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Apply:              true,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("result.Deleted = %d, want 1", result.Deleted)
	}
	if len(client.deleted) != 1 || client.deleted[0] != "100" {
		t.Fatalf("deleted IDs = %v, want [100]", client.deleted)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./internal/mastodon -run 'TestDeletionLimiter|TestCleaner(DryRun|Apply)' -count=1
```

Expected: FAIL with undefined identifiers such as `Cleaner` and `newDeletionLimiter`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/mastodon/rate_limit.go`:

```go
package mastodon

import "time"

type deletionLimiter struct {
	limit   int
	window  time.Duration
	now     func() time.Time
	sleep   func(time.Duration)
	recent  []time.Time
}

func newDeletionLimiter(limit int, window time.Duration, now func() time.Time, sleep func(time.Duration)) *deletionLimiter {
	return &deletionLimiter{
		limit:  limit,
		window: window,
		now:    now,
		sleep:  sleep,
		recent: make([]time.Time, 0, limit),
	}
}

func (l *deletionLimiter) Wait() {
	current := l.now()
	l.prune(current)

	if len(l.recent) >= l.limit {
		waitUntil := l.recent[0].Add(l.window)
		if delay := waitUntil.Sub(current); delay > 0 {
			l.sleep(delay)
		}
		current = l.now()
		l.prune(current)
	}

	l.recent = append(l.recent, current)
}

func (l *deletionLimiter) prune(current time.Time) {
	cut := 0
	for cut < len(l.recent) && !l.recent[cut].Add(l.window).After(current) {
		cut++
	}
	if cut > 0 {
		l.recent = append([]time.Time(nil), l.recent[cut:]...)
	}
}
```

Then extend `internal/mastodon/cleanup.go` with the runnable cleaner:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run:

```bash
go test ./internal/mastodon -run 'TestDeletionLimiter|TestCleaner(Paginates|DryRun|Apply)|TestSelectCleanupCandidates|TestExtractStatusURL|TestBuildArticleDateIndex' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/mastodon/cleanup.go internal/mastodon/cleanup_test.go internal/mastodon/rate_limit.go internal/mastodon/rate_limit_test.go
git commit -m "feat: add Mastodon cleanup execution flow" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 3: Wire the CLI command, safe defaults, and override flags

**Files:**
- Modify: `cmd/planet/main.go`
- Modify: `cmd/planet/main_test.go`
- Test: `cmd/planet/main_test.go`

- [ ] **Step 1: Write the failing tests**

Append these tests to `cmd/planet/main_test.go`:

```go
import (
	"context"
	"flag"
	"testing"
	"time"

	mastodonposter "github.com/alexey-ott/planet-go/internal/mastodon"
	"github.com/alexey-ott/planet-go/internal/config"
)

type fakeCleaner struct {
	gotOptions mastodonposter.CleanupOptions
}

func (f *fakeCleaner) Cleanup(_ context.Context, opts mastodonposter.CleanupOptions) (mastodonposter.CleanupResult, error) {
	f.gotOptions = opts
	return mastodonposter.CleanupResult{Matched: 2}, nil
}

func TestRunCleanupMastodonUsesSafeDefaultWindow(t *testing.T) {
	fake := &fakeCleaner{}

	oldFactory := newMastodonCleaner
	newMastodonCleaner = func(cacheDir string) (mastodonCleanupRunner, error) {
		if cacheDir != "/tmp/cache" {
			t.Fatalf("cacheDir = %q, want %q", cacheDir, "/tmp/cache")
		}
		return fake, nil
	}
	defer func() { newMastodonCleaner = oldFactory }()

	cfg := &config.Config{
		Planet: config.PlanetConfig{
			CacheDirectory: "/tmp/cache",
		},
	}

	if err := runCleanupMastodon(cfg, mastodonposter.CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Apply:              false,
	}); err != nil {
		t.Fatalf("runCleanupMastodon() error = %v", err)
	}

	if fake.gotOptions.Apply {
		t.Fatal("cleanup should default to dry-run mode")
	}
	if !fake.gotOptions.WindowStart.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("WindowStart = %s, want 2026-06-01", fake.gotOptions.WindowStart)
	}
	if !fake.gotOptions.WindowEndExclusive.Equal(time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("WindowEndExclusive = %s, want 2026-06-04", fake.gotOptions.WindowEndExclusive)
	}
	if !fake.gotOptions.ArticleBefore.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ArticleBefore = %s, want 2026-05-01", fake.gotOptions.ArticleBefore)
	}
}

func TestParseCleanupMastodonFlagsOverridesDates(t *testing.T) {
	fs := flag.NewFlagSet("cleanup-mastodon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	opts, err := parseCleanupMastodonFlags(fs, []string{
		"-apply",
		"-from", "2026-06-02T00:00:00Z",
		"-to", "2026-06-04T12:00:00Z",
		"-article-before", "2026-04-15T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("parseCleanupMastodonFlags() error = %v", err)
	}

	if !opts.Apply {
		t.Fatal("Apply = false, want true")
	}
	if !opts.WindowStart.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("WindowStart = %s, want 2026-06-02T00:00:00Z", opts.WindowStart)
	}
	if !opts.WindowEndExclusive.Equal(time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("WindowEndExclusive = %s, want 2026-06-04T12:00:00Z", opts.WindowEndExclusive)
	}
	if !opts.ArticleBefore.Equal(time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("ArticleBefore = %s, want 2026-04-15T00:00:00Z", opts.ArticleBefore)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./cmd/planet -run 'TestRunCleanupMastodonUsesSafeDefaultWindow|TestParseCleanupMastodonFlagsOverridesDates' -count=1
```

Expected: FAIL with undefined identifiers such as `mastodonCleanupRunner`, `newMastodonCleaner`, `runCleanupMastodon`, or `parseCleanupMastodonFlags`.

- [ ] **Step 3: Write the minimal implementation**

Modify `cmd/planet/main.go` with these additions.

Add the new switch case:

```go
	case "cleanup-mastodon":
		cleanupMastodonCommand(os.Args[1:])
```

Add the usage line under Commands:

```go
  cleanup-mastodon  Dry-run or delete incident-era Mastodon posts
```

Add these helpers near the other posting helpers:

```go
func defaultCleanupMastodonOptions() mastodonposter.CleanupOptions {
	return mastodonposter.CleanupOptions{
		WindowStart:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WindowEndExclusive: time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC),
		ArticleBefore:      time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
}

type mastodonCleanupRunner interface {
	Cleanup(ctx context.Context, opts mastodonposter.CleanupOptions) (mastodonposter.CleanupResult, error)
}

var newMastodonCleaner = func(cacheDir string) (mastodonCleanupRunner, error) {
	return mastodonposter.NewCleaner(cacheDir)
}

func cleanupMastodonCommand(args []string) {
	fs := flag.NewFlagSet("cleanup-mastodon", flag.ExitOnError)
	configPath := fs.String("c", "config.ini", "path to config file")
	debugMode := fs.Bool("debug", false, "enable debug logging (overrides config log_level)")
	opts, err := parseCleanupMastodonFlags(fs, args[1:])
	if err != nil {
		slog.Error("failed to parse cleanup-mastodon flags", "error", err)
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath, *debugMode)
	if err != nil {
		slog.Error("failed to load config for Mastodon cleanup", "error", err)
		os.Exit(1)
	}

	if err := runCleanupMastodon(cfg, opts); err != nil {
		slog.Error("failed to cleanup Mastodon statuses", "error", err)
		os.Exit(1)
	}
}

func parseCleanupMastodonFlags(fs *flag.FlagSet, args []string) (mastodonposter.CleanupOptions, error) {
	opts := defaultCleanupMastodonOptions()
	applyMode := fs.Bool("apply", false, "actually delete matching statuses")
	fromValue := fs.String("from", opts.WindowStart.Format(time.RFC3339), "incident window start in RFC3339")
	toValue := fs.String("to", opts.WindowEndExclusive.Format(time.RFC3339), "incident window end (exclusive) in RFC3339")
	articleBeforeValue := fs.String("article-before", opts.ArticleBefore.Format(time.RFC3339), "delete only if article date is before this RFC3339 timestamp")

	if err := fs.Parse(args); err != nil {
		return mastodonposter.CleanupOptions{}, err
	}

	var err error
	opts.WindowStart, err = time.Parse(time.RFC3339, *fromValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -from: %w", err)
	}
	opts.WindowEndExclusive, err = time.Parse(time.RFC3339, *toValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -to: %w", err)
	}
	opts.ArticleBefore, err = time.Parse(time.RFC3339, *articleBeforeValue)
	if err != nil {
		return mastodonposter.CleanupOptions{}, fmt.Errorf("parse -article-before: %w", err)
	}
	opts.Apply = *applyMode

	return opts, nil
}

func runCleanupMastodon(cfg *config.Config, opts mastodonposter.CleanupOptions) error {
	cleaner, err := newMastodonCleaner(cfg.Planet.CacheDirectory)
	if err != nil {
		return fmt.Errorf("create Mastodon cleaner: %w", err)
	}

	result, err := cleaner.Cleanup(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("cleanup Mastodon statuses: %w", err)
	}

	slog.Info("Mastodon cleanup complete",
		"examined", result.Examined,
		"in_window", result.InWindow,
		"matched", result.Matched,
		"deleted", result.Deleted,
		"apply", opts.Apply)

	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run:

```bash
go test ./cmd/planet -run 'TestRunCleanupMastodonUsesSafeDefaultWindow|TestParseCleanupMastodonFlagsOverridesDates' -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

Run:

```bash
git add cmd/planet/main.go cmd/planet/main_test.go
git commit -m "feat: add cleanup-mastodon command" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 4: Document the command and run the full verification pass

**Files:**
- Modify: `QUICKSTART.md`
- Modify: `go.mod` (only if `go test` promotes `goquery` to a direct dependency)
- Test: `./internal/mastodon/...`
- Test: `./cmd/planet`
- Test: `./...`

- [ ] **Step 1: Write the documentation change**

Add this section to `QUICKSTART.md` after the Mastodon setup/testing section:

````md
### Cleanup duplicate/old Mastodon posts

If you need to remove the June 1-3 incident posts without touching newer posts, use:

```bash
./planet cleanup-mastodon -c config.ini
```

This command is a **dry run by default**. It only targets Mastodon statuses posted from **2026-06-01** through **2026-06-03** whose linked article is older than **2026-05-01**.

To actually delete the matched posts:

```bash
./planet cleanup-mastodon -c config.ini -apply
```

The command deletes sequentially and pauses as needed to stay within Mastodon’s delete limit of **30 calls per 30 minutes**.

You can override the built-in incident window or article cutoff with RFC3339 timestamps:

```bash
./planet cleanup-mastodon -c config.ini -from 2026-06-01T00:00:00Z -to 2026-06-04T00:00:00Z -article-before 2026-05-01T00:00:00Z
```
````

- [ ] **Step 2: Run the targeted tests**

Run:

```bash
go test ./internal/mastodon ./cmd/planet -count=1
```

Expected: PASS

- [ ] **Step 3: Run the full test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS

- [ ] **Step 4: Commit**

Run:

```bash
git add QUICKSTART.md go.mod go.sum
git commit -m "docs: add Mastodon cleanup command" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
