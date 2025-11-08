# Planet Clojure Go Reimplementation - Design Document

**Date:** 2025-01-08
**Author:** Design session with Alexey Ott
**Status:** Validated

## Executive Summary

This document describes the design for reimplementing Planet Clojure feed aggregator in Go. The current implementation uses Python 2.x (Venus/Planet), which is unmaintained and lacks modern features. The Go reimplementation aims to provide better performance, maintainability, and ease of deployment while maintaining feature compatibility.

**Approach:** Start with MVP (Approach A) - monolithic CLI tool with sequential processing, designed to easily upgrade to concurrent processing (Approach B) later.

## Goals

- **Performance:** Faster feed processing, lower resource usage
- **Maintainability:** Modern codebase, easy to update and debug
- **Simplicity:** Single binary deployment, minimal dependencies
- **Compatibility:** Support existing config.ini format and core features

## Non-Goals (for MVP)

- Multiple template engines (Django, XSLT, Genshi) - Go templates only
- Complex filter/plugin system - Basic regex filtering only
- Twitter integration
- PubSubHubbub support
- Admin web interface
- Multi-threaded spidering (comes in phase 2)

## Architecture

### High-Level Design

**Project Structure:**
```
planet-go/
├── cmd/
│   └── planet/
│       └── main.go              # CLI entry point
├── internal/
│   ├── config/
│   │   └── config.go            # INI parsing using go-ini
│   ├── fetcher/
│   │   └── fetcher.go           # Feed fetching (with interface for future concurrency)
│   ├── parser/
│   │   └── parser.go            # Feed parsing using gofeed
│   ├── cache/
│   │   └── cache.go             # File-based caching
│   ├── filter/
│   │   └── filter.go            # Regex filtering
│   └── renderer/
│       └── renderer.go          # Template rendering
├── go.mod
├── go.sum
└── README.md
```

### Core Workflow

```
1. Read config.ini → Parse with go-ini/ini
2. For each feed URL (sequential):
   a. Check cache freshness (HTTP conditional GET)
   b. Fetch if stale/missing (with timeout)
   c. Parse with gofeed
   d. Apply filters (regex include/exclude)
   e. Save to cache (JSON)
3. Load all cached entries
4. Sort by date (newest first)
5. Apply pagination (items_per_page, days_per_page)
6. Render each template with Go html/template
7. Write output files
```

### Key Design Decisions

1. **Interface-based fetcher:** Define `Fetcher` interface to enable future concurrent implementation without changing downstream code
2. **Separation of concerns:** Each package has single responsibility
3. **Standard library first:** Minimize external dependencies
4. **Graceful degradation:** Continue processing if individual feeds fail

## Dependencies

