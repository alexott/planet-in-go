package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.ini")

	content := `[Planet]
name = Test Planet
link = http://example.com
cache_directory = /tmp/cache
output_dir = /tmp/output
log_level = INFO
feed_timeout = 20
items_per_page = 15
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Planet.Name != "Test Planet" {
		t.Errorf("Planet.Name = %q, want %q", cfg.Planet.Name, "Test Planet")
	}

	if cfg.Planet.FeedTimeout != 20 {
		t.Errorf("Planet.FeedTimeout = %d, want %d", cfg.Planet.FeedTimeout, 20)
	}
}

func TestLoad_ParsesFeeds(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.ini")

	content := `[Planet]
name = Test Planet
cache_directory = /tmp/cache
output_dir = /tmp/output

[https://example.com/feed.xml]
name = Example Feed
twitter = exampleuser

[https://another.com/rss]
name = Another Feed
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Feeds) != 2 {
		t.Fatalf("len(Feeds) = %d, want 2", len(cfg.Feeds))
	}

	feed := cfg.Feeds[0]
	if feed.URL != "https://example.com/feed.xml" {
		t.Errorf("Feed[0].URL = %q, want %q", feed.URL, "https://example.com/feed.xml")
	}

	if feed.Name != "Example Feed" {
		t.Errorf("Feed[0].Name = %q, want %q", feed.Name, "Example Feed")
	}

	if feed.Extra["twitter"] != "exampleuser" {
		t.Errorf("Feed[0].Extra[twitter] = %q, want %q", feed.Extra["twitter"], "exampleuser")
	}
}
