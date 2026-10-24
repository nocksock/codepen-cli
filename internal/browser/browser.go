// Package browser knows how each OS opens a URL in the default browser.
package browser

// Command returns the program and arguments that open url on goos.
func Command(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "cmd", []string{"/c", "start", "", url}
	default:
		return "xdg-open", []string{url}
	}
}
