# Template Examples

This directory contains example templates in Go template syntax for Planet Go.

## Available Templates

### simple-template.html.tmpl

A clean, modern HTML template with:
- Responsive design (mobile-friendly)
- Date grouping headers
- Author and channel attribution
- Semantic HTML5 markup
- Embedded CSS styling

**Output:** `simple-template.html`

**Usage:**
```ini
[Planet]
template_files = examples/simple-template.html.tmpl
output_dir = ./output
```

### atom-template.xml.tmpl

A standard Atom feed (RFC 4287) template with:
- Feed metadata (title, author, generator)
- Entry details with content
- Source attribution for each entry
- Proper XML namespaces

**Output:** `atom-template.xml`

**Usage:**
```ini
[Planet]
template_files = examples/atom-template.xml.tmpl
output_dir = ./output
```

## Using These Templates

### Method 1: Reference in Config

Add to your `config.ini`:

```ini
[Planet]
name = My Planet
link = http://planet.example.com
cache_directory = ./cache
output_dir = ./output
template_files = examples/simple-template.html.tmpl examples/atom-template.xml.tmpl
```

### Method 2: Copy and Customize

```bash
# Copy template to your config directory
cp examples/simple-template.html.tmpl mytemplate.html.tmpl

# Edit as needed
vim mytemplate.html.tmpl

# Reference in config
[Planet]
template_files = mytemplate.html.tmpl
```

## Template Syntax Reference

### Variables

Access data with dot notation:

```html
{{.Name}}           <!-- Planet name -->
{{.Link}}           <!-- Planet URL -->
{{.Date}}           <!-- Formatted date -->
{{.OwnerName}}      <!-- Owner name -->
```

### Loops

Iterate over items or channels:

```html
{{range .Items}}
    <h2>{{.Title}}</h2>
    <p>{{.Content}}</p>
{{end}}
```

### Conditionals

Show content conditionally:

```html
{{if .Author}}
    <p>By {{.Author}}</p>
{{end}}

{{if .NewDate}}
    <h3>{{.Date}}</h3>
{{end}}
```

### Available Data

**Top-level (outside loops):**
- `.Name` - Planet name
- `.Link` - Planet URL
- `.OwnerName` - Owner name
- `.OwnerEmail` - Owner email
- `.Generator` - Generator string
- `.Date` - Current date (formatted)
- `.DateISO` - Current date (ISO 8601)
- `.Items` - Array of entries
- `.Channels` - Array of feeds

**Inside `{{range .Items}}`:**
- `.Title` - Entry title
- `.Link` - Entry URL
- `.Content` - Entry content (HTML)
- `.Author` - Author name
- `.AuthorEmail` - Author email
- `.Date` - Entry date (formatted)
- `.DateISO` - Entry date (ISO 8601)
- `.ChannelName` - Feed name
- `.ChannelLink` - Feed URL
- `.ChannelTitle` - Feed title
- `.NewDate` - Boolean, true if date changed
- `.NewChannel` - Boolean, true if channel changed

**Inside `{{range .Channels}}`:**
- `.Name` - Channel name
- `.Link` - Channel URL
- `.Title` - Channel title

## Customization Tips

### Styling

The HTML template includes inline CSS. To customize:

1. **Colors:** Change color values in `<style>` tag
   ```css
   h1 { color: #ff6600; }  /* Change from blue to orange */
   ```

2. **Fonts:** Modify `font-family` values
   ```css
   body { font-family: Georgia, serif; }
   ```

3. **Layout:** Adjust `max-width`, `padding`, `margin`
   ```css
   body { max-width: 1200px; }  /* Wider layout */
   ```

### Content

Modify what's displayed:

```html
<!-- Show only titles (no content) -->
{{range .Items}}
    <h2><a href="{{.Link}}">{{.Title}}</a></h2>
{{end}}

<!-- Show full content with images -->
{{range .Items}}
    <article>
        <h2>{{.Title}}</h2>
        <div>{{.Content}}</div>
    </article>
{{end}}
```

### Filtering

Use conditionals to filter display:

```html
<!-- Only show entries with authors -->
{{range .Items}}
    {{if .Author}}
        <article>...</article>
    {{end}}
{{end}}
```

## Advanced Examples

### Sidebar with Channel List

```html
<aside>
    <h3>Feeds</h3>
    <ul>
    {{range .Channels}}
        <li><a href="{{.Link}}">{{.Name}}</a></li>
    {{end}}
    </ul>
</aside>
```

### Group by Channel

```html
{{range .Items}}
    {{if .NewChannel}}
        <h2>{{.ChannelName}}</h2>
    {{end}}
    <article>
        <h3>{{.Title}}</h3>
        {{.Content}}
    </article>
{{end}}
```

### Limit Entry Length

Note: Go templates don't have built-in string truncation. Either:
1. Use `items_per_page` in config to limit count
2. Use CSS to truncate display:
   ```css
   .content { 
       max-height: 200px; 
       overflow: hidden; 
   }
   ```

## Troubleshooting

### Template Won't Parse

**Error:** `unexpected "}" in command`

**Solution:** Check that every `{{range}}` and `{{if}}` has a `{{end}}`

### Variable Not Found

**Error:** `can't evaluate field X`

**Solution:** Check capitalization (`.Author` not `.author`) and spelling

### HTML Not Rendering

**Issue:** HTML tags appear as text

**Solution:** Content is already marked as safe HTML. If you're adding your own HTML:
```html
<!-- This will be escaped (shown as text): -->
<div>{{.MyHTML}}</div>

<!-- To render as HTML, it must be template.HTML type in the code -->
```

### Whitespace Issues

**Issue:** Extra blank lines or spaces

**Solution:** Use `{{-` and `-}}` to trim whitespace:
```html
{{- range .Items -}}
    <article>{{.Title}}</article>
{{- end -}}
```

## Contributing Templates

Have a great template? Consider contributing:

1. Create a new template file
2. Add documentation comments
3. Test with various feeds
4. Submit a pull request

## Resources

- [Go template documentation](https://pkg.go.dev/text/template)
- [HTML template documentation](https://pkg.go.dev/html/template)
- [Planet Go README](../README.md)
- [Migration Guide](../docs/MIGRATION.md)

