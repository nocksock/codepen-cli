package filter

import "testing"

func TestMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.map", "/a.map", true},
		{"*.map", "/src/deep/b.map", true},
		{"*.map", "/a.js", false},
		{"src/", "/src/a.js", true},
		{"src/", "/lib/src/a.js", true},
		{"src/", "/src", false}, // a file named src is not a folder
		{"src", "/src", true},
		{"src", "/src/a.js", true},
		{"/src", "/lib/src/a.js", false},
		{"src/drafts/", "/src/drafts/x.js", true},
		{"src/drafts/", "/lib/src/drafts/x.js", false}, // a middle slash anchors
		{"index.html", "/index.html", true},
		{"index.html", "/sub/index.html", true},
		{"/index.html", "/sub/index.html", false},
		{"src/**/*.test.js", "/src/a.test.js", true},
		{"src/**/*.test.js", "/src/x/y/a.test.js", true},
		{"src/**", "/src/x/y.js", true},
		{"**/fixtures/", "/a/b/fixtures/c.json", true},
		{"a?.js", "/ab.js", true},
		{"*.MAP", "/x.map", true}, // case doesn't matter
		{"DIST/", "/dist/x.js", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.path, func(t *testing.T) {
			t.Parallel()
			if got := Match(tt.pattern, tt.path); got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

func TestFilter_Skips(t *testing.T) {
	t.Parallel()
	f, err := New([]string{"src/", "index.html"}, []string{"*.map"})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		"/index.html":           false,
		"/src/a.js":             false,
		"/src/a.js.map":         true, // excluded although included
		"/readme.md":            true, // not included
		"/src/.env":             true, // built-in ignores always win
		"/.codepen.config.json": true,
	} {
		if got := f.Skips(path); got != want {
			t.Errorf("Skips(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestFilter_Empty(t *testing.T) {
	t.Parallel()
	var f *Filter
	if f.Skips("/a.js") || !f.Skips("/.git/config") {
		t.Error("a nil Filter should skip only the built-in ignores")
	}
}

// Folders are only skipped when nothing inside them could sync, so a watcher
// still descends into src/ when only src/a.js is included.
func TestFilter_SkipsDir(t *testing.T) {
	t.Parallel()
	f, err := New([]string{"src/a.js", "lib/**/*.js"}, []string{"drafts/"})
	if err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]bool{
		"/src":          false,
		"/lib":          false,
		"/lib/x":        false,
		"/drafts":       true,
		"/src/drafts":   true,
		"/other":        false, // can't tell cheaply; files inside are filtered anyway
		"/node_modules": true,
	} {
		if got := f.SkipsDir(dir); got != want {
			t.Errorf("SkipsDir(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestNew_RejectsBadPatterns(t *testing.T) {
	t.Parallel()
	for _, pattern := range []string{"", "  ", "src/[", "a/../b"} {
		if _, err := New(nil, []string{pattern}); err == nil {
			t.Errorf("New(exclude %q) succeeded; want an error", pattern)
		}
	}
}
