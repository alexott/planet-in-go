# Mastodon Posting Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Mastodon posting alongside the existing Twitter support, with independent config and tracking, Mastodon-specific attribution and length budgeting, and command wiring that allows Twitter and Mastodon to run together safely.

**Architecture:** Keep the existing Twitter-centric structure and add Mastodon as a sibling implementation. Parse new global and per-feed config in `internal/config`, add a new `internal/mastodon` package for formatting, tracking, and API calls via `github.com/mattn/go-mastodon`, then wire `cmd/planet/main.go` so `run` and `post` can invoke one or both networks without sharing tracking state or Twitter-specific caps.

**Tech Stack:** Go, `github.com/mattn/go-mastodon`, existing `go test` test suite, file-based JSON tracking.

---

## File map

- Modify: `go.mod` — add `github.com/mattn/go-mastodon`
- Modify: `go.sum` — lock the new module version after `go get`
- Modify: `internal/config/config.go` — parse Mastodon global settings and expose a feed-level helper for `mastodon`
- Modify: `internal/config/config_test.go` — cover Mastodon config parsing and feed metadata extraction
- Modify: `internal/config/config_path_test.go` — cover the default/relative Mastodon tracking file path
- Create: `internal/mastodon/mastodon.go` — Mastodon poster, formatter, tracking persistence, first-run selection, API posting
- Create: `internal/mastodon/mastodon_test.go` — formatter, env validation, deduplication, first-run/latest-50 behavior
- Modify: `cmd/planet/main.go` — add Mastodon command wiring, shared helper functions, updated usage text
- Create: `cmd/planet/main_test.go` — test enabled-target selection, Twitter-vs-Mastodon manual-post preparation, and the Mastodon initial cap handoff
- Modify: `README.md` — describe Mastodon support in features and command examples
- Modify: `QUICKSTART.md` — document Mastodon config/env vars and per-feed `mastodon` attribution

### Task 1: Add Mastodon config surface

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/config_path_test.go`
- Test: `internal/config/config_test.go`
- Test: `internal/config/config_path_test.go`

- [ ] **Step 1: Write the failing config tests**

Add these tests to `internal/config/config_test.go` and `internal/config/config_path_test.go`:

```go
func TestLoad_ParsesMastodonSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.ini")

	content := `[Planet]
name = Test Planet
cache_directory = /tmp/cache
output_dir = /tmp/output
post_to_mastodon = true
mastodon_tracking_file = mastodon_state.json

[https://example.com/feed.xml]
name = Example Feed
mastodon = @example@fosstodon.org
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Planet.PostToMastodon {
		t.Error("Planet.PostToMastodon = false, want true")
	}
	if cfg.Planet.MastodonTrackingFile != "mastodon_state.json" {
		t.Errorf("Planet.MastodonTrackingFile = %q, want %q", cfg.Planet.MastodonTrackingFile, "mastodon_state.json")
	}
	if got := cfg.Feeds[0].MastodonHandle(); got != "@example@fosstodon.org" {
		t.Errorf("Feed.MastodonHandle() = %q, want %q", got, "@example@fosstodon.org")
	}
}

func TestFeedConfig_MastodonHandleFallback(t *testing.T) {
	feed := FeedConfig{
		URL:   "https://example.com/feed.xml",
		Name:  "Example Feed",
		Extra: map[string]string{},
	}

	if got := feed.MastodonHandle(); got != "" {
		t.Errorf("MastodonHandle() = %q, want empty string", got)
	}
}
```

```go
expectedMastodon := "test/mastodon.json"
if cfg.Planet.MastodonTrackingFile != expectedMastodon {
	t.Errorf("MastodonTrackingFile: expected %s, got %s", expectedMastodon, cfg.Planet.MastodonTrackingFile)
}
if filepath.IsAbs(cfg.Planet.MastodonTrackingFile) {
	t.Error("MastodonTrackingFile should be relative (will be resolved to cache directory at runtime)")
}
```

- [ ] **Step 2: Run the config tests to verify they fail**

Run:

```bash
go test ./internal/config -v
```

Expected: FAIL with errors about missing `PostToMastodon`, `MastodonTrackingFile`, and `MastodonHandle`.

- [ ] **Step 3: Implement the new config fields and helper**

