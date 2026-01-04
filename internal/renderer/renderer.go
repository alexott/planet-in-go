package renderer

import (
	"fmt"
	"html/template"
	"io"
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
	Date822      string // RFC 822 format for RSS 2.0
	DateISO      string
	ID           string
	ChannelName  string
	ChannelLink  string
	ChannelTitle string
	NewDate      bool
	NewChannel   bool

	// Additional metadata for Atom templates
	ChannelLanguage    string
	TitleLanguage      string
	ContentLanguage    string
	ChannelAuthorName  string
	ChannelAuthorEmail string
	ChannelSubtitle    string
	ChannelURL         string
	ChannelID          string
	ChannelUpdatedISO  string
	ChannelRights      string
}

// Channel represents a feed channel
type Channel struct {
	Name  string
	Link  string // HTML page URL
	Title string
	URL   string // Feed URL
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

	// Build channel list from ALL configured feeds (not just those with entries on this page)
	// This ensures the sidebar shows all subscriptions for visibility
	channelMap := make(map[string]Channel)
	for _, feed := range cfg.Feeds {
		channelMap[feed.Name] = Channel{
			Name:  feed.Name,
			Link:  "", // Will be populated from cache entries below
			Title: feed.Name,
			URL:   feed.URL,
		}
	}
	
	// Load channel links from ALL cache entries (not just filtered ones being rendered)
	// This ensures channels have proper homepage links even if no recent entries
	cacheInstance := cache.New(cfg.Planet.CacheDirectory)
	if allEntries, err := cacheInstance.LoadAll(); err == nil {
		for _, entry := range allEntries {
			if ch, exists := channelMap[entry.ChannelName]; exists && ch.Link == "" {
				ch.Link = entry.ChannelLink
				ch.Title = entry.ChannelTitle
				channelMap[entry.ChannelName] = ch
			}
		}
	}

	// Track previous entry for NewDate/NewChannel flags
	var prevDate string
	var prevChannel string

	for _, entry := range entries {
		var dateStr, date822, dateISO string

		// Only format non-zero dates — zero time means unknown/unspecified
		if !entry.Date.IsZero() {
			dateStr = entry.Date.Format(cfg.Planet.DateFormat)
			date822 = entry.Date.Format(time.RFC1123Z)
			dateISO = entry.Date.Format(time.RFC3339)
		} else {
			dateStr = ""
			date822 = ""
			dateISO = ""
		}

		// Format channel updated time
		channelUpdatedISO := ""
		if !entry.ChannelUpdated.IsZero() {
			channelUpdatedISO = entry.ChannelUpdated.Format(time.RFC3339)
		}

		item := TemplateEntry{
			Title:        entry.Title,
			Link:         entry.Link,
			Content:      template.HTML(entry.Content),
			Author:       entry.Author,
			AuthorEmail:  entry.AuthorEmail,
			Date:         dateStr,
			Date822:      date822,
			DateISO:      dateISO,
			ID:           entry.ID,
			ChannelName:  entry.ChannelName,
			ChannelLink:  entry.ChannelLink,
			ChannelTitle: entry.ChannelTitle,
			NewDate:      dateStr != prevDate,
			NewChannel:   entry.ChannelName != prevChannel,

			// Additional metadata
			ChannelLanguage:    entry.ChannelLanguage,
			TitleLanguage:      entry.TitleLanguage,
			ContentLanguage:    entry.ContentLanguage,
			ChannelAuthorName:  entry.ChannelAuthorName,
			ChannelAuthorEmail: entry.ChannelAuthorEmail,
			ChannelSubtitle:    entry.ChannelSubtitle,
			ChannelURL:         entry.ChannelURL,
			ChannelID:          entry.ChannelID,
			ChannelUpdatedISO:  channelUpdatedISO,
			ChannelRights:      entry.ChannelRights,
		}

		data.Items = append(data.Items, item)

		// Update channel info from entry if the channel already exists in our map
		// This populates Link and Title for channels that have entries
		if existingChannel, exists := channelMap[entry.ChannelName]; exists {
			existingChannel.Link = entry.ChannelLink
			existingChannel.Title = entry.ChannelTitle
			channelMap[entry.ChannelName] = existingChannel
		}

		prevDate = dateStr
		prevChannel = entry.ChannelName
	}

	// Convert channel map to slice and sort alphabetically by name
	for _, channel := range channelMap {
		data.Channels = append(data.Channels, channel)
	}
	
	// Sort channels alphabetically by name for easier browsing
	sort.Slice(data.Channels, func(i, j int) bool {
		return data.Channels[i].Name < data.Channels[j].Name
	})

	return data
}

// CopyStaticFiles copies static assets from source to output directory
// This mirrors the Python version's behavior where static files live alongside output
func (r *Renderer) CopyStaticFiles(staticSourceDir string) error {
	if staticSourceDir == "" {
		return nil // No static directory specified
	}

	// Check if source static directory exists
	if _, err := os.Stat(staticSourceDir); os.IsNotExist(err) {
		return nil // Source doesn't exist, skip silently
	}

	destStaticDir := filepath.Join(r.outputDir, "static")

	// Remove existing static directory in output to ensure clean copy
	if err := os.RemoveAll(destStaticDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing static directory: %w", err)
	}

	// Copy directory recursively
	return copyDir(staticSourceDir, destStaticDir)
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source directory: %w", err)
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	// Read source directory
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source directory: %w", err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}

	// Create destination file
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}

	return nil
}
