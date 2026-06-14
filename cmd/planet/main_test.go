package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
	mastodonposter "github.com/alexey-ott/planet-go/internal/mastodon"
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
			CacheDirectory:       "/tmp/cache",
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
