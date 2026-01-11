package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathResolution(t *testing.T) {
	// Create a temporary directory and change to it
	tmpDir := t.TempDir()
	// Resolve any symlinks in tmpDir (e.g., on macOS /var/folders vs /private/var/folders)
	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)

	os.Chdir(tmpDir)

	// Create config file in temp dir
	configPath := filepath.Join(tmpDir, "test_config.ini")
	configContent := `[Planet]
name = Test Planet
cache_directory = test/cache
output_dir = test/output
template_files = test/template1.html.tmpl test/template2.xml.tmpl
twitter_tracking_file = test/twitter.json

[test/template1.html.tmpl]
days_per_page = 5

[https://example.com/feed.xml]
name = Example Feed
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test that paths are resolved relative to CWD (tmpDir)
	expectedCache := filepath.Join(tmpDir, "test", "cache")
	if cfg.Planet.CacheDirectory != expectedCache {
		t.Errorf("CacheDirectory: expected %s, got %s", expectedCache, cfg.Planet.CacheDirectory)
	}

	expectedOutput := filepath.Join(tmpDir, "test", "output")
	if cfg.Planet.OutputDir != expectedOutput {
		t.Errorf("OutputDir: expected %s, got %s", expectedOutput, cfg.Planet.OutputDir)
	}

	expectedTwitter := "test/twitter.json"
	if cfg.Planet.TwitterTrackingFile != expectedTwitter {
		t.Errorf("TwitterTrackingFile: expected %s, got %s", expectedTwitter, cfg.Planet.TwitterTrackingFile)
	}

	// Twitter tracking file should be relative (resolved at runtime to cache dir)
	if filepath.IsAbs(cfg.Planet.TwitterTrackingFile) {
		t.Error("TwitterTrackingFile should be relative (will be resolved to cache directory at runtime)")
	}

	// Test template files
	if len(cfg.Planet.TemplateFiles) != 2 {
		t.Fatalf("expected 2 template files, got %d", len(cfg.Planet.TemplateFiles))
	}

	expectedTmpl1 := filepath.Join(tmpDir, "test", "template1.html.tmpl")
	if cfg.Planet.TemplateFiles[0] != expectedTmpl1 {
		t.Errorf("Template[0]: expected %s, got %s", expectedTmpl1, cfg.Planet.TemplateFiles[0])
	}

	expectedTmpl2 := filepath.Join(tmpDir, "test", "template2.xml.tmpl")
	if cfg.Planet.TemplateFiles[1] != expectedTmpl2 {
		t.Errorf("Template[1]: expected %s, got %s", expectedTmpl2, cfg.Planet.TemplateFiles[1])
	}

	// Test that paths are absolute
	if !filepath.IsAbs(cfg.Planet.CacheDirectory) {
		t.Error("CacheDirectory should be absolute")
	}
	if !filepath.IsAbs(cfg.Planet.OutputDir) {
		t.Error("OutputDir should be absolute")
	}
	for i, tmpl := range cfg.Planet.TemplateFiles {
		if !filepath.IsAbs(tmpl) {
			t.Errorf("Template[%d] should be absolute", i)
		}
	}

	// Test template-specific config
	if len(cfg.Templates) != 1 {
		t.Fatalf("expected 1 template config, got %d", len(cfg.Templates))
	}

	tmplConfigKey := filepath.Join(tmpDir, "test", "template1.html.tmpl")
	tmplCfg, ok := cfg.Templates[tmplConfigKey]
	if !ok {
		t.Errorf("template config not found for key: %s", tmplConfigKey)
		t.Logf("Available template configs:")
		for k := range cfg.Templates {
			t.Logf("  %s", k)
		}
	} else if tmplCfg.DaysPerPage != 5 {
		t.Errorf("template days_per_page: expected 5, got %d", tmplCfg.DaysPerPage)
	}
}

func TestAbsolutePathsPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)

	os.Chdir(tmpDir)

	// Create config with absolute paths
	absCache := "/absolute/cache"
	absOutput := "/absolute/output"
	absTemplate := "/absolute/template.html.tmpl"

	configPath := filepath.Join(tmpDir, "test_abs_config.ini")
	configContent := `[Planet]
name = Test Planet
cache_directory = ` + absCache + `
output_dir = ` + absOutput + `
template_files = ` + absTemplate + `
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Absolute paths should be preserved
	if cfg.Planet.CacheDirectory != absCache {
		t.Errorf("CacheDirectory: expected %s, got %s", absCache, cfg.Planet.CacheDirectory)
	}
	if cfg.Planet.OutputDir != absOutput {
		t.Errorf("OutputDir: expected %s, got %s", absOutput, cfg.Planet.OutputDir)
	}
	if len(cfg.Planet.TemplateFiles) != 1 || cfg.Planet.TemplateFiles[0] != absTemplate {
		t.Errorf("Template: expected %s, got %v", absTemplate, cfg.Planet.TemplateFiles)
	}
}
