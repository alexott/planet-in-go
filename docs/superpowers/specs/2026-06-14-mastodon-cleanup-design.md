# Mastodon duplicate cleanup design

## Summary

Add a dedicated `planet cleanup-mastodon` command that finds bad Mastodon posts created during the June 1-3 incident window and removes them safely. The command will fetch statuses from the account timeline, identify only statuses posted between **2026-06-01 00:00:00** and **2026-06-03 23:59:59**, and delete only those whose linked article is older than **2026-05-01 00:00:00**. It will default to a dry run and enforce Mastodon’s deletion limit of **30 delete calls per 30 minutes** during real execution.

## Problem

Because of a bug, the project posted duplicate and stale links to Mastodon between June 1 and June 3. We need a cleanup tool that can remove those old posts without touching newer valid posts and without tripping Mastodon’s rate limits for status deletion.

## Goals

- Add a repeatable cleanup command to this repository rather than relying on an external one-off script.
- Select candidate posts from the live Mastodon account timeline.
- Delete only statuses posted from June 1 through June 3, inclusive.
- Within that time window, delete only statuses whose linked article is older than May 1.
- Never delete statuses posted on or after June 4.
- Default to dry-run mode and require an explicit flag to perform deletions.
- Respect Mastodon’s 30 deletions per 30 minutes limit.

## Non-goals

- No changes to normal feed fetching, rendering, or posting behavior.
- No automatic repair of local tracking files as part of the cleanup.
- No deletion based on a hand-maintained list of status IDs.
- No guessing when article age cannot be determined reliably.

## Proposed design

### CLI shape

Add a new top-level command:

```text
planet cleanup-mastodon [flags]
```

Expected flags:

- `-c <path>`: reuse the existing config-file flag behavior
- `-debug`: reuse existing debug logging behavior
- `-apply`: perform deletions; without this flag the command is dry-run only
- `-from <timestamp>`: optional override for the incident window start, default `2026-06-01T00:00:00`
- `-to <timestamp>`: optional override for the incident window end, default `2026-06-03T23:59:59`
- `-article-before <timestamp>`: optional override for the article-age cutoff, default `2026-05-01T00:00:00`

The default behavior should be conservative: print matches and reasons, but do not delete anything unless `-apply` is set.

### Status selection rules

The command should use the Mastodon API to fetch statuses from the authenticated account timeline and inspect them one by one.

For each status:

1. Keep it as a candidate only if the Mastodon status `created_at` is within the June 1-3 incident window.
2. Immediately skip it if the status was created on or after June 4.
3. Extract the linked article URL from the status body.
4. Determine the linked article’s publication date.
5. Delete the status only if the article date is strictly older than May 1.

This means the cleanup boundary is driven first by the **Mastodon post time**, then by the **linked article time**.

### URL extraction and article-date resolution

Each candidate status should yield exactly one article URL to evaluate.

Resolution strategy:

1. Prefer URLs present in the status content itself.
2. If the status contains multiple URLs, use the final article link rather than Mastodon-local links or tag links.
3. Normalize obvious redirect wrappers only when needed to reach the real article URL.
4. Fetch the article and determine its date from reliable metadata already used by the project where possible.

The resolver should be strict:

- if no article URL can be found, skip the status and report it;
- if the article date cannot be determined confidently, skip the status and report it;
- do not infer article age from URL shape alone.

### Rate limiting and pacing

Mastodon allows **30 delete-related calls per 30 minutes** for status deletion. The cleanup command should therefore delete sequentially and throttle itself locally.

Recommended behavior:

1. Maintain a rolling local window of delete attempts.
2. Allow up to 30 delete calls inside a 30-minute period.
3. Before issuing the 31st call in the active window, wait until the window expires.
4. If Mastodon responds with stricter rate-limit information, honor the longer wait.

This should be implemented as explicit pacing logic in the cleanup flow, not as best-effort retry guessing after the server rejects requests.

### Reporting and operator feedback

The command should print progress that makes a long-running cleanup understandable:

- statuses examined
- statuses inside the June 1-3 window
- statuses matched for deletion
- statuses skipped because article date is too new
- statuses skipped because URL or date could not be resolved
- statuses deleted
- current wait periods caused by rate limiting

In dry-run mode, the output should clearly label matches as `would delete` and include the status ID, status creation time, extracted URL, and resolved article date.

### Error handling

The command should fail explicitly on setup and API failures, following existing project patterns.

- Missing Mastodon credentials should stop the command with a clear error.
- Timeline fetch failures should stop the command with a clear error.
- Individual candidate-analysis failures should not abort the full run if they are local to a single status; they should be reported as skipped.
- Delete failures should be reported with the status ID and should not be silently retried without waiting logic.

This keeps the command safe: setup errors fail fast, while per-status ambiguity is handled conservatively by skipping.

## Components

The implementation should stay local to CLI wiring and the existing Mastodon package boundaries.

Suggested responsibilities:

- **CLI handler** in `cmd/planet/main.go` for argument parsing and command dispatch
- **timeline fetcher** in `internal/mastodon` to page through account statuses
- **status analyzer** in `internal/mastodon` to extract article URLs and apply the time filters
- **article date resolver** in `internal/mastodon` to resolve publication dates
- **deletion scheduler** in `internal/mastodon` to enforce local pacing and execute deletes
- **reporting helpers** to format dry-run and apply-mode output consistently

This keeps the cleanup logic cohesive and avoids mixing incident-specific behavior into the normal posting path.

## Testing

Add tests for:

1. command routing and flag parsing for `cleanup-mastodon`;
2. selecting only statuses created between June 1 and June 3;
3. never selecting statuses created on or after June 4;
4. deleting only when the linked article date is older than May 1;
5. dry-run mode producing `would delete` output without delete calls;
6. pacing logic that waits after 30 delete calls in a 30-minute window;
7. skipping statuses when URL extraction or article-date resolution fails.

## Implementation impact

Expected file touch points:

- `cmd/planet/main.go` for the new subcommand
- `internal/mastodon/mastodon.go` or closely related new files for cleanup support
- `internal/mastodon/*_test.go` for the new cleanup tests
- user-facing docs if the new command should be documented after implementation
