package hey

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// brokenSuccessServer answers every request with the status, promises a 1000-byte body,
// sends the start of one and then breaks the connection, as a proxy that drops a
// connection mid-answer does. It counts the requests it sees.
func brokenSuccessServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"id":2201,"topic_id":`)
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // closes the connection mid-body
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func brokenSuccessClient(t *testing.T, status int, opts ...ClientOption) (*Client, *atomic.Int32) {
	t.Helper()
	server, requests := brokenSuccessServer(t, status)
	opts = append([]ClientOption{WithMaxRetries(2), WithBaseDelay(time.Millisecond), WithMaxJitter(time.Millisecond)}, opts...)
	return NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"}, opts...), requests
}

// A mutation HEY has answered with a success status has taken effect, so a body lost after
// the status must not fail the call: a caller told a send failed would send it again.
func TestRawMutationsReportASuccessWhoseBodyBreaksMidStreamAsASuccess(t *testing.T) {
	body := map[string]any{"subject": "Lunch on Friday"}
	for _, call := range []struct {
		name string
		do   func(*Client) (*Response, error)
	}{
		{"Post", func(c *Client) (*Response, error) { return c.Post(context.Background(), "/messages.json", body) }},
		{"PostMutation", func(c *Client) (*Response, error) {
			return c.PostMutation(context.Background(), "/messages.json", body)
		}},
		{"Put", func(c *Client) (*Response, error) { return c.Put(context.Background(), "/messages/1.json", body) }},
		{"Patch", func(c *Client) (*Response, error) { return c.Patch(context.Background(), "/messages/1.json", body) }},
		{"Delete", func(c *Client) (*Response, error) { return c.Delete(context.Background(), "/messages/1.json") }},
	} {
		for _, status := range []int{http.StatusOK, http.StatusCreated, http.StatusAccepted} {
			t.Run(call.name+"/"+http.StatusText(status), func(t *testing.T) {
				client, requests := brokenSuccessClient(t, status)

				resp, err := call.do(client)
				if err != nil {
					t.Fatalf("a mutation HEY answered %d must not fail over its body, got %v", status, err)
				}
				if resp.StatusCode != status || resp.Data != nil {
					t.Errorf("resp = %d with data %q, want %d with no data", resp.StatusCode, resp.Data, status)
				}
				var readErr *BodyReadError
				if !errors.As(resp.BodyErr, &readErr) || readErr.StatusCode != status {
					t.Fatalf("BodyErr = %#v, want a *BodyReadError for HTTP %d", resp.BodyErr, status)
				}
				var sdkErr *Error
				if errors.As(resp.BodyErr, &sdkErr) && sdkErr.Retryable {
					t.Errorf("BodyErr is retryable; a write that took effect must not be resent")
				}
				var into map[string]any
				if err := resp.UnmarshalData(&into); !errors.Is(err, resp.BodyErr) {
					t.Errorf("UnmarshalData = %v, want the body-read error", err)
				}
				if n := requests.Load(); n != 1 {
					t.Errorf("requests = %d, want 1: a mutation that took effect is never resent", n)
				}
			})
		}
	}
}

// A body past the client's limit is refused as it is read, after the status said the
// mutation took effect, so it costs the body rather than the call.
func TestRawMutationsReportASuccessPastTheBodyLimitAsASuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":2201,"topic_id":880,"notice":"Message sent"}`)
	}))
	t.Cleanup(server.Close)
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "test-token"},
		WithMaxRetries(0), WithMaxResponseBodyBytes(10))

	resp, err := client.Post(context.Background(), "/messages.json", map[string]any{"subject": "Lunch"})
	if err != nil {
		t.Fatalf("a delivered mutation must not fail over its body's size, got %v", err)
	}
	if !errors.Is(resp.BodyErr, ErrResponseTooLarge) {
		t.Errorf("BodyErr = %v, want one that wraps ErrResponseTooLarge", resp.BodyErr)
	}
}

// A GET changes nothing, so a GET whose body breaks is the error it always was: the caller
// can simply fetch again, and an empty success would read as an empty resource.
func TestRawGetStillFailsWhenItsBodyBreaksMidStream(t *testing.T) {
	client, _ := brokenSuccessClient(t, http.StatusOK, WithMaxRetries(0))

	resp, err := client.Get(context.Background(), "/topics/1.json")
	if err == nil || resp != nil {
		t.Fatalf("resp = %+v, err = %v; want an error for a GET whose body broke", resp, err)
	}
}

// The caller's own cancellation stays its error, so a cancelled send is never reported
// delivered.
func TestRawMutationKeepsTheCallersCancellationDuringTheBody(t *testing.T) {
	headersSent := make(chan struct{})
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":2201`)
		w.(http.Flusher).Flush()
		close(headersSent)
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-headersSent
		cancel()
	}()
	resp, err := client.Post(ctx, "/messages.json", map[string]any{"subject": "Lunch"})
	if resp != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("resp = %+v, err = %v; want the caller's cancellation", resp, err)
	}
}

// The form helpers deliver too: a 2xx whose body breaks is a success with the read failure
// attached.
func TestRawFormMutationsReportASuccessWhoseBodyBreaksMidStreamAsASuccess(t *testing.T) {
	for _, call := range []struct {
		name string
		do   func(*Client) (*FormResponse, error)
	}{
		{"PostForm", func(c *Client) (*FormResponse, error) {
			return c.PostForm(context.Background(), "/entries.json", url.Values{"subject": {"Lunch"}})
		}},
		{"PostMultipart", func(c *Client) (*FormResponse, error) {
			return c.PostMultipart(context.Background(), "/attachments.json", "multipart/form-data; boundary=x", []byte("--x--"))
		}},
	} {
		t.Run(call.name, func(t *testing.T) {
			client, requests := brokenSuccessClient(t, http.StatusOK)

			resp, err := call.do(client)
			if err != nil {
				t.Fatalf("a form mutation HEY answered 200 must not fail over its body, got %v", err)
			}
			var readErr *BodyReadError
			if resp.Body != "" || !errors.As(resp.BodyErr, &readErr) || readErr.StatusCode != http.StatusOK {
				t.Fatalf("resp = %+v, want an empty body and a *BodyReadError", resp)
			}
			if e := AsError(resp.BodyErr); e != nil && e.Retryable {
				t.Errorf("BodyErr is retryable; a write that took effect must not be resent")
			}
			if n := requests.Load(); n != 1 {
				t.Errorf("requests = %d, want 1", n)
			}
		})
	}
}

// A write HEY answers 202 has worked as much as one it answers 200, whatever its body.
func TestRawMutationReadsA202AsTheSuccessItIs(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":2201}`)
	})

	resp, err := client.Post(context.Background(), "/messages.json", map[string]any{"subject": "Lunch"})
	if err != nil || resp.StatusCode != http.StatusAccepted || string(resp.Data) != `{"id":2201}` || resp.BodyErr != nil {
		t.Fatalf("resp = %+v, err = %v; want the 202 and its body", resp, err)
	}
}