**External libraries:**
- [go-ini/ini](https://github.com/go-ini/ini) - Config file parsing
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed) - RSS/Atom feed parsing
- (Optional) [spf13/cobra](https://github.com/spf13/cobra) - CLI framework

**Standard library:**
- `net/http` - HTTP client
- `html/template` - Template rendering
- `log/slog` - Structured logging
- `encoding/json` - Cache serialization
- `regexp` - Content filtering

## Configuration

### Config Structure

```go
type Config struct {
    Planet    PlanetConfig
    Feeds     []FeedConfig
    Templates map[string]TemplateConfig
}

type PlanetConfig struct {
    Name           string
    Link           string
    OwnerName      string
    OwnerEmail     string
    CacheDirectory string
    OutputDir      string
    LogLevel       string
    FeedTimeout    int    // seconds
    NewFeedItems   int
    ItemsPerPage   int
    DaysPerPage    int
    DateFormat     string
    NewDateFormat  string
    Encoding       string
    TemplateFiles  []string
    Filter         string // regex include pattern
    Exclude        string // regex exclude pattern
}

type FeedConfig struct {
    URL   string
    Name  string
    Extra map[string]string // Custom fields from config
}
```

### Configuration Loading

- Use `go-ini/ini` to parse Planet's INI format
- Map `[Planet]` section to `PlanetConfig`
- Map each feed section (URLs) to `FeedConfig`
- Store custom per-feed parameters in `Extra` map
- Validate required fields and paths on load

## Data Models

### Entry Model

```go
type Entry struct {
    // Core fields
    Title       string
    Link        string
    Content     string
    Author      string
    AuthorEmail string
    Date        time.Time
    ID          string

    // Channel/Feed info
    ChannelName  string
    ChannelLink  string
    ChannelTitle string

    // For template rendering
    DateFormatted string
    DateISO       string
    NewDate       bool // Different date than previous entry
    NewChannel    bool // Different channel than previous entry
}
```

### Cache Structure

**File layout:**
```
cache_directory/
├── <hash-of-feed-url>.json    # Parsed entries
└── <hash-of-feed-url>.meta    # HTTP headers (ETag, Last-Modified)
```

**Cache file contents:**
- Feed metadata (last fetched, ETag, Last-Modified headers)
- Array of parsed Entry objects
- JSON format for easy debugging and portability

## Feed Fetching

### HTTP Client

```go
type Fetcher struct {
    client  *http.Client
    timeout time.Duration
    cache   *cache.Cache
}
```

**Configuration:**
- Timeout from `feed_timeout` config
- Keep-alive connections (reuse connections across feeds)
- Standard User-Agent header

### Fetch Process

For each feed:

1. **Check cache:** Load cached metadata (ETag, Last-Modified)
2. **Conditional GET:** Set `If-None-Match` and `If-Modified-Since` headers
3. **Handle responses:**
   - `304 Not Modified` → Use cached entries (fast path)
   - `200 OK` → Parse with gofeed, update cache
   - `4xx/5xx` → Log error, use cached entries if available
4. **Respect timeout:** Use context with deadline from config
5. **Parse:** gofeed handles both RSS and Atom automatically

### Error Handling

**Philosophy: Partial success is acceptable**

- If one feed fails, continue with others
- Log all errors but don't abort entire run
- Use cached data when fetch fails (graceful degradation)
- Only fail hard if config is invalid or output directory doesn't exist

```go
type FetchResult struct {
    URL     string
    Entries []Entry
    Error   error
}
```

### Feed Normalization

- gofeed returns normalized `Feed` structure
- Map to our `Entry` model consistently
- Prefer `Published` over `Updated` for entry date
- Handle missing fields gracefully (empty strings, not crashes)
- Basic HTML sanitization (gofeed provides this)

## Filtering

### Filter Implementation

For MVP, support two basic regex filters from Planet config:

```go
type Filter struct {
    Include *regexp.Regexp // Filter field - must match
    Exclude *regexp.Regexp // Exclude field - must not match
}

func (f *Filter) Apply(entries []Entry) []Entry {
    // Search in title + content
    // Include: keep only entries matching pattern
    // Exclude: remove entries matching pattern
}
```

**Filter application:**
- Compile regexes once at startup
- Apply to concatenated title + content
- Case-sensitive matching (can make configurable later)

## Template Rendering

### Template Data Structure

```go
type TemplateData struct {
    // Planet-level variables
    Name       string
    Link       string
    OwnerName  string
    OwnerEmail string
    Generator  string
    Date       string
    DateISO    string

    // Items for iteration
    Items      []Entry

    // Channels (unique feeds)
    Channels   []Channel
}
```

### Rendering Process

1. **Load templates:** Parse all `.tmpl` files with `html/template`
2. **Prepare data:**
   - Collect all entries from cache
   - Apply filters (include/exclude)
   - Sort by date descending
   - Apply pagination (items_per_page, days_per_page)
   - Mark `NewDate` and `NewChannel` flags for template logic
3. **Execute each template:** Render to output_dir
4. **Error handling:** If template fails, log and continue with next

### Template Migration

Users need to convert htmltmpl syntax to Go templates:

| htmltmpl | Go template |
|----------|-------------|
| `<TMPL_VAR name>` | `{{.Name}}` |
| `<TMPL_LOOP Items>...</TMPL_LOOP>` | `{{range .Items}}...{{end}}` |
| `<TMPL_IF foo>...</TMPL_IF>` | `{{if .Foo}}...{{end}}` |
| `ESCAPE="HTML"` | Automatic in `html/template` |

**Benefit:** Go's `html/template` automatically escapes HTML, providing better security.

## CLI Interface

### Commands

```bash
# Main command - fetch and render
planet -c config.ini

# Or explicit subcommands:
planet fetch -c config.ini   # Just fetch and cache
planet render -c config.ini  # Just render from cache
planet run -c config.ini     # Fetch + render (default)
planet version               # Show version info
```

### Logging

Use `log/slog` (Go standard library, 1.21+):

**Log levels:**
- Map Planet's DEBUG, INFO, WARNING, ERROR, CRITICAL
- To slog's Debug, Info, Warn, Error, Error

**Log format:**
```
2025-01-08 10:23:45 INFO  Fetching 45 feeds
2025-01-08 10:23:46 DEBUG Feed https://example.com/feed cached (304)
2025-01-08 10:23:47 ERROR Feed https://broken.com/feed timeout after 20s
2025-01-08 10:23:50 INFO  Rendering 2 templates
2025-01-08 10:23:51 INFO  Done. 123 entries processed.
```

## Operation Flow

### Startup

1. Parse command-line flags
2. Load and validate config.ini
3. Ensure cache_directory and output_dir exist (create if missing)
4. Initialize logger with configured level

### Fetch Phase

1. Log feed count
2. Iterate feeds sequentially (MVP)
3. For each feed:
   - Attempt fetch with timeout
   - Log result (cached/updated/error)
   - Update cache on success
4. Accumulate statistics

### Render Phase

1. Load all cached entries
2. Apply filters (include/exclude patterns)
3. Sort by date descending
4. Apply pagination limits
5. Execute each template
6. Write output files
7. Log summary statistics

### Error Handling Strategy

- **Config errors:** Exit immediately with clear message
- **Feed errors:** Log error, continue with other feeds
- **Template errors:** Log error, continue with other templates
- **Write errors:** Log and exit (can't continue if output is broken)

## Future Enhancements (Phase 2)

### Concurrent Fetching (Option B)

The interface design allows easy upgrade:

```go
// Current (MVP)
type SequentialFetcher struct { ... }

// Future
type ConcurrentFetcher struct {
    workers    int
    rateLimit  time.Duration
    ...
}

// Both implement:
type Fetcher interface {
    FetchFeeds(feeds []FeedConfig) []FetchResult
}
```

**Implementation approach:**
- Worker pool pattern with configurable worker count
- Channel-based pipeline for feed processing
- Rate limiting per feed to be polite
- Graceful shutdown on interrupt

### Other Future Features

- Additional template engines (if needed)
- Advanced filtering (XSLT, custom plugins)
- Web-based admin interface
- Metrics/monitoring endpoints
- RSS feed change detection (notify on new feeds)
- PubSubHubbub support for real-time updates

## Testing Strategy

### Unit Tests

- Config parsing with various INI formats
- Cache read/write operations
- Filter logic with various regex patterns
- Entry sorting and pagination
- Template data preparation

### Integration Tests

- Full fetch → cache → render pipeline
- Error handling (timeouts, malformed feeds)
- Template rendering with sample data
- Compare output with existing Planet output

### Test Data

- Use existing Planet Clojure config
- Sample RSS/Atom feeds (valid and invalid)
- Mock HTTP responses for deterministic testing

## Migration Plan

### Phase 1: Development & Testing

1. Implement core packages (config, fetcher, parser, cache)
2. Implement renderer with Go templates
3. Create template migration guide
4. Test with Planet Clojure config (read-only)

### Phase 2: Template Migration

1. Convert existing htmltmpl templates to Go syntax
2. Validate output matches current Planet output
3. Test with sample data and visual inspection

### Phase 3: Parallel Deployment

1. Run both Python and Go versions in parallel
2. Compare outputs for consistency
3. Monitor for issues or differences

### Phase 4: Cutover

1. Switch to Go version as primary
2. Keep Python version as backup
3. Monitor for issues

### Phase 5: Optimization

1. Implement concurrent fetching if needed
2. Performance tuning based on real usage
3. Add additional features as needed

## Risk Mitigation

### Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Template syntax differences | Provide migration guide and helper script |
| Output differs from Python version | Extensive testing and parallel deployment |
| Performance not improved | Profile and optimize, add concurrency |
| gofeed doesn't parse some feeds | Fallback parsing, report to gofeed project |
| Complex filters needed | Keep Python version available, plan filter system |

## Success Criteria

**MVP is successful if:**

1. ✅ Parses existing config.ini correctly
2. ✅ Fetches all feeds with proper caching
3. ✅ Applies basic regex filters
4. ✅ Renders templates to match existing output
5. ✅ Single binary deployment (easier than Python)
6. ✅ Faster processing than Python version
7. ✅ Clear logs for debugging
8. ✅ Handles feed errors gracefully

## Appendix: Example Code Snippets

### Fetcher Interface

```go
package fetcher

import "context"

type Fetcher interface {
    FetchFeeds(ctx context.Context, feeds []config.FeedConfig) []FetchResult
}

type FetchResult struct {
    URL     string
    Entries []parser.Entry
    Cached  bool
    Error   error
}

// Sequential implementation (MVP)
type SequentialFetcher struct {
    client  *http.Client
    timeout time.Duration
    cache   *cache.Cache
}

func (f *SequentialFetcher) FetchFeeds(ctx context.Context, feeds []config.FeedConfig) []FetchResult {
    results := make([]FetchResult, 0, len(feeds))
    for _, feed := range feeds {
        result := f.fetchOne(ctx, feed)
        results = append(results, result)
    }
    return results
}
```

### Main Application Flow

```go
package main

func main() {
    // Parse flags
    configPath := flag.String("c", "config.ini", "config file path")
    flag.Parse()

    // Load config
    cfg, err := config.Load(*configPath)
    if err != nil {
        log.Fatal(err)
    }

    // Initialize components
    cache := cache.New(cfg.Planet.CacheDirectory)
    fetcher := fetcher.NewSequential(cfg.Planet.FeedTimeout, cache)
    renderer := renderer.New(cfg.Planet.OutputDir)

    // Fetch feeds
    results := fetcher.FetchFeeds(context.Background(), cfg.Feeds)

    // Load entries from cache
    entries := cache.LoadAll()

    // Apply filters
    filter := filter.New(cfg.Planet.Filter, cfg.Planet.Exclude)
    filtered := filter.Apply(entries)

    // Sort and paginate
    sorted := sort.ByDate(filtered)
    paginated := paginate(sorted, cfg.Planet)

    // Render templates
    for _, tmpl := range cfg.Planet.TemplateFiles {
        err := renderer.Render(tmpl, paginated, cfg)
        if err != nil {
            log.Printf("ERROR: template %s: %v", tmpl, err)
        }
    }

    log.Println("Done")
}
```

## References

- [Planet Venus Documentation](http://www.intertwingly.net/code/venus/docs/)
- [go-ini/ini](https://github.com/go-ini/ini)
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed)
- [Go html/template](https://pkg.go.dev/html/template)
- [Go net/http](https://pkg.go.dev/net/http)
