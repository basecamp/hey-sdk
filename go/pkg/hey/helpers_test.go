package hey

import (
	"net/http"
	"testing"
)

func TestCheckResponse_Success(t *testing.T) {
	for _, status := range []int{200, 201, 204, 299} {
		resp := &http.Response{StatusCode: status, Header: http.Header{}}
		if err := CheckResponse(resp); err != nil {
			t.Fatalf("expected no error for %d, got %v", status, err)
		}
	}
}

func TestCheckResponse_Nil(t *testing.T) {
	if err := CheckResponse(nil); err != nil {
		t.Fatalf("expected nil for nil response, got %v", err)
	}
}

func TestCheckResponse_Errors(t *testing.T) {
	cases := []struct {
		status   int
		wantCode string
	}{
		{401, CodeAuth},
		{403, CodeForbidden},
		{404, CodeNotFound},
		{422, CodeValidation},
		{429, CodeRateLimit},
		{500, CodeAPI},
		{503, CodeAPI},
		{418, CodeAPI},
	}
	for _, tc := range cases {
		resp := &http.Response{
			StatusCode: tc.status,
			Status:     http.StatusText(tc.status),
			Header:     http.Header{},
		}
		resp.Header.Set("X-Request-Id", "req-123")

		err := CheckResponse(resp)
		if err == nil {
			t.Fatalf("expected error for %d", tc.status)
		}
		apiErr, ok := err.(*Error)
		if !ok {
			t.Fatalf("expected *Error, got %T", err)
		}
		if apiErr.Code != tc.wantCode {
			t.Fatalf("status %d: expected code %q, got %q", tc.status, tc.wantCode, apiErr.Code)
		}
		if apiErr.RequestID != "req-123" {
			t.Fatalf("expected request ID 'req-123', got %q", apiErr.RequestID)
		}
	}
}

func TestCheckResponse_429IsRetryable(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{}}
	err := CheckResponse(resp)
	if err == nil {
		t.Fatal("expected error for 429")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if !apiErr.Retryable {
		t.Fatal("expected 429 to be retryable")
	}
}

func TestCheckResponse_5xxIsRetryable(t *testing.T) {
	resp := &http.Response{StatusCode: 502, Status: "Bad Gateway", Header: http.Header{}}
	err := CheckResponse(resp)
	if err == nil {
		t.Fatal("expected error for 502")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if !apiErr.Retryable {
		t.Fatal("expected 5xx to be retryable")
	}
}

func TestCheckResponse_4xxIsNotRetryable(t *testing.T) {
	resp := &http.Response{StatusCode: 400, Status: "Bad Request", Header: http.Header{}}
	err := CheckResponse(resp)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Retryable {
		t.Fatal("expected 4xx (non-429) to not be retryable")
	}
}

func TestCheckResponseEmptyOn(t *testing.T) {
	resp404 := &http.Response{StatusCode: 404, Header: http.Header{}}
	if err := checkResponseEmptyOn(resp404, []int{404}); err != nil {
		t.Fatalf("expected no error for 404 in emptyOn list, got %v", err)
	}

	resp500 := &http.Response{StatusCode: 500, Status: "Internal Server Error", Header: http.Header{}}
	if err := checkResponseEmptyOn(resp500, []int{404}); err == nil {
		t.Fatal("expected error for 500 not in emptyOn list")
	}

	if err := checkResponseEmptyOn(nil, []int{404}); err != nil {
		t.Fatalf("expected nil for nil response, got %v", err)
	}
}
