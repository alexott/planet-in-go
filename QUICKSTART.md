# Quick Start Guide

Get Planet Go up and running in 5 minutes!

## Step 1: Install

### Option A: Build from Source (Recommended)

```bash
# Clone the repository
git clone https://github.com/alexey-ott/planet-go
cd planet-go

# Build the binary
make build

# Verify installation
./planet version
```

### Option B: Go Install

```bash
go install github.com/alexey-ott/planet-go/cmd/planet@latest

# Verify installation
planet version
```

## Step 2: Try the Example

Run the included example to verify everything works:

```bash
make run-example
```

This will:
1. Fetch the Go Blog feed
2. Cache the entries
3. Render a beautiful HTML page
4. Save output to `example-output/simple-template.html`

Open `example-output/simple-template.html` in your browser to see the result!

## Step 3: Create Your Config

Create a file named `config.ini`:

```ini
[Planet]
name = My Awesome Planet
link = http://planet.example.com
cache_directory = ./cache
output_dir = ./output
log_level = INFO
feed_timeout = 20
items_per_page = 15
template_files = templates/index.html.tmpl

# Add your feeds here
[https://go.dev/blog/feed.atom]
name = The Go Blog

[https://blog.golang.org/feed.atom]
name = Go Blog

# Add more feeds...
```

## Step 4: Create Your Template

Create `templates/index.html.tmpl`:

```html
<!DOCTYPE html>
<html>
<head>
    <title>{{.Name}}</title>
    <style>
        body { font-family: sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        article { margin-bottom: 30px; border-bottom: 1px solid #ccc; padding-bottom: 20px; }
    </style>
</head>
<body>
    <h1>{{.Name}}</h1>
    <p>Last updated: {{.Date}}</p>

    {{range .Items}}
    <article>
        <h2><a href="{{.Link}}">{{.Title}}</a></h2>
        <p>
            {{if .Author}}by {{.Author}} • {{end}}
            from <a href="{{.ChannelLink}}">{{.ChannelName}}</a>
        </p>
        <div>{{.Content}}</div>
    </article>
    {{end}}
</body>
</html>
```

Or use the example templates:

```bash
# Copy the simple template
mkdir -p templates
cp examples/simple-template.html.tmpl templates/index.html.tmpl
```

## Step 5: Run It!

```bash
./planet -c config.ini
```

You should see output like:

```
INFO  starting planet feeds=2
INFO  fetching feeds
INFO  feed fetched url=https://go.dev/blog/feed.atom entries=10
INFO  feed fetched url=https://blog.golang.org/feed.atom entries=10
INFO  fetch complete success=2 errors=0
INFO  loaded entries count=20
INFO  rendering templates count=1
INFO  template rendered file=templates/index.html.tmpl
INFO  done entries=20
```

Your rendered page is now in `output/index.html`! 🎉

### Debug Mode

If you have issues or want to see what's happening in detail:

```bash
./planet run -c config.ini -debug
```

This shows:
- HTTP connection details
- Request/response timing
- Parse durations
- Cache operations
- Detailed error messages

## Advanced Usage

### Separate Fetch and Render

You can fetch feeds and render templates separately:

```bash
# Fetch all feeds once
./planet fetch -c config.ini

# Render templates multiple times (useful when editing templates)
./planet render -c config.ini
# ... edit template ...
./planet render -c config.ini  # Re-render with changes
```

This is useful when:
- Testing template changes without re-fetching feeds
- Debugging rendering issues
- Working with large feed lists that take time to fetch

## Step 6: Automate It

### With Cron

Add to your crontab:

```bash
# Update every hour (fetch and render)
0 * * * * cd /path/to/planet-go && ./planet run -c config.ini

# Or use default behavior (same as "run")
0 * * * * cd /path/to/planet-go && ./planet -c config.ini
```

### With systemd Timer

Create `/etc/systemd/system/planet.service`:

