package codepen

import (
	"errors"
	"net/http"
	"testing"
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
