package codepen

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries    = 4
	maxRetryAfter = 30 * time.Second
)

// retryable reports whether a failed attempt may be repeated. Reads are retried
// on network errors, 429 and 5xx. Writes only on 429: after a 5xx or a dropped
// connection the write may already have been applied.
func retryable(method string, status int, err error) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if method != http.MethodGet {
		return false
	}
	return err != nil || status >= http.StatusInternalServerError
}

// delay is how long to wait before retry number attempt+1, and false when a
// Retry-After asks for more than maxRetryAfter: a wait that long means a used
// up quota, and retrying would only spend more of it.
func (c *Client) delay(resp *http.Response, attempt int) (time.Duration, bool) {
	if resp != nil {
		if wait, ok := retryAfter(resp.Header.Get("Retry-After")); ok {
			return wait, wait <= maxRetryAfter
		}
	}
	return c.backoff << attempt, true
}

// retryAfter reads a Retry-After value, given in seconds or as an HTTP date.
func retryAfter(value string) (time.Duration, bool) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter + time.Second, true // past the cap, without overflowing
		}
		return time.Duration(seconds) * time.Second, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(time.Until(date), 0), true
	}
	return 0, false
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// drain discards a little of an unwanted body so the connection can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16)) // best effort
	resp.Body.Close()
}
