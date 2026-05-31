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
