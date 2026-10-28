package codepen

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryable(t *testing.T) {
	t.Parallel()
	network := errors.New("connection reset")
	tests := []struct {
		name     string
		method   string
		status   int
		err      error
		expected bool
	}{
		{name: "get network error", method: http.MethodGet, err: network, expected: true},
		{name: "get 503", method: http.MethodGet, status: 503, expected: true},
		{name: "get 429", method: http.MethodGet, status: 429, expected: true},
		{name: "get 404", method: http.MethodGet, status: 404, expected: false},
		{name: "get 200", method: http.MethodGet, status: 200, expected: false},
		{name: "put 429", method: http.MethodPut, status: 429, expected: true},
		{name: "put 503 may have applied", method: http.MethodPut, status: 503, expected: false},
		{name: "post network error may have applied", method: http.MethodPost, err: network, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := retryable(tt.method, tt.status, tt.err); got != tt.expected {
				t.Errorf("retryable(%s, %d, %v) = %v, want %v", tt.method, tt.status, tt.err, got, tt.expected)
			}
		})
	}
}

func TestDelay(t *testing.T) {
	t.Parallel()
	c := &Client{backoff: 200 * time.Millisecond}
	withRetryAfter := func(value string) *http.Response {
		return &http.Response{Header: http.Header{"Retry-After": []string{value}}}
	}
	at := func(d time.Duration) string { return time.Now().Add(d).UTC().Format(http.TimeFormat) }
	tests := []struct {
		name     string
		resp     *http.Response
		attempt  int
		expected time.Duration
		isGiveUp bool
	}{
		{name: "first retry uses the base", attempt: 0, expected: 200 * time.Millisecond},
		{name: "backoff doubles", attempt: 3, expected: 1600 * time.Millisecond},
		{name: "retry-after is honoured", resp: withRetryAfter("2"), attempt: 3, expected: 2 * time.Second},
		{name: "retry-after at the cap", resp: withRetryAfter("30"), expected: 30 * time.Second},
		{name: "retry-after past the cap gives up", resp: withRetryAfter("120"), isGiveUp: true},
		{name: "huge retry-after gives up", resp: withRetryAfter("9223372036854775807"), isGiveUp: true},
		{name: "retry-after date past the cap gives up", resp: withRetryAfter(at(time.Hour)), isGiveUp: true},
		{name: "retry-after date in the past", resp: withRetryAfter(at(-time.Hour)), expected: 0},
		{name: "negative retry-after falls back", resp: withRetryAfter("-5"), attempt: 1, expected: 400 * time.Millisecond},
		{name: "non-numeric retry-after falls back", resp: withRetryAfter("soon"), attempt: 1, expected: 400 * time.Millisecond},
		{name: "missing retry-after falls back", resp: &http.Response{Header: http.Header{}}, attempt: 2, expected: 800 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := c.delay(tt.resp, tt.attempt)
			if ok == tt.isGiveUp || (ok && got != tt.expected) {
				t.Errorf("delay(attempt %d) = %v, %v; want %v, give up %v", tt.attempt, got, ok, tt.expected, tt.isGiveUp)
			}
		})
	}
}

// A date a few seconds ahead waits about that long.
func TestDelay_RetryAfterDate(t *testing.T) {
	t.Parallel()
	c := &Client{backoff: 200 * time.Millisecond}
	date := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	got, ok := c.delay(&http.Response{Header: http.Header{"Retry-After": []string{date}}}, 0)
	if !ok || got < 8*time.Second || got > 10*time.Second {
		t.Errorf("delay() = %v, %v; want about 10s", got, ok)
	}
}
