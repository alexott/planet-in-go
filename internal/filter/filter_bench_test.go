package filter

import (
	"testing"
	"time"

	"github.com/alexey-ott/planet-go/internal/cache"
)

func BenchmarkFilter_Apply_Include(b *testing.B) {
	// Create test entries
	entries := make([]cache.Entry, 100)
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			entries[i] = cache.Entry{
				Title:   "Clojure Tutorial " + string(rune(i)),
				Content: "Learn Clojure programming",
				Date:    time.Now(),
			}
		} else {
			entries[i] = cache.Entry{
				Title:   "Python Guide " + string(rune(i)),
				Content: "Learn Python programming",
				Date:    time.Now(),
			}
		}
	}

	filter, _ := New("Clojure", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Apply(entries)
	}
}

func BenchmarkFilter_Apply_Exclude(b *testing.B) {
	// Create test entries
	entries := make([]cache.Entry, 100)
	for i := 0; i < 100; i++ {
		if i%10 == 0 {
			entries[i] = cache.Entry{
				Title:   "Spam Post " + string(rune(i)),
				Content: "Buy now! Special offer!",
				Date:    time.Now(),
			}
		} else {
			entries[i] = cache.Entry{
				Title:   "Good Post " + string(rune(i)),
				Content: "Quality content about programming",
				Date:    time.Now(),
			}
		}
	}

	filter, _ := New("", "Spam|Buy now")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Apply(entries)
	}
}

func BenchmarkFilter_Apply_BothFilters(b *testing.B) {
	// Create test entries
	entries := make([]cache.Entry, 100)
	for i := 0; i < 100; i++ {
		var title, content string
		switch i % 3 {
		case 0:
			title = "Clojure Tutorial"
			content = "Learn Clojure programming"
		case 1:
			title = "Clojure Spam"
			content = "Buy Clojure books now!"
		case 2:
			title = "Python Guide"
			content = "Learn Python programming"
		}
		entries[i] = cache.Entry{
			Title:   title,
			Content: content,
			Date:    time.Now(),
		}
	}

	filter, _ := New("Clojure", "Spam|Buy")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Apply(entries)
	}
}

func BenchmarkFilter_Apply_NoFilter(b *testing.B) {
	// Create test entries
	entries := make([]cache.Entry, 100)
	for i := 0; i < 100; i++ {
		entries[i] = cache.Entry{
			Title:   "Entry " + string(rune(i)),
			Content: "Test content",
			Date:    time.Now(),
		}
	}

	filter, _ := New("", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Apply(entries)
	}
}

func BenchmarkFilter_Apply_LargeDataset(b *testing.B) {
	// Create large dataset
	entries := make([]cache.Entry, 10000)
	for i := 0; i < 10000; i++ {
		entries[i] = cache.Entry{
			Title:   "Entry about programming and software development",
			Content: "This is a longer content with multiple paragraphs discussing various topics in software development, including design patterns, best practices, and code quality.",
			Date:    time.Now(),
		}
	}

	filter, _ := New("programming|development", "spam|advertisement")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.Apply(entries)
	}
}

