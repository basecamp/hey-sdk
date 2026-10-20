package hey

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Resilience errors for circuit breaker, bulkhead, and rate limiting.
var (
	// ErrCircuitOpen is returned when the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrBulkheadFull is returned when the bulkhead has no available slots.
	ErrBulkheadFull = errors.New("bulkhead is full")
	// ErrRateLimited is returned when the rate limiter rejects a request.
	ErrRateLimited = errors.New("rate limit exceeded")
)

// ErrResponseTooLarge is the error a response body ends with once it passes the bound it
// reads under: a JSON or HTML body, success or error alike, at the client's configured
// MaxResponseBodyBytes (HTTPOptions, WithMaxResponseBodyBytes), and a buffered blob, export,
// form or multipart answer at the 50 MiB MaxResponseBodyBytes constant. It reaches the
// caller wrapped, so test for it with errors.Is. An error response refused this way still
// carries its status: the error is the *Error CheckResponse builds for the status, with the
// refusal as its Cause, so errors.As finds the HTTPStatus and errors.Is finds this. The
// refusal reaches whoever reads the body. The generated parsers read every body, so a
// service method answers with it; the raw Get and GetHTML answer an error status without
// reading its body, so there the status error stands alone and the oversized body is simply
// never buffered — which is what the bound is for.
var ErrResponseTooLarge = errors.New("response body exceeded the size limit")

// Error codes for API responses.
const (
	CodeUsage      = "usage"
	CodeNotFound   = "not_found"
	CodeAuth       = "auth_required"
	CodeForbidden  = "forbidden"
	CodeRateLimit  = "rate_limit"
	CodeNetwork    = "network"
	CodeAPI        = "api_error"
	CodeValidation = "validation"
	CodeAmbiguous  = "ambiguous"
	CodeConflict   = "conflict"
)

// Exit codes for CLI tools.
const (
	ExitOK         = 0 // Success
	ExitUsage      = 1 // Invalid arguments or flags
	ExitNotFound   = 2 // Resource not found
	ExitAuth       = 3 // Not authenticated
	ExitForbidden  = 4 // Access denied (scope issue)
	ExitRateLimit  = 5 // Rate limited (429)
	ExitNetwork    = 6 // Connection/DNS/timeout error
	ExitAPI        = 7 // Server returned error
	ExitAmbiguous  = 8 // Multiple matches for name
	ExitValidation = 9 // Validation error (422)
)

// Error is a structured error with code, message, and optional hint.
type Error struct {
	Code       string
	Message    string
	Hint       string
	HTTPStatus int
	Retryable  bool
	RequestID  string
	Cause      error

	// fieldErrors is what FieldErrors answers. A pointer keeps Error comparable, as it
	// was before the reasons were kept.
	fieldErrors *[]FieldError
}

// FieldError is one reason HEY gave for refusing what was sent (HTTP 422): the field it
// objected to, when HEY named one, and what it said. HEY answers a 422 with
// {"errors": ["Subject can't be blank", ...]}, the model's own full messages, which name
// no field apart from the message, so Field is empty for those; an entry that is an
// object, {"field": "subject", "message": "can't be blank"} ("attribute" is read as
// well), carries both.
type FieldError struct {
	Field   string
	Message string
}

// FieldErrors returns each reason HEY gave for a 422, in the order it gave them: the
// entries of the body's errors list, or the body's single error or message when the list
// is absent or gives no usable reason (so {"errors":[],"message":"Invalid"} gives one).
// Message joins the same reasons for display. It is nil for any other error,
// and for a 422 that carried no reason.
func (e *Error) FieldErrors() []FieldError {
	if e.fieldErrors == nil {
		return nil
	}
	return *e.fieldErrors
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Hint != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.Hint)
	}
	return e.Message
}

// Unwrap returns the underlying cause for errors.Is/As support.
func (e *Error) Unwrap() error {
	return e.Cause
}

