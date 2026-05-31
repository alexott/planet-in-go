# Mastodon posting support design

## Summary

Add Mastodon posting alongside the existing Twitter support, using `github.com/mattn/go-mastodon` for API access. The Mastodon integration should be independently configurable, keep its own tracking state, use Mastodon-specific text budgeting rules, and support per-feed author attribution from a `mastodon` feed parameter with fallback to the feed `name`.

## Goals

- Add a global boolean flag to enable or disable Mastodon posting.
- Add a separate Mastodon tracking file so Mastodon dedup state is independent from Twitter.
- Support a per-feed `mastodon` parameter for attribution.
- Fall back to `(by <name>)` when a feed does not define `mastodon`.
- Respect Mastodon text constraints: 500 character status limit and URLs counted as 23 characters.
- Keep the change aligned with the current Twitter-oriented structure instead of introducing a broader posting abstraction first.

## Non-goals

- No refactor into a generic multi-network provider abstraction in this change.
- No changes to Twitter behavior beyond the minimum command wiring needed for Mastodon coexistence.
- No backdating of Mastodon posts to the original blog post timestamp; the Mastodon API does not support posting with an arbitrary past `created_at`.

## Current codebase shape

The existing social posting flow is Twitter-specific:

- `internal/config/config.go` exposes `post_to_twitter` and `twitter_tracking_file`.
- Feed-level social metadata is stored in `FeedConfig.Extra`, with a `TwitterHandle()` helper reading the `twitter` key.
- `cmd/planet/main.go` posts to Twitter from both the default `run` flow and the explicit `post` command.
- `internal/twitter/twitter.go` owns text formatting, tracking-file persistence, deduplication by `entry.ID`, chronological posting, and first-run limiting.

This shape supports adding Mastodon as a sibling package with matching boundaries and low risk.

## Proposed design

### Configuration

Extend `PlanetConfig` with:

- `PostToMastodon bool`
- `MastodonTrackingFile string`

Parse these from the `[Planet]` section:

- `post_to_mastodon = true|false` (default `false`)
- `mastodon_tracking_file = mastodon_posted.json` (default relative path, resolved at runtime against the cache directory the same way Twitter is handled now)

Extend `FeedConfig` with a helper that reads the feed-level `mastodon` value from `Extra`, parallel to `TwitterHandle()`.

Per-feed attribution behavior:

- If `mastodon` is present and non-empty, use it exactly as configured.
- Otherwise, if `name` is present and non-empty, use `(by <name>)`.
- If neither yields a value, post without attribution.

### Credentials and client setup

Use environment variables, parallel to the existing Twitter setup:

- `MASTODON_SERVER`
- `MASTODON_CLIENT_ID`
- `MASTODON_CLIENT_SECRET`
- `MASTODON_ACCESS_TOKEN`

`internal/mastodon.NewPoster(trackingFile string)` should:

1. Read and validate those environment variables.
2. Construct a `mastodon.Client` using `mastodon.NewClient(&mastodon.Config{...})`.
3. Return a clear error if any required value is missing.

This keeps secrets out of the INI file and matches the project’s current pattern for Twitter credentials.

### Package structure

Add a new package:

- `internal/mastodon`

Responsibilities:

- format Mastodon statuses
- load and save Mastodon tracking data
- deduplicate entries by `entry.ID`
- sort and select which entries to post
- post statuses via `go-mastodon`

Initial structure should closely mirror `internal/twitter`:

- `Poster`
- `PostedArticle`
- `TrackingData`
- formatter helper(s)
- package tests

This is intentionally duplicated structure rather than a shared abstraction. If the implementation reveals awkward duplication, small shared helpers can be extracted later without changing the public behavior.

### Posting flow integration

Update `cmd/planet/main.go` so Mastodon can coexist with Twitter in both command paths.

#### `run` flow

After fetch + render:

- keep the current Twitter behavior behind `cfg.Planet.PostToTwitter`
- add Mastodon posting behind `cfg.Planet.PostToMastodon`
- if both are enabled, attempt both independently
- an error in one network should be logged without corrupting or reusing the other network’s tracking state

#### `post` command

The command currently describes itself as Twitter-only and applies a Twitter-specific “latest 10 entries” pre-limit before posting. For Mastodon support:

