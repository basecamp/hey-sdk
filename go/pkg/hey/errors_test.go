package hey

import (
	"errors"
	"fmt"
	"testing"
)

func TestError_Error(t *testing.T) {
	e := &Error{Code: CodeAPI, Message: "something broke"}
	if e.Error() != "something broke" {
		t.Fatalf("expected 'something broke', got %q", e.Error())
	}

	e.Hint = "check the logs"
	if e.Error() != "something broke: check the logs" {
		t.Fatalf("expected hint appended, got %q", e.Error())
	}
}

func TestError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	e := &Error{Code: CodeNetwork, Message: "net", Cause: cause}
	if !errors.Is(e, cause) {
		t.Fatal("expected errors.Is to find cause")
	}
}

func TestError_ExitCode(t *testing.T) {
	e := &Error{Code: CodeNotFound, Message: "nope"}
	if e.ExitCode() != ExitNotFound {
		t.Fatalf("expected %d, got %d", ExitNotFound, e.ExitCode())
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{CodeUsage, ExitUsage},
		{CodeNotFound, ExitNotFound},
		{CodeAuth, ExitAuth},
		{CodeForbidden, ExitForbidden},
		{CodeRateLimit, ExitRateLimit},
		{CodeNetwork, ExitNetwork},
		{CodeAPI, ExitAPI},
		{CodeValidation, ExitValidation},
		{CodeAmbiguous, ExitAmbiguous},
		{"unknown", ExitAPI},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := ExitCodeFor(tc.code)
			if got != tc.want {
				t.Fatalf("ExitCodeFor(%q) = %d, want %d", tc.code, got, tc.want)
			}
		})
	}
}

func TestErrUsage(t *testing.T) {
	e := ErrUsage("bad arg")
	if e.Code != CodeUsage {
		t.Fatalf("expected code %q, got %q", CodeUsage, e.Code)
	}
	if e.Message != "bad arg" {
		t.Fatalf("expected message 'bad arg', got %q", e.Message)
	}
}

func TestErrUsageHint(t *testing.T) {
	e := ErrUsageHint("bad arg", "try --help")
	if e.Hint != "try --help" {
		t.Fatalf("expected hint 'try --help', got %q", e.Hint)
	}
}

func TestErrNotFound(t *testing.T) {
	e := ErrNotFound("Topic", "42")
	if e.Code != CodeNotFound {
		t.Fatalf("expected code %q", CodeNotFound)
	}
	if e.Message != "Topic not found: 42" {
		t.Fatalf("unexpected message: %q", e.Message)
	}
}

func TestErrNotFoundHint(t *testing.T) {
	e := ErrNotFoundHint("Topic", "42", "check the ID")
	if e.Hint != "check the ID" {
		t.Fatalf("expected hint, got %q", e.Hint)
	}
}

func TestErrAuth(t *testing.T) {
	e := ErrAuth("not logged in")
	if e.Code != CodeAuth {
		t.Fatalf("expected code %q", CodeAuth)
	}
}

func TestErrForbidden(t *testing.T) {
	e := ErrForbidden("nope")
	if e.HTTPStatus != 403 {
		t.Fatalf("expected HTTP 403, got %d", e.HTTPStatus)
	}
}

func TestErrForbiddenScope(t *testing.T) {
	e := ErrForbiddenScope()
	if e.Code != CodeForbidden {
		t.Fatalf("expected code %q", CodeForbidden)
	}
	if e.Hint == "" {
		t.Fatal("expected hint for scope error")
	}
}

func TestErrRateLimit(t *testing.T) {
	e := ErrRateLimit(0)
	if e.Code != CodeRateLimit {
		t.Fatalf("expected code %q", CodeRateLimit)
	}
	if e.Hint != "Try again later" {
		t.Fatalf("expected generic hint, got %q", e.Hint)
	}
	if !e.Retryable {
		t.Fatal("expected retryable")
	}

	e2 := ErrRateLimit(30)
	if e2.Hint != "Try again in 30 seconds" {
		t.Fatalf("expected specific hint, got %q", e2.Hint)
	}
}

func TestErrNetwork(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	e := ErrNetwork(cause)
	if e.Code != CodeNetwork {
		t.Fatalf("expected code %q", CodeNetwork)
	}
	if !e.Retryable {
		t.Fatal("expected retryable")
	}
	if !errors.Is(e, cause) {
		t.Fatal("expected cause to be unwrappable")
	}
}

func TestErrAPI(t *testing.T) {
	e := ErrAPI(500, "server error")
	if e.HTTPStatus != 500 {
		t.Fatalf("expected HTTP 500, got %d", e.HTTPStatus)
	}
}

func TestErrAmbiguous(t *testing.T) {
	e := ErrAmbiguous("contact", []string{"Alice", "Bob"})
	if e.Code != CodeAmbiguous {
		t.Fatalf("expected code %q", CodeAmbiguous)
	}
	wantHint := "Did you mean: [Alice Bob]"
	if e.Hint != wantHint {
		t.Fatalf("expected hint %q, got %q", wantHint, e.Hint)
	}

	e2 := ErrAmbiguous("contact", nil)
	if e2.Hint != "Be more specific" {
		t.Fatalf("expected generic hint, got %q", e2.Hint)
	}
}

func TestAsError_WithSDKError(t *testing.T) {
	original := ErrNotFound("x", "1")
	result := AsError(original)
	if result != original {
		t.Fatal("expected same pointer")
	}
}

func TestAsError_WithPlainError(t *testing.T) {
	plain := fmt.Errorf("something")
	result := AsError(plain)
	if result.Code != CodeAPI {
		t.Fatalf("expected code %q, got %q", CodeAPI, result.Code)
	}
	if result.Message != "something" {
		t.Fatalf("expected message 'something', got %q", result.Message)
	}
	if !errors.Is(result, plain) {
		t.Fatal("expected cause to be preserved")
	}
}