Update `internal/config/config.go`:

```go
type PlanetConfig struct {
	Name                 string
	Link                 string
	OwnerName            string
	OwnerEmail           string
	CacheDirectory       string
	OutputDir            string
	LogLevel             string
	FeedTimeout          int
	NewFeedItems         int
	ItemsPerPage         int
	DaysPerPage          int
	DateFormat           string
	NewDateFormat        string
	Encoding             string
	TemplateFiles        []string
	Filter               string
	Exclude              string
	PostToTwitter        bool
	TwitterTrackingFile  string
	PostToMastodon       bool
	MastodonTrackingFile string
	FetchMode            string
	ParallelWorkers      int
}

func (f *FeedConfig) MastodonHandle() string {
	if handle, ok := f.Extra["mastodon"]; ok {
		return handle
	}
	return ""
}
```

In `parsePlanetSection`, add:

```go
twitterTrackingFile := section.Key("twitter_tracking_file").MustString("twitter_posted.json")
mastodonTrackingFile := section.Key("mastodon_tracking_file").MustString("mastodon_posted.json")

config.Planet = PlanetConfig{
	Name:                 section.Key("name").String(),
	Link:                 section.Key("link").String(),
	OwnerName:            section.Key("owner_name").String(),
	OwnerEmail:           section.Key("owner_email").String(),
	CacheDirectory:       cacheDir,
	OutputDir:            outputDir,
	LogLevel:             section.Key("log_level").MustString("INFO"),
	FeedTimeout:          section.Key("feed_timeout").MustInt(20),
	ItemsPerPage:         section.Key("items_per_page").MustInt(15),
	DateFormat:           strftimeToGoLayout(rawDate),
	NewDateFormat:        strftimeToGoLayout(rawNewDate),
	Encoding:             section.Key("encoding").MustString("utf-8"),
	Filter:               section.Key("filter").String(),
	Exclude:              section.Key("exclude").String(),
	PostToTwitter:        section.Key("post_to_twitter").MustBool(false),
	TwitterTrackingFile:  twitterTrackingFile,
	PostToMastodon:       section.Key("post_to_mastodon").MustBool(false),
	MastodonTrackingFile: mastodonTrackingFile,
	FetchMode:            section.Key("fetch_mode").MustString("parallel"),
	ParallelWorkers:      section.Key("parallel_workers").MustInt(10),
}
```

- [ ] **Step 4: Run the config tests again**

Run:

```bash
go test ./internal/config -v
```

Expected: PASS.

- [ ] **Step 5: Commit the config work**

Run:

```bash
git add internal/config/config.go internal/config/config_test.go internal/config/config_path_test.go
git commit -m "feat: add Mastodon config support"
```

### Task 2: Build the Mastodon poster package

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/mastodon/mastodon.go`
- Create: `internal/mastodon/mastodon_test.go`
- Test: `internal/mastodon/mastodon_test.go`

- [ ] **Step 1: Add the new dependency**

Run:

```bash
go get github.com/mattn/go-mastodon@latest
```

Expected: `go.mod` and `go.sum` are updated with `github.com/mattn/go-mastodon`.

- [ ] **Step 2: Write the failing Mastodon tests**

Create `internal/mastodon/mastodon_test.go` with these core tests:

```go
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
```

Add a helper at the bottom of the test file:

```go
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
```

- [ ] **Step 3: Run the Mastodon package tests to verify they fail**

Run:

```bash
go test ./internal/mastodon -v
```

Expected: FAIL because the package does not exist yet.

- [ ] **Step 4: Implement `internal/mastodon/mastodon.go`**

Create `internal/mastodon/mastodon.go` with this structure:

```go
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

type PostedArticle struct {
	ID                 string    `json:"id"`
	Link               string    `json:"link"`
	Title              string    `json:"title"`
	PostedAt           time.Time `json:"posted_at"`
	ArticleDate        time.Time `json:"article_date,omitempty"`
	MastodonStatusID   string    `json:"mastodon_status_id,omitempty"`
	MastodonStatusText string    `json:"mastodon_status_text,omitempty"`
}

type TrackingData struct {
	Articles []PostedArticle `json:"articles"`
}

type Poster struct {
	client       *gomastodon.Client
	trackingFile string
}

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

	return &Poster{client: client, trackingFile: trackingFile}, nil
}