// ExitCode returns the appropriate exit code for this error.
func (e *Error) ExitCode() int {
	return ExitCodeFor(e.Code)
}

// ExitCodeFor returns the exit code for a given error code.
func ExitCodeFor(code string) int {
	switch code {
	case CodeUsage:
		return ExitUsage
	case CodeNotFound:
		return ExitNotFound
	case CodeAuth:
		return ExitAuth
	case CodeForbidden:
		return ExitForbidden
	case CodeRateLimit:
		return ExitRateLimit
	case CodeNetwork:
		return ExitNetwork
	case CodeAPI:
		return ExitAPI
	case CodeValidation:
		return ExitValidation
	case CodeAmbiguous:
		return ExitAmbiguous
	case CodeConflict:
		return ExitValidation
	default:
		return ExitAPI
	}
}

// ErrUsage creates a usage error.
func ErrUsage(msg string) *Error {
	return &Error{Code: CodeUsage, Message: msg}
}

// ErrUsageHint creates a usage error with a hint.
func ErrUsageHint(msg, hint string) *Error {
	return &Error{Code: CodeUsage, Message: msg, Hint: hint}
}

// ErrNotFound creates a not-found error.
func ErrNotFound(resource, identifier string) *Error {
	return &Error{
		Code:    CodeNotFound,
		Message: fmt.Sprintf("%s not found: %s", resource, identifier),
	}
}

// ErrNotFoundHint creates a not-found error with a hint.
func ErrNotFoundHint(resource, identifier, hint string) *Error {
	return &Error{
		Code:    CodeNotFound,
		Message: fmt.Sprintf("%s not found: %s", resource, identifier),
		Hint:    hint,
	}
}

// ErrAuth creates an authentication error.
func ErrAuth(msg string) *Error {
	return &Error{
		Code:    CodeAuth,
		Message: msg,
	}
}

// ErrForbidden creates a forbidden error.
func ErrForbidden(msg string) *Error {
	return &Error{
		Code:       CodeForbidden,
		Message:    msg,
		HTTPStatus: 403,
	}
}

// ErrForbiddenScope creates a forbidden error due to insufficient scope.
func ErrForbiddenScope() *Error {
	return &Error{
		Code:       CodeForbidden,
		Message:    "Access denied: insufficient scope",
		Hint:       "Re-authenticate with full scope",
		HTTPStatus: 403,
	}
}

// ErrRateLimit creates a rate-limit error.
func ErrRateLimit(retryAfter int) *Error {
	hint := "Try again later"
	if retryAfter > 0 {
		hint = fmt.Sprintf("Try again in %d seconds", retryAfter)
	}
	return &Error{
		Code:       CodeRateLimit,
		Message:    "Rate limited",
		Hint:       hint,
		HTTPStatus: 429,
		Retryable:  true,
	}
}

// ErrNetwork creates a network error from a transport failure. The cause's rendering
// becomes the hint, and net/http's *url.Error carries the whole request URL, so the
// cause is redacted first, with every URL it carries treated as one that can be
// signed; the SDK's own request paths use networkError, which knows the API origin.
func ErrNetwork(cause error) *Error {
	return networkError(cause, "")
}

// networkError creates a network error from a transport failure on a request the SDK
// issued, apiOrigin being its API origin: a URL there carries no credential — the token
// rides in the Authorization header — so beneath it the transport's cause is kept as
// the failure's diagnostic, while beneath a projected URL anywhere else only the
// failure's classification survives (see redactTransportError).
func networkError(cause error, apiOrigin string) *Error {
	cause = redactTransportError(cause, apiOrigin)
	return &Error{
		Code:      CodeNetwork,
		Message:   "Network error",
		Hint:      cause.Error(),
		Retryable: true,
		Cause:     cause,
	}
}

// ErrAPI creates an API error with an HTTP status code.
func ErrAPI(status int, msg string) *Error {
	return &Error{
		Code:       CodeAPI,
		Message:    msg,
		HTTPStatus: status,
	}
}

