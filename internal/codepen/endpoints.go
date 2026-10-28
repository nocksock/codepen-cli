package codepen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

var lastLink = regexp.MustCompile(`<([^>]+)>;\s*rel="last"`)

// Pen returns a Pen's metadata.
func (c *Client) Pen(ctx context.Context, id string) (Pen, error) {
	var pen Pen
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id), nil, &pen)
	return pen, err
}

// Files returns the files of a Pen's head.
func (c *Client) Files(ctx context.Context, id string) ([]File, error) {
	files := []File{}
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id)+"/files", nil, &files)
	return files, err
}

// Version returns a version with its files.
func (c *Client) Version(ctx context.Context, id string) (Version, error) {
	var version Version
	_, err := c.call(ctx, http.MethodGet, "/versions/"+url.PathEscape(id), nil, &version)
	return version, err
}

// Build returns a build.
func (c *Client) Build(ctx context.Context, id string) (Build, error) {
	var build Build
	_, err := c.call(ctx, http.MethodGet, "/builds/"+url.PathEscape(id), nil, &build)
	return build, err
}

// CountVersions returns how many versions a Pen has, read from the rel="last"
// page link of a one-per-page listing.
func (c *Client) CountVersions(ctx context.Context, penID string) (int, error) {
	items := []json.RawMessage{}
	header, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(penID)+"/versions?per_page=1", nil, &items)
	if err != nil {
		return 0, err
	}
	return total(header.Get("Link"), len(items)), nil
}

func total(link string, items int) int {
	match := lastLink.FindStringSubmatch(link)
	if match == nil {
		return items
	}
	last, err := url.Parse(match[1])
	if err != nil {
		return items
	}
	page, err := strconv.Atoi(last.Query().Get("page"))
	if err != nil {
		return items
	}
	return page
}

// NewPen is what CreatePen sends.
type NewPen struct {
	Title   string
	Changes ChangeSet
	Starter bool // ask for CodePen's starter files instead of Changes
}

// CreatePen creates a Pen.
func (c *Client) CreatePen(ctx context.Context, pen NewPen) (Created, error) {
	// Leaving out the list gives the starter files; anything else sends a
	// (possibly empty) list, as the Node client did.
	var files *[]change
	if !pen.Starter {
		list := pen.Changes.wire()
		files = &list
	}
	body := struct {
		Title string    `json:"title"`
		Files *[]change `json:"files,omitempty"`
	}{Title: pen.Title, Files: files}
	data, err := encode("pen", body, "", pen.Changes.Uploads)
	if err != nil {
		return Created{}, err
	}
	var created Created
	_, err = c.call(ctx, http.MethodPost, "/pens"+noWait, data, &created)
	return created, err
}

// noWait makes a write answer once its files are saved. The API would otherwise
// wait up to 45s for the build, longer than a request may take; Settle polls it.
const noWait = "?wait=0"

// WriteFiles applies changes to a Pen as one new version and returns its build.
// With expected set, the API refuses (ErrConflict) if the head is another version.
func (c *Client) WriteFiles(ctx context.Context, id string, changes ChangeSet, expected string) (Build, error) {
	body := struct {
		Files    []change `json:"files"`
		Expected string   `json:"expected_version_id,omitempty"`
	}{Files: changes.wire(), Expected: expected}
	data, err := encode("changes", body, expected, changes.Uploads)
	if err != nil {
		return Build{}, err
	}
	var written struct {
		Build Build `json:"build"`
	}
	_, err = c.call(ctx, http.MethodPut, "/pens/"+url.PathEscape(id)+"/files"+noWait, data, &written)
	return written.Build, err
}

// Download opens the content of a binary file. The caller closes it. The key
// is not sent: file URLs may point at another host. Only http(s) URLs are
// fetched, and reading fails past the size limit (see WithMaxDownload) or when
// no bytes arrive for a while (see WithDownloadIdle). Errors name the URL
// without query or userinfo, which may carry signed tokens.
func (c *Client) Download(ctx context.Context, fileURL string) (io.ReadCloser, error) {
	shown := redact(fileURL)
	if u, err := url.Parse(fileURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("downloading %q: not an http(s) URL", shown)
	}
	stalled := fmt.Errorf("downloading %q: stalled, nothing received for %s", shown, c.idle)
	ctx, cancel := context.WithCancelCause(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		cancel(nil)
		return nil, fmt.Errorf("downloading %q: %w", shown, withoutURL(err))
	}
	resp, err := c.files.Do(req)
	if err != nil {
		cancel(nil)
		return nil, fmt.Errorf("downloading %q: %w", shown, withoutURL(err))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		cancel(nil)
		return nil, fmt.Errorf("downloading %q: %s", shown, resp.Status)
	}
	tooLarge := fmt.Errorf("downloading %q: larger than %d bytes", shown, c.maxSize)
	if resp.ContentLength > c.maxSize {
		resp.Body.Close()
		cancel(nil)
		return nil, tooLarge
	}
	body := &capped{body: resp.Body, left: c.maxSize, err: tooLarge}
	return watchIdle(ctx, body, c.idle, func() { cancel(stalled) }, func() { cancel(nil) }), nil
}

// idleBody is a body that calls stall when a Read waits longer than idle, and
// done once closed. After a stall, reads fail with the cause of ctx.
type idleBody struct {
	ctx   context.Context
	body  io.ReadCloser
	idle  time.Duration
	timer *time.Timer
	done  func()
}

func watchIdle(ctx context.Context, body io.ReadCloser, idle time.Duration, stall, done func()) *idleBody {
	return &idleBody{ctx: ctx, body: body, idle: idle, timer: time.AfterFunc(idle, stall), done: done}
}

func (r *idleBody) Read(p []byte) (int, error) {
	r.timer.Reset(r.idle)
	n, err := r.body.Read(p)
	r.timer.Stop()
	if err != nil && r.ctx.Err() != nil {
		return n, context.Cause(r.ctx)
	}
	return n, err
}

func (r *idleBody) Close() error {
	r.timer.Stop()
	err := r.body.Close()
	r.done()
	return err
}

// capped reads body until more than left bytes arrive, then fails with err
// instead of quietly cutting the file short.
type capped struct {
	body io.ReadCloser
	left int64
	err  error
}

func (r *capped) Read(p []byte) (int, error) {
	if r.left < 0 {
		return 0, r.err
	}
	// One byte more than allowed tells a file of exactly left bytes from a larger one.
	if int64(len(p)) > r.left+1 {
		p = p[:r.left+1]
	}
	n, err := r.body.Read(p)
	r.left -= int64(n)
	if r.left < 0 {
		return 0, r.err
	}
	return n, err
}

func (r *capped) Close() error { return r.body.Close() }

// redact keeps the scheme, host and path of a URL.
func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(unparsable url)"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// withoutURL drops the full URL that net/http puts into its errors.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// Settle follows a build until it is done, including through next_id when the
// API cancelled it to apply the compiler's patches.
func (c *Client) Settle(ctx context.Context, build Build) (Build, error) {
	for {
		var next string
		switch {
		case build.Status == BuildQueued || build.Status == BuildRunning:
			next = build.ID
		case build.Status == BuildCanceled && build.NextID != "":
			next = build.NextID
		default:
			return build, nil
		}
		if err := sleep(ctx, c.poll); err != nil {
			return build, fmt.Errorf("waiting for build %q: %w", build.ID, err)
		}
		polled, err := c.Build(ctx, next)
		if err != nil {
			return build, err
		}
		build = polled
	}
}