func (p *Poster) loadTracking() (*TrackingData, error) {
	data := &TrackingData{Articles: make([]PostedArticle, 0)}
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
		available := maxLength - len(attribution) - 2 - urlLength
		if available < 20 {
			attribution = ""
			available = maxLength - 2 - urlLength
		}
		if available > 3 && len(title) > available {
			title = title[:available-3] + "..."
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

	newEntries := make([]cache.Entry, 0, len(sorted))
	seen := make(map[string]struct{}, len(tracking.Articles))
	for _, article := range tracking.Articles {
		seen[article.ID] = struct{}{}
	}
	for _, entry := range sorted {
		if _, ok := seen[entry.ID]; !ok {
			newEntries = append(newEntries, entry)
		}
	}

	if len(tracking.Articles) == 0 && len(newEntries) > maxInitial {
		return newEntries[len(newEntries)-maxInitial:]
	}
	return newEntries
}

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
```

- [ ] **Step 5: Run the Mastodon package tests**

Run:

```bash
go test ./internal/mastodon -v
```

Expected: PASS.

- [ ] **Step 6: Commit the Mastodon package**

Run:

```bash
git add go.mod go.sum internal/mastodon/mastodon.go internal/mastodon/mastodon_test.go
git commit -m "feat: add Mastodon poster"
```

### Task 3: Wire Mastodon into the CLI posting flow

**Files:**
- Modify: `cmd/planet/main.go`
- Create: `cmd/planet/main_test.go`
- Test: `cmd/planet/main_test.go`

- [ ] **Step 1: Write the failing CLI tests**

Create `cmd/planet/main_test.go` with these tests:

```go
package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

func TestPostingTargets(t *testing.T) {
	cfg := &config.Config{
		Planet: config.PlanetConfig{
			PostToTwitter:  true,
			PostToMastodon: true,
		},
	}

	twitterEnabled, mastodonEnabled := postingTargets(cfg)
	if !twitterEnabled || !mastodonEnabled {
		t.Fatalf("postingTargets() = (%v, %v), want (true, true)", twitterEnabled, mastodonEnabled)
	}
}

func TestPrepareTwitterPostEntriesLimitsToTen(t *testing.T) {
	entries := make([]cache.Entry, 12)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:   fmt.Sprintf("entry-%02d", i),
			Date: time.Unix(int64(i), 0),
		}
	}

	got := prepareTwitterPostEntries(entries)
	if len(got) != 10 {
		t.Fatalf("len(got) = %d, want 10", len(got))
	}
	if got[0].Date.Before(got[len(got)-1].Date) {
		t.Fatal("expected newest-first ordering")
	}
}

func TestPrepareMastodonPostEntriesKeepsAllEntries(t *testing.T) {
	entries := make([]cache.Entry, 12)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:   fmt.Sprintf("entry-%02d", i),
			Date: time.Unix(int64(i), 0),
		}
	}

	got := prepareMastodonPostEntries(entries)
	if len(got) != 12 {
		t.Fatalf("len(got) = %d, want 12", len(got))
	}
}
```

Add one injected-poster test in the same file:

```go
type fakePoster struct {
	gotTrackingFile string
	gotEntries      []cache.Entry
	gotMaxInitial   int
}

