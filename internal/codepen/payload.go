package codepen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"path"
)

// change is one entry of the API's "files" list.
type change struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

// wire lists the text writes, then the deletes, as the API expects them.
func (cs ChangeSet) wire() []change {
	changes := make([]change, 0, len(cs.Writes)+len(cs.Deletes))
	for _, w := range cs.Writes {
		changes = append(changes, change{Path: w.Path, Content: w.Content})
	}
	for _, p := range cs.Deletes {
		changes = append(changes, change{Path: p, Delete: true})
	}
	return changes
}

// encode picks the request encoding: plain JSON when nothing is binary,
// otherwise multipart with the JSON in the part named field, expected (if set)
// in its own part, and one file part per upload, named by its path.
func encode(field string, v any, expected string, uploads []Upload) (*payload, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", field, err)
	}
	if len(uploads) == 0 {
		return &payload{contentType: "application/json", data: data}, nil
	}
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if err := form.WriteField(field, string(data)); err != nil {
		return nil, err
	}
	if expected != "" {
		if err := form.WriteField("expected_version_id", expected); err != nil {
			return nil, err
		}
	}
	for _, upload := range uploads {
		part, err := form.CreateFormFile(upload.Path, path.Base(upload.Path))
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(upload.Data); err != nil {
			return nil, err
		}
	}
	if err := form.Close(); err != nil {
		return nil, err
	}
	return &payload{contentType: form.FormDataContentType(), data: buf.Bytes()}, nil
}
