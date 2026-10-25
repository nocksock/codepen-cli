package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/pen"
)

// printer shows a Mirror's progress on w.
type printer struct{ w io.Writer }

func (p printer) Report(e pen.Event) {
	fmt.Fprint(p.w, render(e))
}

func render(e pen.Event) string {
	var b strings.Builder
	switch e := e.(type) {
	case pen.Planned:
		for _, write := range e.Changes.Writes {
			fmt.Fprintf(&b, "  ~ %s\n", write.Path)
		}
		for _, p := range e.Changes.Deletes {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
		for _, upload := range e.Changes.Uploads {
			fmt.Fprintf(&b, "  ~ %s (binary)\n", upload.Path)
		}
	case pen.Built:
		renderBuild(&b, e.Build)
	case pen.Downloaded:
		fmt.Fprintf(&b, "  ↓ %s\n", e.Path)
	case pen.Created:
		fmt.Fprintf(&b, "created %s\n", e.URLs.Editor)
	case pen.Connected:
		fmt.Fprintf(&b, "connected %s\n", e.URLs.Editor)
	}
	return b.String()
}

func renderBuild(b *strings.Builder, build codepen.Build) {
	for _, patch := range build.AppliedPatches {
		fmt.Fprintf(b, "  patched %s %s\n", patch.Type, patch.Path)
	}
	for _, e := range build.Errors {
		line := ""
		if e.Range != nil && e.Range.Start.Line > 0 {
			line = fmt.Sprintf(":%d", e.Range.Start.Line)
		}
		fmt.Fprintf(b, "  ✗ %s%s %s\n", e.Path, line, e.Message)
	}
	fmt.Fprintf(b, "  build %s\n", build.Status)
}
