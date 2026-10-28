// Package penstate remembers a directory's files as of its last sync, so a
// pull can tell local edits from remote ones. The state lives outside the
// project, under the user's state directory, so it never needs ignoring and
// never travels with a clone.
package penstate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// State is a directory's last sync with a Pen.
type State struct {
	Dir     string            `json:"dir"`
	Pen     string            `json:"pen"`
	Version string            `json:"version"`
	Files   map[string]string `json:"files"` // sha256 by path
}

// Store keeps one State per directory in Dir. With Dir "", nothing is kept.
type Store struct{ Dir string }

// DefaultDir is where states live: $XDG_STATE_HOME/pen, else
// ~/.local/state/pen, or %LOCALAPPDATA%\pen\state on Windows; "" if none of
// those is known.
func DefaultDir(goos string, getenv func(string) (string, bool)) string {
	if dir, _ := getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "pen")
	}
	if goos == "windows" {
		if dir, _ := getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "pen", "state")
		}
		return ""
	}
	if home, _ := getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "pen")
	}
	return ""
}

// Load returns root's state; an empty one when none was saved.
func (s Store) Load(root string) (State, error) {
	if s.Dir == "" {
		return State{}, nil
	}
	data, err := os.ReadFile(s.path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("reading the last sync: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("reading the last sync from %s: %w", s.path(root), err)
	}
	if state.Dir != root {
		return State{}, nil
	}
	return state, nil
}

// Save replaces root's state.
func (s Store) Save(root string, state State) error {
	if s.Dir == "" {
		return nil
	}
	state.Dir = root
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := tree.WriteLocal(s.Dir, "/"+filepath.Base(s.path(root)), bytes.NewReader(data)); err != nil {
		return fmt.Errorf("saving the last sync: %w", err)
	}
	return nil
}

func (s Store) path(root string) string {
	return filepath.Join(s.Dir, Sum([]byte(root))+".json")
}

// Sum is the sha256 of local content.
func Sum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FileSum is the sha256 of a Pen file's content: computed for text, as the API
// reports it for binary files.
func FileSum(file codepen.File) string {
	if file.Content != nil {
		return Sum([]byte(*file.Content))
	}
	return file.ContentHash
}