// ErrConflict signals that the request conflicts with current server state
// (HTTP 409), carrying the server's own message.
func ErrConflict(msg string) *Error {
	return &Error{
		Code:       CodeConflict,
		Message:    msg,
		HTTPStatus: 409,
	}
}

// ErrValidation signals that the server rejected the contents of the request (HTTP 422),
// carrying the messages the model itself produced.
// Each message is also one of its FieldErrors, with no field named.
func ErrValidation(messages ...string) *Error {
	reasons := make([]FieldError, 0, len(messages))
	for _, message := range messages {
		reasons = append(reasons, FieldError{Message: message})
	}
	return validationError(reasons)
}

// validationError is the 422 error for the reasons HEY gave: Message joins them, and
// FieldErrors keeps each one.
func validationError(reasons []FieldError) *Error {
	e := &Error{
		Code:       CodeValidation,
		Message:    "validation error",
		HTTPStatus: 422,
	}
	if len(reasons) > 0 {
		parts := make([]string, 0, len(reasons))
		for _, reason := range reasons {
			parts = append(parts, reason.String())
		}
		e.Message = strings.Join(parts, "; ")
		e.fieldErrors = &reasons
	}
	return e
}

// String renders the reason as it reads in Error.Message: the message alone, or the field
// and the message when HEY named one.
func (f FieldError) String() string {
	if f.Field == "" {
		return f.Message
	}
	return f.Field + ": " + f.Message
}

// validationFromBody reads the reasons out of a 422's body, as validationError takes them.
// A body that is not a JSON object, or carries none, is a 422 with no reason. Entries
// that are neither a string nor an object with a message are skipped; one malformed entry
// does not cost the caller the rest.
func validationFromBody(body []byte) *Error {
	var parsed struct {
		Errors  json.RawMessage `json:"errors"`
		Error   json.RawMessage `json:"error"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return validationError(nil)
	}
	// An errors member that is not a list reads as no list, leaving error and message.
	var entries []json.RawMessage
	_ = json.Unmarshal(parsed.Errors, &entries)
	var reasons []FieldError
	for _, entry := range entries {
		if reason, ok := fieldErrorFrom(entry); ok {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		for _, single := range []json.RawMessage{parsed.Error, parsed.Message} {
			var message string
			if json.Unmarshal(single, &message) == nil && strings.TrimSpace(message) != "" {
				reasons = []FieldError{{Message: message}}
				break
			}
		}
	}
	e := validationError(reasons)
	e.Message = truncateString(e.Message, MaxErrorMessageBytes)
	return e
}

// fieldErrorFrom reads one entry of a 422's errors list: a message, or an object naming
// the field and the message.
func fieldErrorFrom(entry json.RawMessage) (FieldError, bool) {
	var message string
	if json.Unmarshal(entry, &message) == nil {
		return FieldError{Message: message}, strings.TrimSpace(message) != ""
	}
	var object struct {
		Field     string `json:"field"`
		Attribute string `json:"attribute"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(entry, &object) != nil || strings.TrimSpace(object.Message) == "" {
		return FieldError{}, false
	}
	field := object.Field
	if field == "" {
		field = object.Attribute
	}
	return FieldError{Field: field, Message: object.Message}, true
}

// ErrAmbiguous creates an ambiguous match error.
func ErrAmbiguous(resource string, matches []string) *Error {
	hint := "Be more specific"
	if len(matches) > 0 && len(matches) <= 5 {
		hint = fmt.Sprintf("Did you mean: %v", matches)
	}
	return &Error{
		Code:    CodeAmbiguous,
		Message: fmt.Sprintf("Ambiguous %s", resource),
		Hint:    hint,
	}
}

// AsError attempts to convert an error to an *Error.
// If the error is not an *Error, it wraps it in one.
func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	err = redactTransportError(err, "")
	return &Error{
		Code:    CodeAPI,
		Message: err.Error(),
		Cause:   err,
	}
}