func (f *fakePoster) PostNewArticles(entries []cache.Entry, _ []config.FeedConfig, maxInitial int) error {
	f.gotEntries = append([]cache.Entry(nil), entries...)
	f.gotMaxInitial = maxInitial
	return nil
}
```

Then add:

```go
func TestPostToMastodonUsesInitialCapOf50(t *testing.T) {
	fake := &fakePoster{}

	oldFactory := newMastodonPoster
	newMastodonPoster = func(trackingFile string) (articlePoster, error) {
		fake.gotTrackingFile = trackingFile
		return fake, nil
	}
	defer func() { newMastodonPoster = oldFactory }()

	cfg := &config.Config{
		Planet: config.PlanetConfig{
			CacheDirectory:      "/tmp/cache",
			MastodonTrackingFile: "mastodon.json",
		},
	}

	entries := []cache.Entry{{ID: "entry-1", Date: time.Unix(1, 0)}}
	if err := postToMastodon(cfg, entries); err != nil {
		t.Fatalf("postToMastodon() error = %v", err)
	}
	if fake.gotMaxInitial != 50 {
		t.Fatalf("got maxInitial = %d, want 50", fake.gotMaxInitial)
	}
	if fake.gotTrackingFile != "/tmp/cache/mastodon.json" {
		t.Fatalf("tracking file = %q, want %q", fake.gotTrackingFile, "/tmp/cache/mastodon.json")
	}
}
```

- [ ] **Step 2: Run the CLI tests to verify they fail**

Run:

```bash
go test ./cmd/planet -v
```

Expected: FAIL with undefined names such as `postingTargets`, `prepareMastodonPostEntries`, `articlePoster`, and `newMastodonPoster`.

- [ ] **Step 3: Implement the CLI helpers and Mastodon wiring**

Update the imports in `cmd/planet/main.go`:

```go
import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	mastodonposter "github.com/alexey-ott/planet-go/internal/mastodon"
	"github.com/alexey-ott/planet-go/internal/fetcher"
	"github.com/alexey-ott/planet-go/internal/filter"
	"github.com/alexey-ott/planet-go/internal/renderer"
	"github.com/alexey-ott/planet-go/internal/twitter"
)
```

Add these helpers near the posting functions:

```go
type articlePoster interface {
	PostNewArticles(entries []cache.Entry, feedConfigs []config.FeedConfig, maxInitial int) error
}

var newTwitterPoster = func(trackingFile string) (articlePoster, error) {
	return twitter.NewPoster(trackingFile)
}

var newMastodonPoster = func(trackingFile string) (articlePoster, error) {
	return mastodonposter.NewPoster(trackingFile)
}

func postingTargets(cfg *config.Config) (bool, bool) {
	return cfg.Planet.PostToTwitter, cfg.Planet.PostToMastodon
}

func resolveTrackingFile(cacheDir, trackingFile string) string {
	if filepath.IsAbs(trackingFile) {
		return trackingFile
	}
	return filepath.Join(cacheDir, trackingFile)
}

func prepareTwitterPostEntries(entries []cache.Entry) []cache.Entry {
	return limitEntries(entries, 10)
}

func prepareMastodonPostEntries(entries []cache.Entry) []cache.Entry {
	return sortEntriesByDate(entries)
}
```

Add the Mastodon poster function:

```go
func postToMastodon(cfg *config.Config, entries []cache.Entry) error {
	trackingFile := resolveTrackingFile(cfg.Planet.CacheDirectory, cfg.Planet.MastodonTrackingFile)

	poster, err := newMastodonPoster(trackingFile)
	if err != nil {
		return fmt.Errorf("create Mastodon poster: %w", err)
	}

	const maxInitial = 50
	if err := poster.PostNewArticles(entries, cfg.Feeds, maxInitial); err != nil {
		return fmt.Errorf("post to Mastodon: %w", err)
	}

	return nil
}
```

Update the command text and wiring:

```go
fmt.Fprintf(os.Stderr, `Planet Go - Feed Aggregator

Usage:
  planet [command] [options]

Commands:
  run      Fetch feeds, render templates, and post to enabled networks (default)
  fetch    Fetch feeds and update cache only (no posting)
  render   Render templates from cache only (no posting)
  post     Post new articles to enabled networks from cache
  version  Show version information
`)
```

In `runFetchAndRender`, replace the Twitter-only branch with:

```go
	twitterEnabled, mastodonEnabled := postingTargets(cfg)

	if twitterEnabled {
		slog.Info("Twitter posting enabled, posting new articles")
		if err := postToTwitter(cfg, filtered); err != nil {
			slog.Error("Twitter posting failed", "error", err)
		}
	}

	if mastodonEnabled {
		slog.Info("Mastodon posting enabled, posting new articles")
		if err := postToMastodon(cfg, filtered); err != nil {
			slog.Error("Mastodon posting failed", "error", err)
		}
	}
