// Package human formats values for people reading a terminal.
package human

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var units = []string{"B", "KB", "MB", "GB"}

// Bytes formats n as "18 B", "1.5 KB", "3.0 MB".
func Bytes(n int64) string {
	value, unit := float64(n), 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", n, units[0])
	}
	// Round half up like JS toFixed(1); %.1f would round ties to even.
	return fmt.Sprintf("%.1f %s", math.Round(value*10)/10, units[unit])
}

// Plural formats "1 file" or "2 files".
func Plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

type span struct {
	unit    string
	seconds float64
}

var spans = []span{
	{unit: "year", seconds: 31536000},
	{unit: "month", seconds: 2592000},
	{unit: "day", seconds: 86400},
	{unit: "hour", seconds: 3600},
	{unit: "minute", seconds: 60},
}

// named holds the words that replace -1, 0 and +1 of a unit, as
// Intl.RelativeTimeFormat("en", {numeric: "auto"}) does.
var named = map[string][3]string{
	"second": {"", "now", ""},
	"minute": {"", "this minute", ""},
	"hour":   {"", "this hour", ""},
	"day":    {"yesterday", "today", "tomorrow"},
	"month":  {"last month", "this month", "next month"},
	"year":   {"last year", "this year", "next year"},
}

// spanFor picks the largest unit that fits seconds, falling back to seconds.
func spanFor(seconds float64) (unit string, size float64) {
	for _, s := range spans {
		if math.Abs(seconds) >= s.seconds {
			return s.unit, s.seconds
		}
	}
	return "second", 1
}

// Ago formats t relative to now: "now", "5 minutes ago", "yesterday", "in 2 days".
func Ago(t, now time.Time) string {
	seconds := t.Sub(now).Seconds()
	unit, size := spanFor(seconds)
	// floor(x+0.5) matches JS Math.round (half rounds toward +Inf).
	return relative(int(math.Floor(seconds/size+0.5)), unit)
}

func relative(n int, unit string) string {
	if n >= -1 && n <= 1 {
		if word := named[unit][n+1]; word != "" {
			return word
		}
	}
	if n < 0 {
		return Plural(-n, unit) + " ago"
	}
	return "in " + Plural(n, unit)
}

// Row is one "label  value" line of a Table.
type Row struct {
	Label string
	Value string
}

// Table aligns rows into "label  value" lines and drops rows without a value.
// Values are cleaned (see Clean), since they often come from elsewhere.
func Table(rows []Row) string {
	shown := []Row{}
	width := 0
	for _, row := range rows {
		if row.Value == "" {
			continue
		}
		shown = append(shown, row)
		width = max(width, len(row.Label))
	}
	lines := make([]string, 0, len(shown))
	for _, row := range shown {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, row.Label, Clean(row.Value)))
	}
	return strings.Join(lines, "\n")
}

// Clean escapes control characters (\x1b, \n, ...) and bidi controls in s, so
// a string from elsewhere can't move the cursor, recolour the terminal or
// reorder what is shown around it. Invalid UTF-8 becomes U+FFFD; everything
// else is kept as is.
func Clean(s string) string {
	if utf8.ValidString(s) && strings.IndexFunc(s, isUnsafe) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if !isUnsafe(r) {
			b.WriteRune(r)
			continue
		}
		quoted := strconv.QuoteRune(r)
		b.WriteString(quoted[1 : len(quoted)-1])
	}
	return b.String()
}

func isUnsafe(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
}
