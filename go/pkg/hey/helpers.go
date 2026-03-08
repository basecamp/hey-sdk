package hey

import (
	"fmt"
	"net/http"
)

// CheckResponse converts HTTP response errors to SDK errors for non-2xx responses.
// It is exported for use by conformance testing and consumers using the raw generated client.
func CheckResponse(resp *http.Response) error {
	if resp == nil {
		return nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	requestID := resp.Header.Get("X-Request-Id")

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &Error{Code: CodeAuth, Message: "authentication required", HTTPStatus: 401, RequestID: requestID}
	case http.StatusForbidden:
		return &Error{Code: CodeForbidden, Message: "access denied", HTTPStatus: 403, RequestID: requestID}
	case http.StatusNotFound:
		return &Error{Code: CodeNotFound, Message: "resource not found", HTTPStatus: 404, RequestID: requestID}
	case http.StatusUnprocessableEntity:
		return &Error{Code: CodeValidation, Message: "validation error", HTTPStatus: 422, RequestID: requestID}
	case http.StatusTooManyRequests:
		return &Error{Code: CodeRateLimit, Message: "rate limited - try again later", HTTPStatus: 429, Retryable: true, RequestID: requestID}
	default:
		retryable := resp.StatusCode >= 500 && resp.StatusCode < 600
		return &Error{Code: CodeAPI, Message: fmt.Sprintf("API error: %s", resp.Status), HTTPStatus: resp.StatusCode, Retryable: retryable, RequestID: requestID}
	}
}

// checkResponseEmptyOn converts HTTP response errors to SDK errors, but returns
// nil for the specified status codes (treating them as "empty" rather than error).
// Used for operations like GetOngoingTimeTrack where 404 means "no active track"
// rather than "resource not found" (ADR-004).
func checkResponseEmptyOn(resp *http.Response, emptyOnCodes []int) error {
	if resp == nil {
		return nil
	}
	for _, code := range emptyOnCodes {
		if resp.StatusCode == code {
			return nil
		}
	}
	return CheckResponse(resp)
}

// ListMeta contains pagination metadata from list operations.
type ListMeta struct {
	// TotalCount is the total number of items available (from X-Total-Count header).
	// Zero if the header was not present or could not be parsed.
	TotalCount int
}
