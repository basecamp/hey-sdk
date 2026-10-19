package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// The body HEY answers a refused habit with: the model's full messages
// (Calendar::HabitsController renders habit.errors.full_messages).
const habit422Body = `{"errors":["Title can't be blank","Days must include at least one day"]}`

var habit422Reasons = []FieldError{
	{Message: "Title can't be blank"},
	{Message: "Days must include at least one day"},
}

// Error stays comparable: the reasons ride behind a pointer.
var _ = Error{} == Error{}

func newValidationTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-422")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func assertValidationReasons(t *testing.T, err error, message string, reasons []FieldError) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected an *Error, got %#v", err)
	}
	if e.Code != CodeValidation || e.HTTPStatus != 422 || e.ExitCode() != ExitValidation {
		t.Errorf("Code = %q, HTTPStatus = %d, exit %d; want a 422 validation error", e.Code, e.HTTPStatus, e.ExitCode())
	}
	if e.Message != message {
		t.Errorf("Message = %q, want %q", e.Message, message)
	}
	if !reflect.DeepEqual(e.FieldErrors(), reasons) {
		t.Errorf("FieldErrors() = %#v, want %#v", e.FieldErrors(), reasons)
	}
	if e.RequestID != "req-422" {
		t.Errorf("RequestID = %q, want req-422", e.RequestID)
	}
	if e.Retryable {
		t.Error("a validation error is not retryable")
	}
}

// A service method whose wrapper maps the status with CheckResponse, after the generated
// parser has read the body, still answers with HEY's reasons.
func TestValidation_ServiceMethodKeepsTheReasons(t *testing.T) {
	server := newValidationTestServer(t, habit422Body)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	_, err := client.Habits().Create(context.Background(), HabitParams{})

	assertValidationReasons(t, err, "Title can't be blank; Days must include at least one day", habit422Reasons)
}

// The raw helpers answer a 422 as the validation error it is, reasons and all, rather than
// a generic API error.
func TestValidation_RawRequestKeepsTheReasons(t *testing.T) {
	server := newValidationTestServer(t, habit422Body)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))

	_, err := client.Post(context.Background(), "/calendar/habits.json", map[string]any{"habit": map[string]any{}})

	assertValidationReasons(t, err, "Title can't be blank; Days must include at least one day", habit422Reasons)
}

func TestValidation_ReadsEveryShapeOfReason(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		message string
		reasons []FieldError
	}{
		{
			name:    "objects naming the field",
			body:    `{"errors":[{"field":"subject","message":"is too long"},{"attribute":"content","message":"can't be blank"}]}`,
			message: "subject: is too long; content: can't be blank",
			reasons: []FieldError{{Field: "subject", Message: "is too long"}, {Field: "content", Message: "can't be blank"}},
		},
		{
			name:    "malformed entries skipped",
			body:    `{"errors":["Title can't be blank", 7, {"field":"days"}, "", null]}`,
			message: "Title can't be blank",
			reasons: []FieldError{{Message: "Title can't be blank"}},
		},
		{
			name:    "a single error",
			body:    `{"error":"Subject can't be blank"}`,
			message: "Subject can't be blank",
			reasons: []FieldError{{Message: "Subject can't be blank"}},
		},
		{
			name:    "an errors member that is not a list",
			body:    `{"errors":{"subject":["is too long"]},"message":"Invalid message"}`,
			message: "Invalid message",
			reasons: []FieldError{{Message: "Invalid message"}},
		},
		{name: "no body", body: ``, message: "validation error"},
		{name: "an HTML page", body: `<html>Unprocessable</html>`, message: "validation error"},
		{name: "an empty list", body: `{"errors":[]}`, message: "validation error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newValidationTestServer(t, tc.body)
			resp, err := http.Get(server.URL) //nolint:noctx // a test server
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()

			assertValidationReasons(t, CheckResponse(resp), tc.message, tc.reasons)

			// CheckResponse leaves the body for the caller to read.
			left, _ := io.ReadAll(resp.Body)
			if string(left) != tc.body {
				t.Errorf("body after CheckResponse = %q, want %q", left, tc.body)
			}
		})
	}
}

func TestValidation_MessageIsBounded(t *testing.T) {
	long := strings.Repeat("x", 2*MaxErrorMessageBytes)
	server := newValidationTestServer(t, `{"errors":["`+long+`"]}`)
	resp, err := http.Get(server.URL) //nolint:noctx // a test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	e := AsError(CheckResponse(resp))
	if len(e.Message) > MaxErrorMessageBytes {
		t.Errorf("Message is %d bytes, want at most %d", len(e.Message), MaxErrorMessageBytes)
	}
	if got := e.FieldErrors(); len(got) != 1 || got[0].Message != long {
		t.Errorf("FieldErrors() lost the reason: %d entries", len(got))
	}
}

func TestErrValidation_KeepsEachMessageAsAReason(t *testing.T) {
	e := ErrValidation("Content can't be blank")
	if want := []FieldError{{Message: "Content can't be blank"}}; !reflect.DeepEqual(e.FieldErrors(), want) {
		t.Errorf("FieldErrors() = %#v, want %#v", e.FieldErrors(), want)
	}
	if got := ErrValidation().FieldErrors(); got != nil {
		t.Errorf("FieldErrors() with no messages = %#v, want nil", got)
	}
	if got := ErrNotFound("box", "1").FieldErrors(); got != nil {
		t.Errorf("FieldErrors() on a not-found error = %#v, want nil", got)
	}
}
