// Package codepen is a client for the CodePen REST API v2.
// https://blog.codepen.io/docs/api/api-v2/rest-api-v2/
package codepen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://api.codepen.io/v2"

const (
	defaultPollInterval = 2 * time.Second
	defaultBackoff      = 200 * time.Millisecond
	requestTimeout      = 30 * time.Second
)

// Client talks to the API with one key. It is safe for concurrent use.
type Client struct {
	key     string
	baseURL string
	poll    time.Duration
	backoff time.Duration
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another API, such as a fake in tests.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(url, "/") }
}

// WithPollInterval sets how often Settle asks about a running build.
func WithPollInterval(d time.Duration) Option {
	return func(c *Client) { c.poll = d }
}

// WithBackoff sets the delay before the first retry; it doubles with each retry.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// WithHTTPClient replaces the HTTP client (default: 30s timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New returns a client that authenticates with key.
func New(key string, opts ...Option) *Client {
	c := &Client{
		key:     key,
		baseURL: DefaultBaseURL,
		poll:    defaultPollInterval,
		backoff: defaultBackoff,
		http:    &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// payload is an encoded request body, kept as bytes so retries can resend it.
type payload struct {
	contentType string
	data        []byte
}

// call sends one API request (with retries) and decodes the JSON answer into
// out unless out is nil. It returns the response headers.
func (c *Client) call(ctx context.Context, method, path string, body *payload, out any) (http.Header, error) {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.Header, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("%s %s: decoding the answer: %w", method, path, err)
	}
	return resp.Header, nil
}

func (c *Client) send(ctx context.Context, method, path string, body *payload) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.attempt(ctx, method, path, body)
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if attempt == maxRetries || !retryable(method, status, err) {
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
			return resp, nil
		}
		if ctx.Err() != nil {
			if resp != nil {
				drain(resp)
			}
			return nil, ctx.Err()
		}
		wait := c.delay(resp, attempt)
		if resp != nil {
			drain(resp)
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func (c *Client) attempt(ctx context.Context, method, path string, body *payload) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body.data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", body.contentType)
	}
	return c.http.Do(req)
}
