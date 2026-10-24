// Package diff compares a local Snapshot with a Pen's files and turns the
// difference into the change set a push sends.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// ByPath indexes files by their path.
func ByPath(files []codepen.File) map[string]codepen.File {
	byPath := make(map[string]codepen.File, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}
	return byPath
}

// IsSame reports whether local holds remote's content: text is compared as is,
// binary files by sha256.
func IsSame(local []byte, remote codepen.File) bool {
	if remote.Content != nil {
		return string(local) == *remote.Content
	}
	sum := sha256.Sum256(local)
	return hex.EncodeToString(sum[:]) == remote.ContentHash
}

// Between lists what a push must send so remote matches local: changed and new
// text files as Writes, binary ones as Uploads, and remote-only paths as
// Deletes, except ignored paths, which are never local. Empty local files are skipped, since the API refuses empty content.
func Between(local tree.Snapshot, remote map[string]codepen.File) codepen.ChangeSet {
	changes := codepen.ChangeSet{Writes: []codepen.Write{}, Deletes: []string{}, Uploads: []codepen.Upload{}}
	for _, p := range slices.Sorted(maps.Keys(local)) {
		data := local[p]
		file, exists := remote[p]
		if len(data) == 0 || (exists && IsSame(data, file)) {
			continue
		}
		if tree.IsText(data) {
			changes.Writes = append(changes.Writes, codepen.Write{Path: p, Content: string(data)})
		} else {
			changes.Uploads = append(changes.Uploads, codepen.Upload{Path: p, Data: data})
		}
	}
	for _, p := range slices.Sorted(maps.Keys(remote)) {
		if _, exists := local[p]; !exists && !tree.IsIgnored(p) {
			changes.Deletes = append(changes.Deletes, p)
		}
	}
	return changes
}
