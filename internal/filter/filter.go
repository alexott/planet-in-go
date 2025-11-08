package filter

import (
	"fmt"
	"regexp"

	"github.com/alexey-ott/planet-go/internal/cache"
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
