// Package info gathers and renders what `pen info` shows: the connected Pen's
// details and how the local directory compares.
package info

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/diff"
	"github.com/nocksock/codepen-cli/internal/human"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Status is everything info shows.
type Status struct {
	Pen      codepen.Pen
	Files    []codepen.File
	Versions int
	Changes  codepen.ChangeSet // what a push would send
	LocalErr error             // why the directory couldn't be compared; nil when it was
}

// Gather fetches the Pen, its files and version count, and compares the files
// with the directory root.
func Gather(ctx context.Context, api *codepen.Client, id, root string) (Status, error) {
	var s Status
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		s.Pen, err = api.Pen(gctx, id)
		return err
	})
	g.Go(func() (err error) {
		s.Files, err = api.Files(gctx, id)
		return err
	})
	g.Go(func() (err error) {
		s.Versions, err = api.CountVersions(gctx, id)
		return err
	})
	if err := g.Wait(); err != nil {
		return Status{}, err
	}
	s.Changes, s.LocalErr = compare(root, s.Files)
	return s, nil
}

// compare returns what a push from root would send. Its error goes into the
// Status rather than failing Gather, so info still shows the Pen.
func compare(root string, files []codepen.File) (codepen.ChangeSet, error) {
	local, err := tree.Read(root)
	if err != nil {
		return codepen.ChangeSet{}, err
	}
	return diff.Between(local, diff.ByPath(files)), nil
}

// Render formats s as aligned "label  value" lines, with times relative to now.
func Render(s Status, now time.Time) string {
	p := s.Pen
	return human.Table([]human.Row{
		{Label: "title", Value: p.Title},
		{Label: "id", Value: fmt.Sprintf("%s (%s)", p.ID, p.Editor)},
		{Label: "access", Value: p.Access},
		{Label: "tags", Value: strings.Join(p.Tags, ", ")},
		{Label: "created", Value: human.Ago(p.CreatedAt, now)},
		{Label: "updated", Value: human.Ago(p.UpdatedAt, now)},
		{Label: "versions", Value: versions(s.Versions, p.SavedVersion, now)},
		{Label: "build", Value: build(p.SavedVersion)},
		{Label: "files", Value: size(s.Files)},
		{Label: "local", Value: local(s.Changes, s.LocalErr)},
		{Label: "deployed", Value: deployed(p.Deployment)},
		{Label: "popularity", Value: strconv.FormatFloat(p.Popularity, 'f', -1, 64)},
		{Label: "editor", Value: p.URLs.Editor},
		{Label: "preview", Value: p.URLs.Preview},
	})
}

// JSON is s for scripts: stable snake_case keys, times in RFC 3339, and the
// paths a push would change rather than their counts.
func JSON(s Status) ([]byte, error) {
	p := s.Pen
	view := jsonView{
		ID: p.ID, Title: p.Title, Editor: p.Editor, Access: p.Access, Tags: p.Tags,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Versions: s.Versions,
		Popularity: p.Popularity, URLs: p.URLs,
		Local: jsonLocal{Upload: []string{}, Delete: s.Changes.Deletes},
	}
	if view.Tags == nil {
		view.Tags = []string{}
	}
	if view.Local.Delete == nil {
		view.Local.Delete = []string{}
	}
	if head := p.SavedVersion; head != nil {
		view.Head = &jsonHead{ID: head.ID, CreatedAt: head.CreatedAt}
		if b := head.Build; b != nil {
			view.Build = &jsonBuild{Status: b.Status, Errors: b.Errors}
		}
	}
	for _, f := range s.Files {
		view.Files.Count++
		if f.Content == nil {
			view.Files.Binary++
		} else {
			view.Files.TextBytes += int64(len(*f.Content))
		}
	}
	for _, w := range s.Changes.Writes {
		view.Local.Upload = append(view.Local.Upload, w.Path)
	}
	for _, u := range s.Changes.Uploads {
		view.Local.Upload = append(view.Local.Upload, u.Path)
	}
	slices.Sort(view.Local.Upload)
	if s.LocalErr != nil {
		msg := s.LocalErr.Error()
		view.Local.Error = &msg
	}
	if p.Deployment != nil {
		view.DeployedURL = &p.Deployment.URL
	}
	return json.MarshalIndent(view, "", "  ")
}

type jsonView struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Editor      string       `json:"editor_version"`
	Access      string       `json:"access"`
	Tags        []string     `json:"tags"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Versions    int          `json:"versions"`
	Head        *jsonHead    `json:"head"`
	Build       *jsonBuild   `json:"build"`
	Files       jsonFiles    `json:"files"`
	Local       jsonLocal    `json:"local"`
	DeployedURL *string      `json:"deployed_url"`
	Popularity  float64      `json:"popularity"`
	URLs        codepen.URLs `json:"urls"`
}

type jsonHead struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type jsonBuild struct {
	Status string               `json:"status"`
	Errors []codepen.BuildError `json:"errors"`
}

type jsonFiles struct {
	Count     int   `json:"count"`
	TextBytes int64 `json:"text_bytes"`
	Binary    int   `json:"binary"`
}

type jsonLocal struct {
	Upload []string `json:"upload"`
	Delete []string `json:"delete"`
	Error  *string  `json:"error"` // why the directory couldn't be compared
}

func versions(n int, head *codepen.Version, now time.Time) string {
	out := strconv.Itoa(n)
	if head != nil {
		out += ", head " + human.Ago(head.CreatedAt, now)
	}
	return out
}

func build(head *codepen.Version) string {
	if head == nil {
		return ""
	}
	if head.Build == nil {
		return "none"
	}
	out := head.Build.Status
	if n := len(head.Build.Errors); n > 0 {
		out += ", " + human.Plural(n, "error")
	}
	return out
}

func size(files []codepen.File) string {
	var text int64
	binaries := 0
	for _, f := range files {
		if f.Content == nil {
			binaries++
			continue
		}
		text += int64(len(*f.Content))
	}
	out := fmt.Sprintf("%s, %s text", human.Plural(len(files), "file"), human.Bytes(text))
	if binaries > 0 {
		out += fmt.Sprintf(" + %d binary", binaries)
	}
	return out
}

func local(changes codepen.ChangeSet, err error) string {
	if err != nil {
		return "can't compare: " + err.Error()
	}
	writes := len(changes.Writes) + len(changes.Uploads)
	deletes := len(changes.Deletes)
	if writes == 0 && deletes == 0 {
		return "in sync"
	}
	parts := []string{}
	if writes > 0 {
		parts = append(parts, fmt.Sprintf("%d to upload", writes))
	}
	if deletes > 0 {
		parts = append(parts, fmt.Sprintf("%d to delete", deletes))
	}
	return strings.Join(parts, ", ") + " (pen push)"
}

func deployed(d *codepen.Deployment) string {
	if d == nil {
		return "no"
	}
	return d.URL
}
