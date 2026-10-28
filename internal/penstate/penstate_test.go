package penstate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

func TestLoad_NothingSaved(t *testing.T) {
	t.Parallel()
	got, err := Store{Dir: t.TempDir()}.Load("/work/demo")
	if err != nil || got.Pen != "" || len(got.Files) != 0 {
		t.Errorf("Load() = %+v, %v; want an empty state", got, err)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	t.Parallel()
	store := Store{Dir: filepath.Join(t.TempDir(), "pen")}
	saved := State{Pen: "p1", Version: "v1", Files: map[string]string{"/a.js": "abc"}}
	if err := store.Save("/work/demo", saved); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("/work/demo")
	if err != nil || got.Pen != "p1" || got.Version != "v1" || got.Files["/a.js"] != "abc" {
		t.Errorf("Load() = %+v, %v; want what was saved", got, err)
	}
	if other, _ := store.Load("/work/other"); other.Pen != "" {
		t.Errorf("another directory sees %+v; want an empty state", other)
	}
}

// Without a place to keep state, nothing is remembered and nothing fails.
func TestStore_WithoutDir(t *testing.T) {
	t.Parallel()
	if err := (Store{}).Save("/work/demo", State{Pen: "p1"}); err != nil {
		t.Fatal(err)
	}
	if got, err := (Store{}).Load("/work/demo"); err != nil || got.Pen != "" {
		t.Errorf("Load() = %+v, %v; want an empty state", got, err)
	}
}

func TestLoad_Broken(t *testing.T) {
	t.Parallel()
	store := Store{Dir: t.TempDir()}
	if err := os.WriteFile(store.path("/work/demo"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("/work/demo"); err == nil {
		t.Error("Load() of a broken file succeeded; want an error")
	}
}

func TestSum(t *testing.T) {
	t.Parallel()
	text := "body {}"
	raw := sha256.Sum256([]byte(text))
	want := hex.EncodeToString(raw[:])
	if got := Sum([]byte(text)); got != want {
		t.Errorf("Sum() = %s, want %s", got, want)
	}
	if got := FileSum(codepen.File{Content: &text}); got != want {
		t.Errorf("FileSum(text) = %s, want %s", got, want)
	}
	if got := FileSum(codepen.File{ContentHash: "beef", IsBinary: true}); got != "beef" {
		t.Errorf("FileSum(binary) = %s, want the content hash", got)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Parallel()
	env := func(vars map[string]string) func(string) (string, bool) {
		return func(name string) (string, bool) { v, ok := vars[name]; return v, ok }
	}
	tests := []struct {
		name string
		goos string
		vars map[string]string
		want string
	}{
		{name: "xdg", goos: "linux", vars: map[string]string{"XDG_STATE_HOME": "/s", "HOME": "/h"}, want: filepath.Join("/s", "pen")},
		{name: "home", goos: "darwin", vars: map[string]string{"HOME": "/h"}, want: filepath.Join("/h", ".local", "state", "pen")},
		{name: "windows", goos: "windows", vars: map[string]string{"LOCALAPPDATA": `C:\L`}, want: filepath.Join(`C:\L`, "pen", "state")},
		{name: "nowhere", goos: "linux", vars: map[string]string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DefaultDir(tt.goos, env(tt.vars)); got != tt.want {
				t.Errorf("DefaultDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
