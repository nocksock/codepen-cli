// Package penconfig keeps the connected Pen's id in .codepen.config.json, at
// the top of the mirrored directory. The file is local: tree ignores it, so it
// is never pushed and a pull can't replace it.
package penconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/tree"
)

// Name is the config file's name inside the mirrored directory.
const Name = ".codepen.config.json"

const idKey = "id"

// Read returns the Pen id stored in root, or "" when there is none.
func Read(root string) (string, error) {
	data, err := read(root)
	if err != nil {
		return "", err
	}
	return ID(data), nil
}

// Save stores id in root's config, keeping any other fields there.
func Save(root, id string) error {
	data, err := read(root)
	if err != nil {
		return err
	}
	withID, err := WithID(data, id)
	if err != nil {
		return fmt.Errorf("%s: %w", Name, err)
	}
	return tree.WriteLocal(root, "/"+Name, bytes.NewReader(withID))
}

func read(root string) ([]byte, error) {
	file := filepath.Join(root, Name)
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", file, err)
	}
	return data, nil
}

// ID returns the Pen id stored in data, or "" when there is none.
func ID(data []byte) string {
	fields, err := decode(data)
	if err != nil {
		return ""
	}
	var id string
	if json.Unmarshal(fields[idKey], &id) != nil {
		return ""
	}
	return id
}

// WithID returns data with its id set to id. Empty data becomes a new object.
func WithID(data []byte, id string) ([]byte, error) {
	fields, err := decode(data)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	fields[idKey] = value
	return encode(fields)
}

// decode reads a JSON object, keeping every value as raw JSON so numbers and
// nested structures survive untouched. Empty data is an empty object.
func decode(data []byte) (map[string]json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) == 0 {
		return fields, nil
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

// encode writes fields with sorted keys, two-space indents and a final newline.
func encode(fields map[string]json.RawMessage) ([]byte, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
