# Mastodon latest-50 window posting design

## Summary

Adjust Mastodon posting so eligibility is always computed from the current latest 50 cached entries. On a first run without `mastodon_posted.json`, post those 50 entries. On later runs, recompute the latest-50 window and post only entries in that window whose IDs are not already tracked. Older entries outside the window are never eligible for Mastodon posting.

## Problem

The current Mastodon logic only applies the 50-entry limit on an empty tracking file. After that first run, all remaining unseen historical cache entries stay eligible forever. In practice, this allowed an older backlog to be posted later even though the intended behavior was to seed Mastodon from a limited recent window.

## Goals

- Make Mastodon eligibility a strict sliding window over the latest 50 cached entries.
- Preserve the existing tracking file format and Mastodon config surface.
- Prevent unseen historical entries outside the latest 50 from being posted later.
- Keep the change local to Mastodon selection logic and tests.

## Non-goals

- No changes to Twitter behavior.
- No changes to Mastodon formatting, attribution, credentials, or CLI wiring.
- No migration or cleanup of existing tracking files beyond continuing to read them as-is.

## Proposed design

### Selection rules

1. Sort cached entries by article date from oldest to newest.
2. Take only the latest 50 entries from that sorted list.
3. If `mastodon_posted.json` does not exist, post all 50 entries in that window.
4. If the tracking file exists, post only entries in that same 50-entry window whose `entry.ID` is not already tracked.
5. Ignore all entries older than the current latest-50 window, even if they have never been posted.

This makes the window itself the eligibility boundary and uses the tracking file only for deduplication inside that boundary.

### Tracking semantics

The tracking file remains an append-only record of what was actually posted. It does not need to record every entry that was considered ineligible because it fell outside the latest-50 window. Existing tracking files remain valid because IDs already recorded there will continue to suppress reposting when those entries still fall inside the current window.

### Expected behavior

- **No tracking file:** the poster publishes the current latest 50 cached entries.
- **Tracking file present:** the poster recomputes the current latest 50 cached entries and publishes only the IDs in that window that are not already tracked.
- **Older unseen entries:** never become eligible later, even if they remain in the cache.
- **Deleted Mastodon posts:** are not reposted automatically if their IDs are still present in the tracking file.

## Error handling

This change does not add new error cases. Existing behavior stays in place for tracking-file read/write errors and Mastodon API failures. If a post fails, only successfully posted entries should be written to tracking, as today.

## Testing

Update Mastodon selection tests to cover:

1. no tracking file selects exactly the latest 50 entries;
2. a tracking file causes posting to filter only within the latest-50 window;
3. older untracked entries outside the latest-50 window are not selected;
4. existing formatting and posting behavior remain unchanged.

## Implementation impact

Only the Mastodon selection path should need code changes, primarily in `internal/mastodon/mastodon.go` and its tests. CLI behavior and config parsing should remain unchanged.
