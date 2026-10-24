// Package info gathers and renders what `cdpn info` shows: the connected Pen's
// details and how the local directory compares.
package info

import (
	"context"
	"fmt"
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
	s.Changes = diff.Between(localFiles(root), diff.ByPath(s.Files))
	return s, nil
}

// localFiles snapshots root; an unreadable directory counts as empty, so info
// still shows the Pen.
func localFiles(root string) tree.Snapshot {
	local, err := tree.Read(root)
	if err != nil {
		return tree.Snapshot{}
	}
	return local
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
		{Label: "local", Value: local(s.Changes)},
		{Label: "deployed", Value: deployed(p.Deployment)},
		{Label: "popularity", Value: strconv.FormatFloat(p.Popularity, 'f', -1, 64)},
		{Label: "editor", Value: p.URLs.Editor},
		{Label: "preview", Value: p.URLs.Preview},
	})
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

func local(changes codepen.ChangeSet) string {
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
	return strings.Join(parts, ", ") + " (cdpn push)"
}

func deployed(d *codepen.Deployment) string {
	if d == nil {
		return "no"
	}
	return d.URL
}
