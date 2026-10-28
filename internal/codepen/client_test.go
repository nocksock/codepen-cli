package codepen_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

const testKey = "secret-key"

func newClient(t *testing.T, handler http.HandlerFunc) *codepen.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return codepen.New(testKey,
		codepen.WithBaseURL(srv.URL),
		codepen.WithBackoff(time.Millisecond),
		codepen.WithPollInterval(time.Millisecond),
	)
}

func TestClient_Pen(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/pens/p1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"id":"p1","title":"Demo","urls":{"editor":"https://e"},"saved_version":{"id":"v1"}}`))
	})
	pen, err := api.Pen(t.Context(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if pen.Title != "Demo" || pen.URLs.Editor != "https://e" || pen.SavedVersion.ID != "v1" {
		t.Errorf("Pen() = %+v", pen)
	}
}

func TestAPIError(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"code":"CONFLICT","message":"the Pen's head moved"}`))
	})
	_, err := api.Pen(t.Context(), "p1")
	if !errors.Is(err, codepen.ErrConflict) {
		t.Errorf("errors.Is(err, ErrConflict) = false for %v", err)
	}
	if errors.Is(err, codepen.ErrNotFound) {
		t.Errorf("a conflict matched ErrNotFound")
	}
	if got := err.Error(); got != "409 CONFLICT: the Pen's head moved" {
		t.Errorf("Error() = %q", got)
	}
}

func TestAPIError_WithoutJSONBody(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gateway", http.StatusBadGateway)
	})
	_, err := api.Pen(context.Background(), "p1")
	var apiErr *codepen.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway {
		t.Errorf("err = %v, want APIError 502", err)
	}
}

func TestClient_ErrorsNeverContainTheKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		client func(t *testing.T) *codepen.Client
	}{
		{name: "api error", client: func(t *testing.T) *codepen.Client {
			t.Helper()
			return newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":"UNAUTHENTICATED","message":"bad key"}`))
			})
		}},
		{name: "unreadable answer", client: func(t *testing.T) *codepen.Client {
			t.Helper()
			return newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`not json`))
			})
		}},
		{name: "network error", client: func(*testing.T) *codepen.Client {
			return codepen.New(testKey,
				codepen.WithHTTPClient(&http.Client{Transport: failingTransport{}}),
				codepen.WithBackoff(time.Millisecond),
			)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.client(t).Pen(t.Context(), "p1")
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), testKey) {
				t.Errorf("error leaks the API key: %q", err)
			}
		})
	}
}

func TestClient_Retries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		statuses   []int // replies in order; the last one repeats
		retryAfter string
		call       func(context.Context, *codepen.Client) error
		attempts   int32
		isError    bool
	}{
		{name: "get retries 503", statuses: []int{503, 503, 200}, call: getPen, attempts: 3},
		{name: "get gives up after 4 retries", statuses: []int{503}, call: getPen, attempts: 5, isError: true},
		{name: "get does not retry 404", statuses: []int{404}, call: getPen, attempts: 1, isError: true},
		{name: "put retries 429", statuses: []int{429, 200}, call: putFiles, attempts: 2},
		{name: "put does not retry 503", statuses: []int{503}, call: putFiles, attempts: 1, isError: true},
		{name: "a long retry-after is not waited for", statuses: []int{429}, retryAfter: "3600", call: getPen, attempts: 1, isError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				n := int(attempts.Add(1)) - 1
				status := tt.statuses[min(n, len(tt.statuses)-1)]
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(status)
				w.Write([]byte(`{"build":{"id":"b1"}}`))
			})
			err := tt.call(t.Context(), api)
			if (err != nil) != tt.isError {
				t.Errorf("err = %v, want error: %v", err, tt.isError)
			}
			if got := attempts.Load(); got != tt.attempts {
				t.Errorf("attempts = %d, want %d", got, tt.attempts)
			}
		})
	}
}

func TestClient_StopsRetryingWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if _, err := api.Pen(ctx, "p1"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func getPen(ctx context.Context, api *codepen.Client) error {
	_, err := api.Pen(ctx, "p1")
	return err
}

func putFiles(ctx context.Context, api *codepen.Client) error {
	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}}
	_, err := api.WriteFiles(ctx, "p1", changes, "")
	return err
}

