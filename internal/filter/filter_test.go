package filter

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
)

func TestFilter_Apply_Include(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Clojure Tutorial", Content: "Learn Clojure", Date: time.Now()},
		{Title: "Python Guide", Content: "Learn Python", Date: time.Now()},
		{Title: "Clojure Tips", Content: "Advanced tips", Date: time.Now()},
	}

	filter, err := New("Clojure", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	for _, entry := range filtered {
		if entry.Title != "Clojure Tutorial" && entry.Title != "Clojure Tips" {
			t.Errorf("unexpected entry: %q", entry.Title)
		}
	}
}

func TestFilter_Apply_Exclude(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Good Post", Content: "Quality content", Date: time.Now()},
		{Title: "Spam Post", Content: "Buy now!", Date: time.Now()},
		{Title: "Another Good Post", Content: "More quality", Date: time.Now()},
	}

	filter, err := New("", "Spam")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	for _, entry := range filtered {
		if entry.Title == "Spam Post" {
			t.Errorf("spam entry not filtered out")
		}
	}
}

func TestFilter_Apply_NoFilter(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Entry 1", Content: "Content 1", Date: time.Now()},
		{Title: "Entry 2", Content: "Content 2", Date: time.Now()},
	}

	filter, err := New("", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
}

func TestFilter_Apply_BothFilters(t *testing.T) {
	entries := []cache.Entry{
		{Title: "Good Clojure Post", Content: "Learn Clojure", Date: time.Now()},
		{Title: "Bad Clojure Spam", Content: "Buy Clojure now!", Date: time.Now()},
		{Title: "Good Python Post", Content: "Learn Python", Date: time.Now()},
	}

	filter, err := New("Clojure", "Spam")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	filtered := filter.Apply(entries)

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}

	if filtered[0].Title != "Good Clojure Post" {
		t.Errorf("filtered[0].Title = %q, want %q", filtered[0].Title, "Good Clojure Post")
	}
}