```ini
[Unit]
Description=Planet Go Feed Aggregator

[Service]
Type=oneshot
WorkingDirectory=/path/to/planet-go
ExecStart=/path/to/planet-go/planet run -c config.ini
```

Create `/etc/systemd/system/planet.timer`:

```ini
[Unit]
Description=Run Planet Go hourly

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
```

Enable and start:

```bash
sudo systemctl enable planet.timer
sudo systemctl start planet.timer
```

## Common Configurations

### Basic Planet

```ini
[Planet]
name = My Planet
link = http://planet.example.com
cache_directory = ./cache
output_dir = ./output
items_per_page = 20
template_files = index.html.tmpl

[https://example.com/feed.xml]
name = Example Blog
```

### With Filtering

```ini
[Planet]
name = Clojure Planet
# Only include Clojure-related posts
filter = Clojure|ClojureScript|clj
# Exclude job postings and ads
exclude = hiring|job opening|advertisement
...
```

### Multiple Templates

```ini
[Planet]
template_files = index.html.tmpl atom.xml.tmpl rss.xml.tmpl
...
```

### Show Only Recent Posts

```ini
[Planet]
# Only posts from last 7 days
days_per_page = 7
# Max 30 items
items_per_page = 30
...
```

## Troubleshooting

### Feeds hanging or timing out

**Problem:** Planet hangs for several minutes when fetching feeds.

**Solution 1:** Enable debug mode to see what's happening:

```bash
./planet -debug -c config.ini
```

This shows exactly which feed is slow or hanging.

**Solution 2:** Increase the timeout:

```ini
[Planet]
feed_timeout = 60  # 60 seconds instead of default 20
```

**Note:** With the improved timeout handling, connections now timeout after:
- 10 seconds for initial connection
- 10 seconds for TLS handshake
- 20 seconds (or config value) for reading response

### "feed timeout"

If you see timeout errors in debug mode, the feed might be:
- Down or unreachable
- Very slow to respond
- Blocking connections

Try accessing the feed URL in your browser to verify it works.

### "template failed"

Check your template syntax:
- Use `{{.Name}}` not `<TMPL_VAR name>`
- Close all `{{range}}` with `{{end}}`
- Field names are CamelCase: `.AuthorName` not `.author_name`

See [MIGRATION.md](docs/MIGRATION.md) for template conversion guide.

### "cache directory not found"

Create the directories:

```bash
mkdir -p cache output
```

Or Planet Go will create them automatically on first run.

### Debug output shows connection refused

The feed server might be:
- Blocking your IP (check if you're making too many requests)
- Behind a firewall
- Temporarily down

Planet Go will use cached data if available.

## Next Steps

- **Add more feeds:** Just add `[URL]` sections to your config
- **Customize templates:** Copy from `examples/` and modify
- **Filter content:** Use `filter` and `exclude` patterns
- **Deploy:** Set up cron/systemd for automatic updates

## Resources

- **Full Documentation:** [README.md](README.md)
- **Migration Guide:** [docs/MIGRATION.md](docs/MIGRATION.md)
- **Example Templates:** [examples/](examples/)
- **Design Document:** [docs/plans/2025-01-08-planet-go-design.md](docs/plans/2025-01-08-planet-go-design.md)

## Getting Help

- **Issues:** https://github.com/alexey-ott/planet-go/issues
- **Discussions:** https://github.com/alexey-ott/planet-go/discussions

## Tips

### Performance

Run with DEBUG logging to see what's happening:

```ini
[Planet]
log_level = DEBUG
```

### Debugging

Check cache files to see what was fetched:

```bash
ls -lh cache/
cat cache/*.json | jq '.entries[] | {title, date}'
```

### Testing

Use `make run-example` to quickly test changes without affecting your main setup.

### Backup

Keep backups of your config and templates:

```bash
cp config.ini config.ini.backup
```

## Success!

You now have a working Planet aggregator! 🚀

Customize it to your needs and enjoy fast, reliable feed aggregation.