// failingTransport fails like a dropped connection, echoing the request URL
// the way net/http's own errors do.
type failingTransport struct{}

func (failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("dial %s: connection refused", req.URL)
}

func TestClient_WriteFiles_JSON(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/pens/p1/files" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if wait := r.URL.Query().Get("wait"); wait != "0" {
			t.Errorf("wait = %q, want 0 so the answer can't outlast the request timeout", wait)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		expected := `{"files":[{"path":"/a.js","content":"1"},{"path":"/b.js","delete":true}],"expected_version_id":"v1"}`
		if string(body) != expected {
			t.Errorf("body = %s\nwant %s", body, expected)
		}
		w.Write([]byte(`{"build":{"id":"b1","status":"Queued"}}`))
	})
	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}, Deletes: []string{"/b.js"}}
	build, err := api.WriteFiles(t.Context(), "p1", changes, "v1")
	if err != nil || build.ID != "b1" {
		t.Errorf("WriteFiles() = %+v, %v", build, err)
	}
}

func TestClient_WriteFiles_Multipart(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		var changes struct {
			Files             []map[string]any `json:"files"`
			ExpectedVersionID string           `json:"expected_version_id"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("changes")), &changes); err != nil {
			t.Errorf("changes part: %v", err)
		}
		if len(changes.Files) != 1 || changes.ExpectedVersionID != "v1" {
			t.Errorf("changes = %+v", changes)
		}
		if wait := r.URL.Query().Get("wait"); wait != "0" {
			t.Errorf("wait = %q, want 0", wait)
		}
		if got := r.FormValue("expected_version_id"); got != "v1" {
			t.Errorf("expected_version_id part = %q", got)
		}
		file, header, err := r.FormFile("/img/dot.png")
		if err != nil {
			t.Fatalf("file part: %v", err)
		}
		data, _ := io.ReadAll(file)
		if string(data) != "\x89PNG" || header.Filename != "dot.png" {
			t.Errorf("file part = %q (%s)", data, header.Filename)
		}
		w.Write([]byte(`{"build":{"id":"b1"}}`))
	})
	changes := codepen.ChangeSet{
		Writes:  []codepen.Write{{Path: "/a.js", Content: "1"}},
		Uploads: []codepen.Upload{{Path: "/img/dot.png", Data: []byte("\x89PNG")}},
	}
	if _, err := api.WriteFiles(t.Context(), "p1", changes, "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestClient_CreatePen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pen      codepen.NewPen
		expected string
	}{
		{name: "with files", pen: codepen.NewPen{Title: "work", Changes: codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}}}, expected: `{"title":"work","files":[{"path":"/a.js","content":"1"}]}`},
		{name: "no changes send an empty list", pen: codepen.NewPen{Title: "work"}, expected: `{"title":"work","files":[]}`},
		{name: "starter files", pen: codepen.NewPen{Title: "work", Starter: true}, expected: `{"title":"work"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || r.URL.Path != "/pens" || string(body) != tt.expected {
					t.Errorf("%s %s %s\nwant %s", r.Method, r.URL.Path, body, tt.expected)
				}
				if wait := r.URL.Query().Get("wait"); wait != "0" {
					t.Errorf("wait = %q, want 0 so the answer can't outlast the request timeout", wait)
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id":"p1","urls":{"editor":"https://e"},"build":{"id":"b1"}}`))
			})
			created, err := api.CreatePen(t.Context(), tt.pen)
			if err != nil || created.ID != "p1" || created.Build.ID != "b1" || created.URLs.Editor != "https://e" {
				t.Errorf("CreatePen() = %+v, %v", created, err)
			}
		})
	}
}