- update user-facing messaging so `post` is no longer described as Twitter-only
- keep Twitter’s existing pre-limit behavior as-is
- add a separate Mastodon path that does **not** inherit the Twitter “latest 10 entries” cap
- allow the command to run Twitter, Mastodon, or both based on config flags

This avoids accidentally throttling Mastodon with Twitter-specific command behavior.

### Tracking data

Use a separate JSON tracking file for Mastodon, parallel to Twitter.

Suggested tracked fields:

- `id` (`entry.ID`)
- `link`
- `title`
- `posted_at`
- `mastodon_status_id`
- `mastodon_status_text`
- optional `article_date` for local bookkeeping/debugging

Deduplication must be based on `entry.ID`, matching Twitter.

Tracking-file behavior:

- if the file does not exist, treat it as an empty first run
- create parent directories as needed
- write explicit errors on read, parse, marshal, or write failures

### Status formatting

Format Mastodon posts as:

```text
<title><attribution>

<link>
```

Where:

- `attribution` is either empty, or ` (by <mastodon>)`, or ` (by <name>)`
- the feed `mastodon` value is used exactly as written; no normalization is applied

Length budgeting rules:

- Mastodon maximum status length: **500**
- URLs count as **23** characters
- separator between text and link: **2** characters (`\n\n`)

Budget formula:

```text
len(title) + len(attribution) + 2 + 23 <= 500
```

If the status is too long:

1. compute the title space remaining after attribution, separator, and URL reserve
2. truncate the title to fit
3. append `...` when truncation occurs
4. if there is too little room to keep attribution and a minimally useful title, drop attribution before truncating the title

This mirrors the Twitter formatter’s style while using Mastodon’s limits.

### Selection and ordering

Selection should mirror Twitter unless Mastodon-specific requirements override it:

- sort entries oldest to newest before posting
- post only entries not found in Mastodon tracking

First-run behavior differs from Twitter:

- Mastodon should initialize from the **latest 50 posts**
- this is an independent Mastodon-specific first-run cap
- after the first run, post all newly discovered entries with no additional Mastodon cap

This preserves the current Twitter behavior while meeting the new Mastodon requirement.

### Time semantics

The implementation should **not** attempt to backdate Mastodon posts to the original article publish time.

Reason:

- Mastodon’s status API accepts `scheduled_at`, but only for datetimes at least five minutes in the future.
- `go-mastodon` exposes this through `mastodon.Toot.ScheduledAt`.
- Neither the API nor the library supports supplying an arbitrary past `created_at`.

Therefore:

- Mastodon `created_at` will be the actual posting time.
- If desired for diagnostics, the original article date may be stored in tracking metadata, but it will not affect the remote post timestamp.

### Error handling

- Missing Mastodon credentials should fail poster initialization with a clear, explicit error.
- Tracking-file failures should surface as errors.
- Posting failures for Mastodon should not write successful tracking entries for failed posts.
- When both Twitter and Mastodon are enabled, each network should operate independently with its own logging and tracking state.

### Testing

Add or update tests in these areas:

1. **Config parsing**
   - `post_to_mastodon`
   - `mastodon_tracking_file`
   - feed-level `mastodon` helper
   - default tracking-file path behavior
2. **Formatter behavior**
   - simple status without attribution
   - status with `mastodon` attribution
   - fallback to feed `name`
   - 500-character budgeting
   - 23-character URL accounting
   - truncation with and without attribution
3. **Tracking behavior**
   - first run with missing file
   - deduplication by `entry.ID`
   - first-run selection of latest 50 entries
   - chronological posting order
4. **Command integration**
   - Mastodon gated by its boolean flag
   - `post` command messaging updated for Mastodon coexistence
   - Mastodon path does not inherit Twitter’s latest-10 pre-limit
   - both networks can be enabled together

## Implementation notes

- Preferred dependency: `github.com/mattn/go-mastodon`
- Use `mastodon.NewClient` with environment-sourced credentials.
- Use `Client.PostStatus(ctx, &mastodon.Toot{...})` for posting.
- Keep the initial implementation focused and local to config, command wiring, and the new `internal/mastodon` package.

## Open decisions resolved during brainstorming

- Credentials live in environment variables, not the INI file.
- The feed-level `mastodon` value is used exactly as written.
- Deduplication mirrors Twitter by `entry.ID`.
- Initial Mastodon backfill is the latest 50 posts.
- Backdating Mastodon posts to article publish time is not possible with the upstream API.
