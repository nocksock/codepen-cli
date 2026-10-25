// Package browser knows how each OS opens a URL in the default browser.
package browser

import (
	"fmt"
	"net/url"
)

// Command returns the program and arguments that open rawURL on goos. Only
// http and https URLs are opened: the URL comes from the API, and other
// schemes (file:, custom app handlers) would launch more than a browser.
func Command(goos, rawURL string) (string, []string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", nil, fmt.Errorf("refusing to open %q: not an http(s) url", rawURL)
	}
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		// Not `cmd /c start`: cmd would interpret & and other characters in the URL.
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "xdg-open", []string{rawURL}, nil
	}
}
