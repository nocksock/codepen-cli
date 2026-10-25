package codepen

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError is an error answer from the API.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Sentinels for errors.Is; they match any APIError with the same Code.
var (
	ErrConflict        = &APIError{Code: "CONFLICT"}
	ErrNotFound        = &APIError{Code: "NOT_FOUND"}
	ErrUnauthenticated = &APIError{Code: "UNAUTHENTICATED"}
)

func (e *APIError) Error() string {
	message := e.Message
	if message == "" {
		message = "request failed"
	}
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, message)
}

// Is matches sentinels by Code.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && t.Code != "" && t.Code == e.Code
}

func decodeError(resp *http.Response) error {
	apiErr := &APIError{Status: resp.StatusCode}
	// A body that isn't JSON still leaves a useful error with the status alone.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(apiErr)
	return apiErr
}
