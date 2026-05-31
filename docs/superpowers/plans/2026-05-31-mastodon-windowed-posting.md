# Mastodon Latest-50 Window Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change Mastodon post selection so only the current latest 50 cached entries are ever eligible for posting, with tracking used only to dedupe within that window.

**Architecture:** Keep the change local to `internal/mastodon`. Update the selection tests first to describe the new latest-50 window semantics, then change `selectEntriesToPost` so it limits the sorted entry list before filtering against `TrackingData`. Leave formatting, CLI wiring, and config handling untouched.

**Tech Stack:** Go, `testing`, existing `internal/mastodon` package

---

## File map

- Modify: `internal/mastodon/mastodon_test.go` — replace the old “after first run post all unseen entries” assumption with tests for the strict latest-50 window.
- Modify: `internal/mastodon/mastodon.go` — change `selectEntriesToPost` to limit to the latest 50 entries before deduplication.

### Task 1: Update Mastodon selection behavior

**Files:**
- Modify: `internal/mastodon/mastodon_test.go`
- Modify: `internal/mastodon/mastodon.go`
- Test: `internal/mastodon/mastodon_test.go`

- [ ] **Step 1: Write the failing tests**

Add these two tests to `internal/mastodon/mastodon_test.go` below `TestSelectEntriesForFirstRunKeepsLatest50`:

```go
func TestSelectEntriesToPostFiltersOnlyWithinLatest50Window(t *testing.T) {
	entries := make([]cache.Entry, 60)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Title: "Post",
			Link:  "https://example.com/post",
			Date:  time.Unix(int64(i), 0),
		}
	}

	tracking := &TrackingData{
		Articles: []PostedArticle{
			{ID: "entry-55"},
			{ID: "entry-57"},
		},
	}

	selected := selectEntriesToPost(entries, tracking, 50)

	wantIDs := []string{
		"entry-10", "entry-11", "entry-12", "entry-13", "entry-14",
		"entry-15", "entry-16", "entry-17", "entry-18", "entry-19",
		"entry-20", "entry-21", "entry-22", "entry-23", "entry-24",
		"entry-25", "entry-26", "entry-27", "entry-28", "entry-29",
		"entry-30", "entry-31", "entry-32", "entry-33", "entry-34",
		"entry-35", "entry-36", "entry-37", "entry-38", "entry-39",
		"entry-40", "entry-41", "entry-42", "entry-43", "entry-44",
		"entry-45", "entry-46", "entry-47", "entry-48", "entry-49",
		"entry-50", "entry-51", "entry-52", "entry-53", "entry-54",
		"entry-56", "entry-58", "entry-59",
	}

	if len(selected) != len(wantIDs) {
		t.Fatalf("len(selected) = %d, want %d", len(selected), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if selected[i].ID != wantID {
			t.Fatalf("selected[%d].ID = %q, want %q", i, selected[i].ID, wantID)
		}
	}
}

func TestSelectEntriesToPostIgnoresOlderUntrackedEntriesOutsideLatest50Window(t *testing.T) {
	entries := make([]cache.Entry, 60)
	for i := range entries {
		entries[i] = cache.Entry{
			ID:    fmt.Sprintf("entry-%02d", i),
			Title: "Post",
			Link:  "https://example.com/post",
			Date:  time.Unix(int64(i), 0),
		}
	}

	tracking := &TrackingData{
		Articles: []PostedArticle{
			{ID: "entry-10"}, {ID: "entry-11"}, {ID: "entry-12"}, {ID: "entry-13"}, {ID: "entry-14"},
			{ID: "entry-15"}, {ID: "entry-16"}, {ID: "entry-17"}, {ID: "entry-18"}, {ID: "entry-19"},
			{ID: "entry-20"}, {ID: "entry-21"}, {ID: "entry-22"}, {ID: "entry-23"}, {ID: "entry-24"},
			{ID: "entry-25"}, {ID: "entry-26"}, {ID: "entry-27"}, {ID: "entry-28"}, {ID: "entry-29"},
			{ID: "entry-30"}, {ID: "entry-31"}, {ID: "entry-32"}, {ID: "entry-33"}, {ID: "entry-34"},
			{ID: "entry-35"}, {ID: "entry-36"}, {ID: "entry-37"}, {ID: "entry-38"}, {ID: "entry-39"},
			{ID: "entry-40"}, {ID: "entry-41"}, {ID: "entry-42"}, {ID: "entry-43"}, {ID: "entry-44"},
			{ID: "entry-45"}, {ID: "entry-46"}, {ID: "entry-47"}, {ID: "entry-48"}, {ID: "entry-49"},
			{ID: "entry-50"}, {ID: "entry-51"}, {ID: "entry-52"}, {ID: "entry-53"}, {ID: "entry-54"},
			{ID: "entry-55"}, {ID: "entry-56"}, {ID: "entry-57"}, {ID: "entry-58"}, {ID: "entry-59"},
		},
	}

	selected := selectEntriesToPost(entries, tracking, 50)
	if len(selected) != 0 {
		t.Fatalf("len(selected) = %d, want 0", len(selected))
	}
}
```

- [ ] **Step 2: Run the Mastodon tests to verify the new test fails**

Run:

```bash
go test ./internal/mastodon -run 'TestSelectEntriesToPost|TestSelectEntriesForFirstRunKeepsLatest50' -count=1
```

Expected: FAIL because `selectEntriesToPost` still filters all unseen entries first and only applies `maxInitial` when `len(tracking.Articles) == 0`.

- [ ] **Step 3: Write the minimal implementation**

Replace `selectEntriesToPost` in `internal/mastodon/mastodon.go` with:

```go
func selectEntriesToPost(entries []cache.Entry, tracking *TrackingData, maxInitial int) []cache.Entry {
	sorted := make([]cache.Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	window := sorted
	if len(window) > maxInitial {
		window = window[len(window)-maxInitial:]
	}

	newEntries := make([]cache.Entry, 0, len(window))
	for _, entry := range window {
		if !(&Poster{}).isPosted(entry.ID, tracking) {
			newEntries = append(newEntries, entry)
		}
	}

	return newEntries
}
```

This keeps the ordering chronological while making the latest-50 window the eligibility boundary for both first and later runs.

- [ ] **Step 4: Run the targeted Mastodon tests to verify they pass**

Run:

```bash
go test ./internal/mastodon -run 'TestSelectEntriesToPost|TestSelectEntriesForFirstRunKeepsLatest50|TestFormatStatus|TestNewPosterRequiresEnvironment' -count=1
```

Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run:

```bash
go test ./...
```

Expected: PASS

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/mastodon/mastodon.go internal/mastodon/mastodon_test.go
git commit -m "fix: limit Mastodon posting to latest 50" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
