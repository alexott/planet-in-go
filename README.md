# Planet Go

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

A modern, fast reimplementation of the Planet feed aggregator in Go.

Originally designed for Planet Clojure, replacing the unmaintained Python 2.x Venus/Planet.

## Features

- **Fast RSS/Atom feed fetching** with HTTP conditional GET caching
- **Flexible template rendering** with Go's html/template
- **Regex-based content filtering** (include/exclude patterns)
- **Graceful error handling** - continues on individual feed failures
- **Single binary deployment** - no dependencies to install
- **Structured logging** with customizable log levels

## Installation

### From Source

```bash
git clone https://github.com/alexey-ott/planet-go
cd planet-go
go build -o planet ./cmd/planet
```

Or with Go directly:

```bash
go install github.com/alexey-ott/planet-go/cmd/planet@latest
```

### Requirements

- Go 1.21 or later (for building from source)

## Usage

```bash
# Basic usage - fetch and render (uses config.ini by default)
./planet
./planet -c path/to/config.ini

# Explicit subcommands
./planet run -c config.ini           # Fetch feeds and render templates (default)
./planet fetch -c config.ini         # Only fetch and cache feeds
./planet render -c config.ini        # Only render templates from cache

# Other commands
./planet version                     # Show version information
./planet --help                      # Show help message

# Enable debug logging (shows detailed timing and connection info)
./planet run -c config.ini -debug
./planet fetch -c config.ini -debug
```

### Workflow Examples

```bash
# Typical workflow: fetch and render together
./planet run -c config.ini

# Separate workflow: fetch first, then render multiple times
./planet fetch -c config.ini         # Fetch all feeds once
./planet render -c config.ini        # Render with current template
# ... edit templates ...
./planet render -c config.ini        # Re-render with updated template

# Debug a slow feed
./planet fetch -c config.ini -debug  # Shows timing for each feed
```

## Configuration

Planet Go uses the same INI format as Venus/Planet. See `docs/MIGRATION.md` for full config documentation.

### Minimal Example

```ini
[Planet]
name = My Planet
link = http://planet.example.com
cache_directory = ./cache
output_dir = ./output
log_level = INFO
feed_timeout = 20
items_per_page = 15
template_files = index.html.tmpl

[https://example.com/feed.xml]
name = Example Blog
```

### Configuration Options

**[Planet] Section:**
- `name` - Planet name
- `link` - Planet URL
- `owner_name` - Owner name (optional)
- `owner_email` - Owner email (optional)
- `cache_directory` - Directory for cached feed data
- `output_dir` - Directory for rendered output
- `log_level` - Logging level: DEBUG, INFO, WARNING, ERROR
- `feed_timeout` - HTTP timeout in seconds (default: 20)
- `items_per_page` - Max items per page (default: 15)
- `days_per_page` - Only show items from last N days (default: 0 = all)
- `date_format` - Date format string (default: "%B %d, %Y %I:%M %p")
- `template_files` - Space-separated list of template files
- `filter` - Regex pattern for including entries (optional)
- `exclude` - Regex pattern for excluding entries (optional)

**Feed Sections:**
- Section name is the feed URL (must start with http:// or https://)
- `name` - Display name for the feed
- Additional custom fields are stored and available in templates

## Templates

Planet Go uses Go's `html/template` package. Templates must be migrated from htmltmpl syntax:

| htmltmpl | Go template |
|----------|-------------|
| `<TMPL_VAR name>` | `{{.Name}}` |
| `<TMPL_LOOP Items>...</TMPL_LOOP>` | `{{range .Items}}...{{end}}` |
| `<TMPL_IF foo>...</TMPL_IF>` | `{{if .Foo}}...{{end}}` |

See `docs/MIGRATION.md` for complete migration guide and `examples/` for sample templates.

### Template Data Structure

Available variables in templates:

**Top-level:**
- `.Name` - Planet name
- `.Link` - Planet link
- `.OwnerName` - Owner name
- `.OwnerEmail` - Owner email
- `.Generator` - Generator string ("Planet Go")
- `.Date` - Formatted current date
- `.DateISO` - ISO 8601 current date
- `.Items` - Array of entries
- `.Channels` - Array of channels (feeds)

**Inside `{{range .Items}}`:**
- `.Title` - Entry title
- `.Link` - Entry link
- `.Content` - Entry content (HTML, auto-escaped)
- `.Author` - Author name
- `.AuthorEmail` - Author email
- `.Date` - Formatted date
- `.DateISO` - ISO 8601 date
- `.ID` - Entry ID
- `.ChannelName` - Feed name
- `.ChannelLink` - Feed link
- `.ChannelTitle` - Feed title
- `.NewDate` - Boolean, true if date differs from previous entry
- `.NewChannel` - Boolean, true if channel differs from previous entry

## Development

### Running Tests

```bash
# Unit tests
go test ./...

# Specific package
go test ./internal/config -v

# With coverage
go test ./... -cover
```

### Project Structure

```
planet-go/
├── cmd/planet/          # CLI entry point
├── internal/
│   ├── config/          # Configuration parsing
│   ├── cache/           # File-based caching
│   ├── fetcher/         # Feed fetching
│   ├── filter/          # Content filtering
│   └── renderer/        # Template rendering
├── docs/                # Documentation
└── examples/            # Example templates
```

## Performance

Typical performance on modern hardware:

- **Feed fetching:** ~100 feeds in 30-60 seconds (network dependent)
- **Caching:** ~1000 entries/second
- **Filtering:** ~10000 entries/second
- **Rendering:** ~5000 entries/second

Performance will improve significantly with concurrent fetching (planned for future release).

## Migration from Venus/Planet

See `docs/MIGRATION.md` for step-by-step migration guide.

**Key differences:**
1. Templates must be converted to Go template syntax
2. Only basic regex filtering supported (no complex plugins)
3. Single template engine (Go templates only)
4. No Twitter integration or PubSubHubbub support in MVP

## Troubleshooting

### Templates Don't Render

**Error:** Template parsing failed

**Solution:** Check for htmltmpl syntax that wasn't converted:
- Replace `<TMPL_VAR>` with `{{.}}`
- Add `{{end}}` for all loops/conditionals
- Check capitalization (Go templates are case-sensitive)

### Feeds Not Fetching

**Error:** Feed timeout or connection errors

**Solution:**
- Increase `feed_timeout` in config
- Check network connectivity
- Verify feed URLs are accessible

### Output Differs from Venus

**Cause:** Date formatting, sorting, or filtering differences

**Solution:**
- Check `date_format` in config
- Verify `filter` and `exclude` patterns
- Compare cache contents between versions

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Credits

Based on the Venus/Planet feed aggregator. Reimplemented in Go by Alexey Ott.

**Dependencies:**
- [go-ini/ini](https://github.com/go-ini/ini) - INI parsing
- [mmcdole/gofeed](https://github.com/mmcdole/gofeed) - RSS/Atom parsing

## Roadmap

**v0.1.0 (MVP)** - Current
- ✅ Sequential feed fetching
- ✅ File-based caching
- ✅ Go template rendering
- ✅ Basic regex filtering

**v0.2.0 (Planned)**
- Concurrent feed fetching
- Rate limiting
- Improved error recovery
- Metrics/monitoring

**Future**
- Additional template engines
- Advanced filtering
- Web-based admin interface
- PubSubHubbub support