func TestClient_Settle(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/builds/b1":
			w.Write([]byte(`{"id":"b1","status":"Canceled","next_id":"b2"}`))
		case "/builds/b2":
			w.Write([]byte(`{"id":"b2","status":"Success","version_id":"v2"}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	build, err := api.Settle(t.Context(), codepen.Build{ID: "b1", Status: codepen.BuildQueued})
	if err != nil || build.ID != "b2" || build.Status != codepen.BuildSuccess {
		t.Errorf("Settle() = %+v, %v", build, err)
	}
}

func TestClient_Settle_DoneBuildMakesNoRequest(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})
	if _, err := api.Settle(t.Context(), codepen.Build{ID: "b1", Status: codepen.BuildSuccess}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_CountVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		link     string
		expected int
	}{
		{name: "from the last page link", link: `<https://api/pens/p1/versions?page=7&per_page=1>; rel="last"`, expected: 7},
		{name: "without a link", link: "", expected: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/pens/p1/versions" || r.URL.Query().Get("per_page") != "1" {
					t.Errorf("%s?%s", r.URL.Path, r.URL.RawQuery)
				}
				if tt.link != "" {
					w.Header().Set("Link", tt.link)
				}
				w.Write([]byte(`[{"id":"v1"}]`))
			})
			got, err := api.CountVersions(t.Context(), "p1")
			if err != nil || got != tt.expected {
				t.Errorf("CountVersions() = %d, %v; want %d", got, err, tt.expected)
			}
		})
	}
}

func TestClient_Download(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("download sent Authorization %q to the file host", auth)
		}
		w.Write([]byte("bytes"))
	}))
	t.Cleanup(srv.Close)
	api := codepen.New("secret-key")
	body, err := api.Download(t.Context(), srv.URL+"/raw/p1/f1")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "bytes" {
		t.Errorf("Download() = %q", data)
	}
}

func TestClient_Download_Status(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err := codepen.New("k").Download(t.Context(), srv.URL+"/raw/x")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want a 404 error", err)
	}
}

func TestClient_Download_SizeLimit(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("chunked") {
			w.(http.Flusher).Flush() // no Content-Length: the limit applies while reading
		}
		w.Write([]byte(strings.TrimPrefix(r.URL.Path, "/")))
	}))
	t.Cleanup(srv.Close)
	api := codepen.New("k", codepen.WithMaxDownload(4))
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "at the limit", url: srv.URL + "/1234"},
		{name: "over the limit", url: srv.URL + "/12345", wantErr: true},
		{name: "chunked at the limit", url: srv.URL + "/1234?chunked"},
		{name: "chunked over the limit", url: srv.URL + "/12345?chunked", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body, err := api.Download(t.Context(), tt.url)
			if err == nil {
				defer body.Close()
				_, err = io.ReadAll(body)
			}
			if (err != nil) != tt.wantErr || (err != nil && !strings.Contains(err.Error(), "larger than 4 bytes")) {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_Download_OnlyHTTP(t *testing.T) {
	t.Parallel()
	for _, u := range []string{"file:///etc/passwd", "ftp://host/x", "/raw/x"} {
		_, err := codepen.New("k").Download(t.Context(), u)
		if err == nil || !strings.Contains(err.Error(), "not an http(s) URL") {
			t.Errorf("Download(%q) err = %v", u, err)
		}
	}
}

func TestClient_CreatePen_OnlyUploadsSendsEmptyFileList(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		if got := r.FormValue("pen"); got != `{"title":"work","files":[]}` {
			t.Errorf("pen part = %s", got)
		}
		w.Write([]byte(`{"id":"p1"}`))
	})
	changes := codepen.ChangeSet{Uploads: []codepen.Upload{{Path: "/dot.png", Data: []byte("\x89PNG")}}}
	if _, err := api.CreatePen(t.Context(), codepen.NewPen{Title: "work", Changes: changes}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Settle_Cancelled(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"id":"b1","status":"Running"}`))
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := api.Settle(ctx, codepen.Build{ID: "b1", Status: codepen.BuildQueued})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestClient_Download_ErrorsRedactTheURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	tests := []struct {
		name string
		url  string
	}{
		{name: "bad status", url: srv.URL + "/raw/x?token=secret"},
		{name: "network error", url: closedURL + "/raw/x?token=secret"},
		{name: "userinfo", url: strings.Replace(srv.URL, "http://", "http://user:pass@", 1) + "/raw/x?token=secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := codepen.New("k").Download(t.Context(), tt.url)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, leaked := range []string{"token", "secret", "pass"} {
				if strings.Contains(err.Error(), leaked) {
					t.Errorf("error leaks %q: %v", leaked, err)
				}
			}
		})
	}
}

func TestClient_Settle_KeepsLastBuildOnError(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	build, err := api.Settle(t.Context(), codepen.Build{ID: "b1", Status: codepen.BuildQueued})
	if err == nil || build.ID != "b1" {
		t.Errorf("Settle() = %+v, %v; want the last build and an error", build, err)
	}
}
