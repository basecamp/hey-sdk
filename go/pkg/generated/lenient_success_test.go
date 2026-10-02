package generated

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// answering serves every request the one answer: a status, a content type and a body, or a
// body cut off mid-read when cut is set.
func answering(t *testing.T, status int, contentType, body string, cut bool) *ClientWithResponses {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cut {
			// Promise more than is sent and hang up, so the body is lost mid-read.
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(buf, "HTTP/1.1 %d OK\r\nContent-Type: %s\r\nContent-Length: 1000\r\n\r\n%s", status, contentType, body)
			_ = buf.Flush()
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	client, err := NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var lunchMessage = CreateMessageRequestContent{
	ActingSenderId: 314,
	Message:        MessagePayload{Subject: "Lunch on Friday", Content: "Are you free at noon?"},
}

// A message HEY answers a success for has gone out, so the operations that deliver one
// (heyLenientSuccess) parse an unreadable 2xx without an error: a caller told the send
// failed would send it again. The typed payload is left nil, as a 204's is.
func TestDeliveringOperationsParseAnUnreadableSuccessWithoutAnError(t *testing.T) {
	ctx := context.Background()
	for _, answer := range []struct {
		name, contentType, body string
		cut                     bool
	}{
		{"not JSON", "application/json", "<html>Message sent</html>", false},
		{"truncated", "application/json", `{"id":2201,"topic_`, false},
		{"the wrong types", "application/json", `{"id":"2201"}`, false},
		{"a body lost mid-read", "application/json", `{"id":2201`, true},
	} {
		t.Run(answer.name, func(t *testing.T) {
			client := answering(t, 200, answer.contentType, answer.body, answer.cut)

			created, err := client.CreateMessageWithResponse(ctx, lunchMessage)
			if err != nil || created.JSON200 != nil {
				t.Errorf("CreateMessage = %+v, %v; want no payload and no error", created, err)
			}
			updated, err := client.UpdateMessageWithResponse(ctx, 2201, lunchMessage)
			if err != nil || updated.JSON200 != nil {
				t.Errorf("UpdateMessage = %+v, %v; want no payload and no error", updated, err)
			}
			replied, err := client.CreateReplyWithResponse(ctx, 1990, CreateReplyRequestContent{
				ActingSenderId: 314, Message: ReplyMessagePayload{Content: "Noon works."},
			})
			if err != nil || replied.JSON200 != nil {
				t.Errorf("CreateReply = %+v, %v; want no payload and no error", replied, err)
			}
		})
	}
}

func TestDeliveringOperationsStillDecodeAReadableSuccess(t *testing.T) {
	client := answering(t, 200, "application/json", `{"id":2201,"topic_id":880,"subject":"Lunch on Friday","delayed":false}`, false)

	created, err := client.CreateMessageWithResponse(context.Background(), lunchMessage)
	if err != nil || created.JSON200 == nil || created.JSON200.TopicId != 880 {
		t.Fatalf("CreateMessage = %+v, %v; want topic 880", created, err)
	}
}

// Only a success is read leniently: an error status whose body does not decode is still
// the error it was — the status answers, as it does for every operation, and nothing is
// decoded out of it — and an operation without the trait still fails on an unreadable
// success.
func TestLenientSuccessStopsAtSuccessAndAtTheOperationsThatDeliver(t *testing.T) {
	ctx := context.Background()
	refused, err := answering(t, 422, "application/json", "not json", false).CreateMessageWithResponse(ctx, lunchMessage)
	if err != nil {
		t.Fatalf("CreateMessage 422 with an unreadable body: %v; want the response with its status", err)
	}
	if refused.StatusCode() != 422 || refused.JSON422 != nil {
		t.Errorf("CreateMessage 422 = status %d, 422 body %+v; want the 422 and nothing decoded", refused.StatusCode(), refused.JSON422)
	}
	if _, err := answering(t, 200, "application/json", "not json", false).GetMessageWithResponse(ctx, 9); err == nil {
		t.Error("GetMessage with an unreadable body parsed without an error")
	}
}

// The caller's own cancellation is not absorbed: a read it cuts short after a 2xx arrived
// is still its error, so a cancelled call is never reported as done.
func TestDeliveringOperationsKeepTheCallersCancellation(t *testing.T) {
	headersSent := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":2201`)
		w.(http.Flusher).Flush()
		close(headersSent)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	client, err := NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-headersSent
		cancel()
	}()
	if _, err := client.CreateMessageWithResponse(ctx, lunchMessage); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the caller's cancellation", err)
	}
}
