package codepen_test

import (
	"context"
	"errors"
	"fmt"
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
			return newClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"code":"UNAUTHENTICATED","message":"bad key"}`))
			})
		}},
		{name: "unreadable answer", client: func(t *testing.T) *codepen.Client {
			return newClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`not json`))
			})
		}},
		{name: "network error", client: func(t *testing.T) *codepen.Client {
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
		name     string
		statuses []int // replies in order; the last one repeats
		call     func(context.Context, *codepen.Client) error
		attempts int32
		isError  bool
	}{
		{name: "get retries 503", statuses: []int{503, 503, 200}, call: getPen, attempts: 3},
		{name: "get gives up after 4 retries", statuses: []int{503}, call: getPen, attempts: 5, isError: true},
		{name: "get does not retry 404", statuses: []int{404}, call: getPen, attempts: 1, isError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				n := int(attempts.Add(1)) - 1
				status := tt.statuses[min(n, len(tt.statuses)-1)]
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

// failingTransport fails like a dropped connection, echoing the request URL
// the way net/http's own errors do.
type failingTransport struct{}

func (failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("dial %s: connection refused", req.URL)
}
