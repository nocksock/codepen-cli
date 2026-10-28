package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/human"
	"github.com/nocksock/codepen-cli/internal/pen"
)

// printer shows a Mirror's progress on w. Strings from the API are cleaned
// of control characters first.
type printer struct{ w, err io.Writer }

func (p printer) Report(e pen.Event) {
	if built, ok := e.(pen.Built); ok {
		fmt.Fprint(p.err, renderBuildErrors(built.Build))
	}
	fmt.Fprint(p.w, render(e))
}

func render(e pen.Event) string {
	var b strings.Builder
	switch e := e.(type) {
	case pen.Planned:
		for _, write := range e.Changes.Writes {
			fmt.Fprintf(&b, "  %s %s\n", marker(e.New[write.Path]), human.Clean(write.Path))
		}
		for _, upload := range e.Changes.Uploads {
			fmt.Fprintf(&b, "  %s %s (binary)\n", marker(e.New[upload.Path]), human.Clean(upload.Path))
		}
		for _, p := range e.Changes.Deletes {
			fmt.Fprintf(&b, "  - %s\n", human.Clean(p))
		}
	case pen.Built:
		for _, patch := range e.Build.AppliedPatches {
			fmt.Fprintf(&b, "  patched %s %s\n", human.Clean(patch.Type), human.Clean(patch.Path))
		}
	case pen.Downloaded:
		fmt.Fprintf(&b, "  ↓ %s\n", human.Clean(e.Path))
	case pen.Kept:
		fmt.Fprintf(&b, "  kept %s (local edit; push uploads it)\n", human.Clean(e.Path))
	case pen.Created:
		fmt.Fprintf(&b, "created \"%s\"\n", human.Clean(e.Title))
		b.WriteString(renderURLs(e.URLs))
	case pen.Connected:
		fmt.Fprintf(&b, "connected to \"%s\" (%s)\n", human.Clean(e.Title), human.Clean(e.ID))
		b.WriteString(renderURLs(e.URLs))
	case pen.Pushed:
		fmt.Fprintf(&b, "pushed %s to \"%s\"\n", human.Plural(e.Files, "file"), human.Clean(e.Title))
	case pen.Pulled:
		fmt.Fprintf(&b, "pulled %s\n", human.Plural(e.Files, "file"))
	case pen.WouldPush:
		fmt.Fprintf(&b, "would push %s to \"%s\"\n", human.Plural(e.Files, "file"), human.Clean(e.Title))
	case pen.WouldCreate:
		if e.Files == 0 {
			b.WriteString("would create a Pen from CodePen's starter files\n")
		} else {
			fmt.Fprintf(&b, "would create a Pen with %s\n", human.Plural(e.Files, "file"))
		}
	case pen.UpToDate:
		b.WriteString("already up to date\n")
	}
	return b.String()
}

func marker(isNew bool) string {
	if isNew {
		return "+"
	}
	return "~"
}

// renderURLs indents the table of where the Pen can be opened.
func renderURLs(urls codepen.URLs) string {
	table := human.Table([]human.Row{
		{Label: "editor", Value: urls.Editor},
		{Label: "preview", Value: urls.Preview},
	})
	if table == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(table, "\n", "\n  ") + "\n"
}

func renderBuildErrors(build codepen.Build) string {
	var b strings.Builder
	for _, e := range build.Errors {
		line := ""
		if e.Range != nil && e.Range.Start.Line > 0 {
			line = fmt.Sprintf(":%d", e.Range.Start.Line)
		}
		fmt.Fprintf(&b, "  ✗ %s%s %s\n", human.Clean(e.Path), line, human.Clean(e.Message))
	}
	return b.String()
}
