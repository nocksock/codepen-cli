package codepen

import "time"

// Build statuses the client acts on.
const (
	BuildQueued   = "Queued"
	BuildRunning  = "Running"
	BuildCanceled = "Canceled"
	BuildSuccess  = "Success"
)

// Pen is a Pen's metadata (GET /pens/{id}).
type Pen struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Tags         []string    `json:"tags"`
	Access       string      `json:"access"`
	Editor       string      `json:"editor"`
	Popularity   float64     `json:"popularity"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	URLs         URLs        `json:"urls"`
	SavedVersion *Version    `json:"saved_version"`
	Deployment   *Deployment `json:"deployment"`
}

// URLs are where a Pen can be opened.
type URLs struct {
	Editor  string `json:"editor"`
	Preview string `json:"preview"`
}

// Deployment is a Pen's live site.
type Deployment struct {
	URL string `json:"url"`
}

// Version is one saved state of a Pen. Summaries leave out Files.
type Version struct {
	ID        string    `json:"id"`
	PenID     string    `json:"pen_id"`
	CreatedAt time.Time `json:"created_at"`
	Files     []File    `json:"files,omitempty"`
	Build     *Build    `json:"build,omitempty"`
}

// Build is the compiler run for a version.
type Build struct {
	ID             string       `json:"id"`
	VersionID      string       `json:"version_id"`
	Status         string       `json:"status"`
	Errors         []BuildError `json:"errors"`
	AppliedPatches []Patch      `json:"applied_patches"`
	NextID         string       `json:"next_id"`
}

// BuildError is a compiler error in one file.
type BuildError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Range   *Range `json:"range,omitempty"`
}

// Range locates a BuildError.
type Range struct {
	Start Position `json:"start"`
}

// Position is a line in a file.
type Position struct {
	Line int `json:"line"`
}

// Patch is a file the compiler created or changed, e.g. .codepen/pen.config.json.
type Patch struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// File is one file of a version. Content is nil for binary files; their
// bytes are at URL and ContentHash is their sha256 in hex.
type File struct {
	ID          string  `json:"id"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	IsBinary    bool    `json:"is_binary"`
	Content     *string `json:"content"`
	ContentHash string  `json:"content_hash"`
	URL         string  `json:"url"`
}

// Created is the answer to CreatePen: the new Pen and its first build.
type Created struct {
	Pen
	Build Build `json:"build"`
}

// ChangeSet is what a write sends: text files, deleted paths and binary uploads.
type ChangeSet struct {
	Writes  []Write
	Deletes []string
	Uploads []Upload
}

// Write sets a text file.
type Write struct {
	Path    string
	Content string
}

// Upload sets a binary file.
type Upload struct {
	Path string
	Data []byte
}

// IsEmpty reports whether cs changes nothing.
func (cs ChangeSet) IsEmpty() bool {
	return len(cs.Writes)+len(cs.Deletes)+len(cs.Uploads) == 0
}
