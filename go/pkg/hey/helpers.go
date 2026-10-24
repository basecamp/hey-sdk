package hey

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CheckResponse converts HTTP response errors to SDK errors for non-2xx responses.
// It is exported for use by conformance testing and consumers using the raw generated client.
//
// A 422's error carries the reasons HEY gave (Error.FieldErrors), read from the body. Any
// other error status's body is read for an RFC 6749 refusal, which HEY's token endpoint
// answers with: its error_description becomes the message and its error the OAuthCode, while
// the error's code, status and retryability stay the status's. Either way the body is left
// for the caller to read again.
//
// Converting the response is all this does. Of what the model says about an operation, the
// generated client honours that a 401 on an unsigned one is its answer, and that a
// no-redirect one follows no redirect when its Doer is an *http.Client; leaving an unsigned
// operation's credentials off is a request editor's to do, and only the Client's own does.
func CheckResponse(resp *http.Response) error {
	if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
		e := validationFromBody(peekErrorBody(resp))
		e.RequestID = resp.Header.Get("X-Request-Id")
		return e
	}
	err := statusError(resp)
	if e, ok := err.(*Error); ok {
		describeRefusal(e, peekErrorBody(resp))
	}
	return err
}

// describeRefusal gives e the words of an RFC 6749 refusal in body,
// {"error": "<code>", "error_description": "<sentence>"}: the description as its message
// and the code as its OAuthCode. A body without a description to show is no such refusal —
// HEY's other errors send {"error": ...}, {"errors": [...]} or {"message": ...} — and leaves
// e as the status made it.
func describeRefusal(e *Error, body []byte) {
	var refusal struct {
		Error       json.RawMessage `json:"error"`
		Description json.RawMessage `json:"error_description"`
	}
	var description string
	if json.Unmarshal(body, &refusal) != nil || json.Unmarshal(refusal.Description, &description) != nil ||
		strings.TrimSpace(description) == "" {
		return
	}
	e.Message = truncateString(description, MaxErrorMessageBytes)
	_ = json.Unmarshal(refusal.Error, &e.OAuthCode)
}

// statusError is the error CheckResponse builds from the status and headers alone, without
// reading the body: the transport's body cap names it for a refused error body
// (bodyLimitTransport) before anything has read that body.
func statusError(resp *http.Response) error {
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
		// A read-only OAuth token can GET but not mutate: tell the caller why,
		// the same way the raw request path does (see doRequest).
		if resp.Request != nil && resp.Request.Method != http.MethodGet {
			e := ErrForbiddenScope()
			e.RequestID = requestID
			return e
		}
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

// peekErrorBody reads up to MaxErrorBodyBytes of an error response's body and puts what it
// read back in front of the rest, so a caller that reads the body after CheckResponse
// still reads all of it. The generated parsers hand the body back the same way once they
// have read it. A body that will not read is no body.
func peekErrorBody(resp *http.Response) []byte {
	if resp.Body == nil || resp.Body == http.NoBody {
		return nil
	}
	read, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodyBytes))
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(read), resp.Body), Closer: resp.Body}
	return read
}

// readCloser reads from one reader and closes another: a body put back together in front
// of the one it was read from still closes that one.
type readCloser struct {
	io.Reader
	io.Closer
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

// instrument runs a service call inside the gating and hook lifecycle every operation shares,
// so wrappers only have to say what they do.
func (c *Client) instrument(ctx context.Context, op OperationInfo, fn func(context.Context) error) (err error) {
	if gater, ok := c.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return err
		}
	}

	start := time.Now()
	ctx = c.hooks.OnOperationStart(ctx, op)
	defer func() { c.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	return fn(ctx)
}

// gen returns the shared generated client, initializing it on first use.
func (c *Client) genClient() *generated.ClientWithResponses {
	c.initGeneratedClient()
	return c.gen
}

// joinIDs renders ids for endpoints that take posting_ids as a comma-joined query parameter.
func joinIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}
