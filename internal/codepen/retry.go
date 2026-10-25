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

// delay is how long to wait before retry number attempt+1.
func (c *Client) delay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
			return time.Duration(min(seconds, int(maxRetryAfter/time.Second))) * time.Second
		}
	}
	return c.backoff << attempt
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
