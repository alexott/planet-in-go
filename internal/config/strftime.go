package config

import "strings"

// strftimeToGoLayout converts a subset of strftime-style directives to Go
// time.Format layout strings. It intentionally supports the common tokens used
// in the example `config.ini` (e.g. "%B %d, %Y %I:%M %p"). Tokens that are
// not present are left untouched.
func strftimeToGoLayout(s string) string {
	// Replacement pairs: strftime -> Go layout
	// Order matters for tokens where one is prefix of another.
	r := strings.NewReplacer(
		"%%", "%",
		"%A", "Monday",
		"%a", "Mon",
		"%B", "January",
		"%b", "Jan",
		"%d", "02",
		"%e", "_2",
		"%H", "15",
		"%I", "03",
		"%m", "01",
		"%M", "04",
		"%S", "05",
		"%p", "PM",
		"%Y", "2006",
		"%y", "06",
		"%Z", "MST",
		"%z", "-0700",
	)

	return r.Replace(s)
}