```

In `runPost`, replace the Twitter-only logic with:

```go
	twitterEnabled, mastodonEnabled := postingTargets(cfg)
	if !twitterEnabled && !mastodonEnabled {
		slog.Warn("posting is disabled in configuration")
		fmt.Println("Posting is disabled. Enable one or both in your config.ini:")
		fmt.Println("  [Planet]")
		fmt.Println("  post_to_twitter = true")
		fmt.Println("  post_to_mastodon = true")
		return nil
	}

	if twitterEnabled {
		twitterEntries := prepareTwitterPostEntries(filtered)
		slog.Info("posting to Twitter", "entries", len(twitterEntries))
		if err := postToTwitter(cfg, twitterEntries); err != nil {
			return fmt.Errorf("post to Twitter: %w", err)
		}
	}

	if mastodonEnabled {
		mastodonEntries := prepareMastodonPostEntries(filtered)
		slog.Info("posting to Mastodon", "entries", len(mastodonEntries))
		if err := postToMastodon(cfg, mastodonEntries); err != nil {
			return fmt.Errorf("post to Mastodon: %w", err)
		}
	}
```

Also update the surrounding log strings from “Twitter only” to “enabled networks”.

- [ ] **Step 4: Run the CLI tests again**

Run:

```bash
go test ./cmd/planet -v
```

Expected: PASS.

- [ ] **Step 5: Commit the CLI wiring**

Run:

```bash
git add cmd/planet/main.go cmd/planet/main_test.go
git commit -m "feat: wire Mastodon posting into CLI"
```

### Task 4: Update docs and run full verification

**Files:**
- Modify: `README.md`
- Modify: `QUICKSTART.md`
- Test: repository test suite

- [ ] **Step 1: Update the README feature list and command examples**

In `README.md`, change the feature bullet and command examples:

```md
- **Twitter and Mastodon integration** - automatically post new articles to supported social networks
```

```bash
./planet run -c config.ini           # Fetch + render + post to enabled networks
./planet post -c config.ini          # Only post to enabled networks from cache
```

Also update the development structure snippet:

```text
├── internal/
│   ├── config/          # Configuration parsing
│   ├── cache/           # File-based caching
│   ├── fetcher/         # Feed fetching
│   ├── filter/          # Content filtering
│   ├── mastodon/        # Mastodon posting
│   ├── renderer/        # Template rendering
│   └── twitter/         # Twitter posting
```

- [ ] **Step 2: Update `QUICKSTART.md` with Mastodon setup**

Replace the Twitter-only optional posting section with a combined social-posting section containing this content:

#### Heading and intro

```md
## Social Posting (Optional)

Planet Go can automatically post new articles to Twitter, Mastodon, or both.

### Mastodon quick setup
```

#### Planet config snippet

```ini
[Planet]
post_to_mastodon = true
mastodon_tracking_file = mastodon_posted.json
```

#### Environment variables

```bash
export MASTODON_SERVER="https://fosstodon.org"
export MASTODON_CLIENT_ID="your-client-id"
export MASTODON_CLIENT_SECRET="your-client-secret"
export MASTODON_ACCESS_TOKEN="your-access-token"
```

#### Feed-level attribution

```ini
[https://blog.example.com/feed.xml]
name = Example Blog
mastodon = @example@fosstodon.org
```

#### Fallback note

```md
If `mastodon` is omitted for a feed, Planet Go will post with `(by Example Blog)` using the feed `name`.
```

Keep the existing Twitter setup directly below it so both networks are documented in one place.

- [ ] **Step 3: Run the targeted tests**

Run:

```bash
go test ./internal/config ./internal/mastodon ./cmd/planet -v
```

Expected: PASS.

- [ ] **Step 4: Run the full unit test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit docs and final verification**

Run:

```bash
git add README.md QUICKSTART.md
git commit -m "docs: document Mastodon posting"
```

## Self-review against the spec

- **Spec coverage:** the plan covers config flag + tracking file (Task 1), `mastodon` feed attribution and fallback (Tasks 1-2), `go-mastodon` integration (Task 2), separate tracking and latest-50 first run behavior (Task 2), CLI coexistence with Twitter (Task 3), and docs/testing updates (Task 4).
- **Placeholder scan:** no `TODO`, `TBD`, or “similar to previous task” shortcuts are left in the steps.
- **Type consistency:** the plan uses a consistent set of names across tasks: `PostToMastodon`, `MastodonTrackingFile`, `MastodonHandle`, `postToMastodon`, `prepareMastodonPostEntries`, `postingTargets`, and `articlePoster`.
