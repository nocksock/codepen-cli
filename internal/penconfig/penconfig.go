// Package penconfig keeps the connected Pen's id in .codepen/pen.config.json.
//
// That file is also a file of the Pen: CodePen's compiler writes it and it is
// pushed and pulled like any other. The "id" field is local only, so it never
// goes up, a pull can't replace it, and a forked Pen can't bring someone
// else's id along. Shared turns the file into what both sides compare and
// upload; WithID puts the local id back.
package penconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Path is the config file's path inside a Pen.
const Path = "/.codepen/pen.config.json"

const idKey = "id"

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
		return nil, fmt.Errorf("%s: %w", Path, err)
	}
	value, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	fields[idKey] = value
	return encode(fields)
}

// Shared returns the part of data that belongs to the Pen: everything but the
// id, in one canonical formatting so both sides compare equal. isKept is false
// when nothing but the id is left. Data that isn't a JSON object is returned
// unchanged.
func Shared(data []byte) (shared []byte, isKept bool) {
	fields, err := decode(data)
	if err != nil {
		return data, true
	}
	delete(fields, idKey)
	if len(fields) == 0 {
		return nil, false
	}
	shared, err = encode(fields)
	if err != nil {
		return data, true
	}
	return shared, true
}

// Local returns a copy of a local snapshot with the config file reduced to
// its shared part.
func Local(local tree.Snapshot) tree.Snapshot {
	data, ok := local[Path]
	if !ok {
		return local
	}
	out := maps.Clone(local)
	if shared, isKept := Shared(data); isKept {
		out[Path] = shared
	} else {
		delete(out, Path)
	}
	return out
}

// Remote returns a copy of the Pen's files with the config file reduced to its
// shared part.
func Remote(files []codepen.File) []codepen.File {
	out := make([]codepen.File, 0, len(files))
	for _, f := range files {
		if f.Path != Path || f.Content == nil {
			out = append(out, f)
			continue
		}
		shared, isKept := Shared([]byte(*f.Content))
		if !isKept {
			continue
		}
		content := string(shared)
		f.Content = &content
		out = append(out, f)
	}
	return out
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
