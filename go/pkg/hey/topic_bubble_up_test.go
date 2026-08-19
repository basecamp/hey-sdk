package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTopicsService_ScheduleBubbleUp(t *testing.T) {
	tests := []struct {
		name          string
		waitingOn     bool
		wantWaitingOn string
	}{
		{name: "custom date"},
		{name: "waiting on someone", waitingOn: true, wantWaitingOn: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRequestTestClient(t, http.MethodPost, "/topics/%s/bubble_up", func(t *testing.T, r *http.Request) {
				t.Helper()
				if got := r.URL.Query().Get("slot"); got != "custom" {
					t.Errorf("slot = %q, want custom", got)
				}
				if got := r.URL.Query().Get("waiting_on"); got != tt.wantWaitingOn {
					t.Errorf("waiting_on = %q, want %q", got, tt.wantWaitingOn)
				}
				if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
					t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
				}
				if got := r.Header.Get("Accept"); got != "*/*" {
					t.Errorf("Accept = %q, want */*", got)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}
				if got := r.PostForm.Get("date"); got != "2026-08-20" {
					t.Errorf("date = %q, want 2026-08-20", got)
				}
			}, http.StatusNoContent, "")

			if err := client.Topics().ScheduleBubbleUp(context.Background(), 123, "2026-08-20", tt.waitingOn); err != nil {
				t.Fatalf("ScheduleBubbleUp: %v", err)
			}
		})
	}
}

func TestTopicsService_CancelAndBubbleUpNow(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		call   func(*Client) error
	}{
		{
			name:   "cancel",
			method: http.MethodDelete,
			path:   "/topics/%s/bubble_up",
			call: func(client *Client) error {
				return client.Topics().CancelBubbleUp(context.Background(), 123)
			},
		},
		{
			name:   "now",
			method: http.MethodPost,
			path:   "/topics/%s/bubble_up_now",
			call: func(client *Client) error {
				return client.Topics().BubbleUpNow(context.Background(), 123)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newRequestTestClient(t, tt.method, tt.path, nil, http.StatusNoContent, "")
			if err := tt.call(client); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
		})
	}
}

func TestTopicsService_BubbleUpRejectsInvalidInputBeforeRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0),
	)
	tests := []struct {
		name string
		call func() error
		want string
	}{
		{name: "schedule topic ID", call: func() error {
			return client.Topics().ScheduleBubbleUp(context.Background(), 0, "2026-08-20", false)
		}, want: "topic ID must be positive"},
		{name: "schedule date", call: func() error {
			return client.Topics().ScheduleBubbleUp(context.Background(), 123, "August 20", false)
		}, want: "YYYY-MM-DD"},
		{name: "cancel topic ID", call: func() error {
			return client.Topics().CancelBubbleUp(context.Background(), 0)
		}, want: "topic ID must be positive"},
		{name: "now topic ID", call: func() error {
			return client.Topics().BubbleUpNow(context.Background(), 0)
		}, want: "topic ID must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if AsError(err).Code != CodeUsage {
				t.Errorf("error code = %q, want %q", AsError(err).Code, CodeUsage)
			}
		})
	}

	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
}
