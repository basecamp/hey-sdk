package hey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestTopicsService_Rename(t *testing.T) {
	for _, name := range []string{"Kitchen renovation", "", "Renovation: \"café\" & garden"} {
		t.Run(name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPatch || r.URL.Path != "/topics/42.json" {
					t.Errorf("request = %s %s, want PATCH /topics/42.json", r.Method, r.URL.Path)
				}
				if r.Header.Get("Accept") != "application/json" {
					t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
				}
				var body map[string]map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode body: %v", err)
				}
				want := map[string]map[string]string{"topic": {"name": name}}
				if !reflect.DeepEqual(body, want) {
					t.Errorf("body = %#v, want %#v", body, want)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(srv.Close)
			client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

			if err := client.Topics().Rename(context.Background(), 42, name); err != nil {
				t.Fatalf("rename: %v", err)
			}
			if requests != 1 {
				t.Errorf("requests = %d, want one write with no read-back", requests)
			}
		})
	}
}

func TestTopicsService_RenameRejectsUnacknowledgedSuccess(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "rename-unacknowledged")
				w.WriteHeader(status)
			}))
			t.Cleanup(srv.Close)
			client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

			err := client.Topics().Rename(context.Background(), 42, "Kitchen renovation")
			var sdkError *Error
			if !errors.As(err, &sdkError) || sdkError.Code != CodeAPI || sdkError.HTTPStatus != status || sdkError.Retryable {
				t.Fatalf("error = %v, want non-retryable API error with status %d", err, status)
			}
			if sdkError.RequestID != "rename-unacknowledged" {
				t.Errorf("request ID = %q", sdkError.RequestID)
			}
		})
	}
}

func TestTopicsService_RenameMergedTopicDoesNotReportAReadAsAWrite(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/topics/42.json":
			http.Redirect(w, r, "/topics/43", http.StatusFound)
		case "/topics/43":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":43,"name":"Original name"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(3))

	err := client.Topics().Rename(context.Background(), 42, "Kitchen renovation")
	var sdkError *Error
	if !errors.As(err, &sdkError) || sdkError.Code != CodeAPI || sdkError.HTTPStatus != http.StatusOK || sdkError.Retryable {
		t.Fatalf("error = %v, want a non-retryable API error for the redirected read", err)
	}
	if want := []string{"PATCH /topics/42.json", "GET /topics/43"}; !reflect.DeepEqual(requests, want) {
		t.Errorf("requests = %v, want %v", requests, want)
	}
}

func TestTopicsService_RenameErrors(t *testing.T) {
	for _, status := range []int{401, 403, 404, 422, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			t.Cleanup(srv.Close)
			client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(0))

			err := client.Topics().Rename(context.Background(), 42, "Kitchen renovation")
			var sdkError *Error
			if !errors.As(err, &sdkError) || sdkError.HTTPStatus != status {
				t.Fatalf("error = %v, want SDK error with status %d", err, status)
			}
		})
	}
}
