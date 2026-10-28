// Package ref turns anything that points at a Pen into its API id:
//
//	01a120bd-45fb-7095-a3ea-622e774cf8f0               2.0 Pen id
//	PwWmgeN                                            Classic Pen id
//	https://codepen.io/editor/user/pen/<uuid>/<hash>   2.0 editor
//	https://codepen.io/user/pen/PwWmgeN  (/full, …)    Classic / public
//	https://codepen.io/pen/?template=<id>              template
//	codepen.io/user/embed/preview/PwWmgeN              without scheme
package ref

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var (
	uuidPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	classicPattern = regexp.MustCompile(`^[A-Za-z0-9]{5,10}$`)
	schemePattern  = regexp.MustCompile(`(?i)^[a-z]+://`)
)

// views are path segments naming a view of a Pen; the Classic id follows them.
var views = map[string]bool{
	"pen": true, "full": true, "details": true, "debug": true, "embed": true, "preview": true,
	"pres": true, "live": true, "fullpage": true, "fullcpgrid": true, "fullembedgrid": true,
}

// prefixes start a URL path before the user name, so the id doesn't follow them.
var prefixes = map[string]bool{"editor": true}

// Parse returns the Pen id that ref points at: a bare id or any URL containing one.
func Parse(ref string) (string, error) {
	value := strings.TrimSpace(ref)
	if isID(value) {
		return value, nil
	}
	if id := fromURL(value); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("can't find a Pen id in %q", ref)
}

func isID(value string) bool {
	return uuidPattern.MatchString(value) || (classicPattern.MatchString(value) && !views[value] && !prefixes[value])
}

func fromURL(value string) string {
	u, ok := parseLoose(value)
	if !ok {
		return ""
	}
	if template := u.Query().Get("template"); isID(template) {
		return template
	}
	// EscapedPath, not Path: ids are matched against the still-encoded path.
	segments := strings.FieldsFunc(u.EscapedPath(), func(r rune) bool { return r == '/' })
	if id := firstUUID(segments); id != "" {
		return id
	}
	return afterView(segments)
}

// parseLoose parses value as a URL, assuming https when the scheme is missing.
func parseLoose(value string) (*url.URL, bool) {
	if !schemePattern.MatchString(value) {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	return u, err == nil
}

// firstUUID returns the first segment that is a UUID.
func firstUUID(segments []string) string {
	for _, segment := range segments {
		if uuidPattern.MatchString(segment) {
			return segment
		}
	}
	return ""
}

// afterView returns the id right after the first run of view segments, as in
// /user/pen/<id> or /user/embed/preview/<id>; "" when something else is there.
func afterView(segments []string) string {
	i := slices.IndexFunc(segments, func(s string) bool { return views[s] })
	if i < 0 {
		return ""
	}
	for i < len(segments) && views[segments[i]] {
		i++
	}
	if i < len(segments) && isID(segments[i]) {
		return segments[i]
	}
	return ""
}
