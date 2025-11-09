package filter

import (
	"fmt"
	"log/slog"
	"regexp"

	"github.com/alexey-ott/planet-go/internal/cache"
	"github.com/alexey-ott/planet-go/internal/config"
)

// Filter applies regex-based filtering to entries
type Filter struct {
	include *regexp.Regexp
	exclude *regexp.Regexp
}

// New creates a new filter with include and exclude patterns
func New(includePattern, excludePattern string) (*Filter, error) {
	f := &Filter{}

	if includePattern != "" {
		re, err := regexp.Compile(includePattern)
		if err != nil {
			return nil, fmt.Errorf("compile include pattern: %w", err)
		}
		f.include = re
	}

	if excludePattern != "" {
		re, err := regexp.Compile(excludePattern)
		if err != nil {
			return nil, fmt.Errorf("compile exclude pattern: %w", err)
		}
		f.exclude = re
	}

	return f, nil
}

// Apply filters entries based on include/exclude patterns
func (f *Filter) Apply(entries []cache.Entry) []cache.Entry {
	if f.include == nil && f.exclude == nil {
		return entries
	}

	filtered := make([]cache.Entry, 0, len(entries))

	for _, entry := range entries {
		// Combine title and content for searching
		text := entry.Title + " " + entry.Content

		// Check include pattern
		if f.include != nil && !f.include.MatchString(text) {
			continue
		}

		// Check exclude pattern
		if f.exclude != nil && f.exclude.MatchString(text) {
			continue
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// ApplyPerFeed filters entries using per-feed filters combined with global filters
// Each entry is filtered using its feed's specific filter (if any) plus the global filter
func ApplyPerFeed(entries []cache.Entry, feedConfigs []config.FeedConfig, globalInclude, globalExclude string) ([]cache.Entry, error) {
	// Build a map of feed URL -> filter
	feedFilters := make(map[string]*Filter)

	// Create filters for each feed
	for _, feedConfig := range feedConfigs {
		// Combine global and feed-level patterns
		includePattern := globalInclude
		excludePattern := globalExclude

		feedFilter := feedConfig.Filter()
		feedExclude := feedConfig.Exclude()

		// If feed has its own filter, use it (feed-level overrides global)
		if feedFilter != "" {
			includePattern = feedFilter
		}

		// If feed has its own exclude, use it (feed-level overrides global)
		if feedExclude != "" {
			excludePattern = feedExclude
		}

		// Only create a filter if there's something to filter
		if includePattern != "" || excludePattern != "" {
			filter, err := New(includePattern, excludePattern)
			if err != nil {
				return nil, fmt.Errorf("create filter for feed %s: %w", feedConfig.URL, err)
			}
			feedFilters[feedConfig.URL] = filter

			slog.Debug("created per-feed filter",
				"feed", feedConfig.Name,
				"url", feedConfig.URL,
				"include", includePattern,
				"exclude", excludePattern)
		}
	}

	// Apply filters per feed
	filtered := make([]cache.Entry, 0, len(entries))
	filteredCount := 0

	for _, entry := range entries {
		// Find the filter for this entry's feed
		filter, hasFilter := feedFilters[entry.ChannelURL]

		if !hasFilter {
			// No filter for this feed, keep the entry
			filtered = append(filtered, entry)
			continue
		}

		// Apply the filter
		result := filter.Apply([]cache.Entry{entry})
		if len(result) > 0 {
			filtered = append(filtered, entry)
		} else {
			filteredCount++
			slog.Debug("entry filtered out",
				"feed", entry.ChannelName,
				"title", entry.Title)
		}
	}

	if filteredCount > 0 {
		slog.Info("per-feed filtering complete",
			"total_entries", len(entries),
			"filtered_out", filteredCount,
			"remaining", len(filtered))
	}

	return filtered, nil
}
