package renderer

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

// Renderer handles template rendering
type Renderer struct {
	outputDir string
}

// New creates a new renderer
func New(outputDir string) *Renderer {
	return &Renderer{outputDir: outputDir}
}

// TemplateData contains data passed to templates
type TemplateData struct {
	Name       string
	Link       string
	OwnerName  string
	OwnerEmail string
	Generator  string
	Date       string
	DateISO    string
	Items      []TemplateEntry
	Channels   []Channel
}

// TemplateEntry represents an entry for templates
type TemplateEntry struct {
	Title        string
	Link         string
	Content      template.HTML
	Author       string
	AuthorEmail  string
	Date         string
	DateISO      string
	ID           string
	ChannelName  string
	ChannelLink  string
	ChannelTitle string
	NewDate      bool
	NewChannel   bool
}

// Channel represents a feed channel
type Channel struct {
	Name  string
	Link  string
	Title string
}

// Render renders a template with entries
func (r *Renderer) Render(templatePath string, entries []cache.Entry, cfg *config.Config) error {
	// Ensure output directory exists
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Sort entries by date (newest first)
	sorted := sortByDate(entries)

	// Apply pagination
	paginated := paginate(sorted, cfg.Planet.ItemsPerPage, cfg.Planet.DaysPerPage)

	// Prepare template data
	data := r.prepareTemplateData(paginated, cfg)

	// Parse template
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	// Determine output filename (remove .tmpl extension)
	outputName := filepath.Base(templatePath)
	if ext := filepath.Ext(outputName); ext == ".tmpl" {
		outputName = outputName[:len(outputName)-len(ext)]
	}
	outputPath := filepath.Join(r.outputDir, outputName)

	// Create output file
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	// Execute template
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	return nil
}

// sortByDate sorts entries by date (newest first)
func sortByDate(entries []cache.Entry) []cache.Entry {
	sorted := make([]cache.Entry, len(entries))
	copy(sorted, entries)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.After(sorted[j].Date)
	})

	return sorted
}

// paginate limits entries by count and days
func paginate(entries []cache.Entry, itemsPerPage, daysPerPage int) []cache.Entry {
	if len(entries) == 0 {
		return entries
	}

	// Apply items per page limit
	if itemsPerPage > 0 && len(entries) > itemsPerPage {
		entries = entries[:itemsPerPage]
	}

	// Apply days per page limit
	if daysPerPage > 0 {
		cutoff := time.Now().AddDate(0, 0, -daysPerPage)
		filtered := make([]cache.Entry, 0, len(entries))
		for _, entry := range entries {
			if entry.Date.After(cutoff) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}

	return entries
}

// prepareTemplateData converts entries to template data
func (r *Renderer) prepareTemplateData(entries []cache.Entry, cfg *config.Config) TemplateData {
	data := TemplateData{
		Name:       cfg.Planet.Name,
		Link:       cfg.Planet.Link,
		OwnerName:  cfg.Planet.OwnerName,
		OwnerEmail: cfg.Planet.OwnerEmail,
		Generator:  "Planet Go",
		Date:       time.Now().Format(cfg.Planet.DateFormat),
		DateISO:    time.Now().Format(time.RFC3339),
		Items:      make([]TemplateEntry, 0, len(entries)),
		Channels:   make([]Channel, 0),
	}

	// Track unique channels
	channelMap := make(map[string]Channel)

	// Track previous entry for NewDate/NewChannel flags
	var prevDate string
	var prevChannel string

	for _, entry := range entries {
		dateStr := entry.Date.Format(cfg.Planet.DateFormat)

		item := TemplateEntry{
			Title:        entry.Title,
			Link:         entry.Link,
			Content:      template.HTML(entry.Content),
			Author:       entry.Author,
			AuthorEmail:  entry.AuthorEmail,
			Date:         dateStr,
			DateISO:      entry.Date.Format(time.RFC3339),
			ID:           entry.ID,
			ChannelName:  entry.ChannelName,
			ChannelLink:  entry.ChannelLink,
			ChannelTitle: entry.ChannelTitle,
			NewDate:      dateStr != prevDate,
			NewChannel:   entry.ChannelName != prevChannel,
		}

		data.Items = append(data.Items, item)

		// Track channel
		if _, exists := channelMap[entry.ChannelName]; !exists {
			channelMap[entry.ChannelName] = Channel{
				Name:  entry.ChannelName,
				Link:  entry.ChannelLink,
				Title: entry.ChannelTitle,
			}
		}

		prevDate = dateStr
		prevChannel = entry.ChannelName
	}

	// Convert channel map to slice
	for _, channel := range channelMap {
		data.Channels = append(data.Channels, channel)
	}

	return data
}
