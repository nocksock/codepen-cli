package info

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestRender(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	content := "12345"
	status := Status{
		Pen: codepen.Pen{
			ID: "p1", Title: "Demo", Editor: "V2", Access: "Private", Tags: []string{"api", "demo"},
			CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-time.Hour),
			URLs:         codepen.URLs{Editor: "https://e", Preview: "https://p"},
			SavedVersion: &codepen.Version{CreatedAt: now.Add(-time.Hour), Build: &codepen.Build{Status: "Success"}},
		},
		Files:    []codepen.File{{Path: "/a.js", Content: &content}, {Path: "/dot.png"}},
		Versions: 3,
		Changes:  codepen.ChangeSet{Writes: []codepen.Write{{Path: "/b.js"}}, Deletes: []string{"/c.js"}},
	}
	expected := strings.Join([]string{
		"title       Demo",
		"id          p1 (V2)",
		"access      Private",
		"tags        api, demo",
		"created     2 days ago",
		"updated     1 hour ago",
		"versions    3, head 1 hour ago",
		"build       Success",
		"files       2 files, 5 B text + 1 binary",
		"local       1 to upload, 1 to delete (pen push)",
		"deployed    no",
		"popularity  0",
		"editor      https://e",
		"preview     https://p",
	}, "\n")
	if got := Render(status, now); got != expected {
		t.Errorf("Render() =\n%s\nwant\n%s", got, expected)
	}
}

func TestRender_InSyncWithBuildErrors(t *testing.T) {
	t.Parallel()
	status := Status{Pen: codepen.Pen{
		SavedVersion: &codepen.Version{Build: &codepen.Build{Status: "Failure", Errors: []codepen.BuildError{{}, {}}}},
		Deployment:   &codepen.Deployment{URL: "https://live"},
	}}
	got := Render(status, time.Now())
	for _, line := range []string{"build       Failure, 2 errors", "local       in sync", "deployed    https://live"} {
		if !strings.Contains(got, line) {
			t.Errorf("Render() lacks %q:\n%s", line, got)
		}
	}
}

func TestRender_BuildAndLocalVariants(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tests := []struct {
		name   string
		status Status
		want   string
		absent string
	}{
		{name: "head without build", status: Status{Pen: codepen.Pen{SavedVersion: &codepen.Version{}}}, want: "build       none"},
		{name: "no head drops build", status: Status{}, absent: "build "},
		{name: "single error", status: Status{Pen: codepen.Pen{SavedVersion: &codepen.Version{Build: &codepen.Build{Status: "Failure", Errors: []codepen.BuildError{{}}}}}}, want: "build       Failure, 1 error\n"},
		{name: "uploads only", status: Status{Changes: codepen.ChangeSet{Uploads: []codepen.Upload{{Path: "/a.png"}}}}, want: "local       1 to upload (pen push)"},
		{name: "deletes only", status: Status{Changes: codepen.ChangeSet{Deletes: []string{"/a"}}}, want: "local       1 to delete (pen push)"},
		{name: "unreadable directory", status: Status{LocalErr: errors.New("no such directory")}, want: "local       can't compare: no such directory\n"},
		{name: "single file", status: Status{Files: []codepen.File{{Path: "/a"}}}, want: "files       1 file, 0 B text + 1 binary"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Render(tt.status, now) + "\n"
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("Render() lacks %q:\n%s", tt.want, got)
			}
			if tt.absent != "" && strings.Contains(got, tt.absent) {
				t.Errorf("Render() has %q:\n%s", tt.absent, got)
			}
		})
	}
}

// A directory that can't be read must not look like one whose files were all
// deleted: info would then advise a push that deletes everything.
func TestGather_UnreadableDirectory(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	client := codepen.New(codepentest.Key, codepen.WithBaseURL(api.URL()))
	s, err := Gather(t.Context(), client, id, filepath.Join(t.TempDir(), "typo"))
	if err != nil {
		t.Fatal(err)
	}
	if s.LocalErr == nil || !s.Changes.IsEmpty() {
		t.Errorf("LocalErr = %v, Changes = %+v; want an error and no changes", s.LocalErr, s.Changes)
	}
}
