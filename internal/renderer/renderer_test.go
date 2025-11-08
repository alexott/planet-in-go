package renderer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

func TestSortByDate(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		{Title: "Old", Date: now.Add(-2 * time.Hour)},
		{Title: "New", Date: now},
		{Title: "Middle", Date: now.Add(-1 * time.Hour)},
	}

	sorted := sortByDate(entries)

	if sorted[0].Title != "New" {
		t.Errorf("sorted[0].Title = %q, want New", sorted[0].Title)
	}
	if sorted[1].Title != "Middle" {
		t.Errorf("sorted[1].Title = %q, want Middle", sorted[1].Title)
	}
	if sorted[2].Title != "Old" {
		t.Errorf("sorted[2].Title = %q, want Old", sorted[2].Title)
	}
}

func TestPaginate(t *testing.T) {
	entries := make([]cache.Entry, 20)
	for i := range entries {
		entries[i] = cache.Entry{Title: "Entry", Date: time.Now()}
	}

	paginated := paginate(entries, 10, 0)

	if len(paginated) != 10 {
		t.Errorf("len(paginated) = %d, want 10", len(paginated))
	}
}

func TestPaginate_DaysPerPage(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		{Title: "Recent", Date: now.Add(-1 * time.Hour)},
		{Title: "Yesterday", Date: now.Add(-25 * time.Hour)},
		{Title: "Old", Date: now.Add(-72 * time.Hour)},
	}

	// Only keep entries from last 2 days
	paginated := paginate(entries, 0, 2)

	if len(paginated) != 2 {
		t.Errorf("len(paginated) = %d, want 2", len(paginated))
	}
}

func TestRenderer_Render(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	tmplPath := filepath.Join(tmpDir, "test.html.tmpl")

	// Create simple template
	tmplContent := `<h1>{{.Name}}</h1>
{{range .Items}}
<div>{{.Title}}</div>
{{end}}`

	if err := os.WriteFile(tmplPath, []byte(tmplContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create config
	cfg := &config.Config{
		Planet: config.PlanetConfig{
			Name:          "Test Planet",
			Link:          "http://example.com",
			ItemsPerPage:  10,
			DateFormat:    "2006-01-02",
			NewDateFormat: "2006-01-02",
		},
	}

	// Create test entries
	entries := []cache.Entry{
		{Title: "Entry 1", Link: "http://example.com/1", Date: time.Now(), Content: "Content 1"},
		{Title: "Entry 2", Link: "http://example.com/2", Date: time.Now(), Content: "Content 2"},
	}

	// Render
	renderer := New(outputDir)
	err := renderer.Render(tmplPath, entries, cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Check output file exists
	outputPath := filepath.Join(outputDir, "test.html")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("output file not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	html := string(content)
	if len(html) == 0 {
		t.Fatal("output file is empty")
	}

	// Should contain planet name and entry titles
	if !containsString(html, "Test Planet") {
		t.Error("output does not contain planet name")
	}
	if !containsString(html, "Entry 1") {
		t.Error("output does not contain first entry")
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		haystack != needle &&
		(haystack == needle || len(haystack) > len(needle) &&
			(haystack[:len(needle)] == needle ||
				haystack[len(haystack)-len(needle):] == needle ||
				containsSubstring(haystack, needle)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
