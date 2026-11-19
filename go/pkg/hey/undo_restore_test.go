package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// redirectingServer answers every request as answer says and counts them, so a test can
// tell a redirect taken as the answer from one that was followed.
func redirectingServer(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) (*Client, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		answer(w, r)
	}))
	t.Cleanup(srv.Close)
	return NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "t"}, WithMaxRetries(3)), &requests
}

// HEY answers an undo with a 303 to the draft. The redirect is the answer: it names the
// entry called back, and following it would only read a page.
func TestTopicsService_UndoSendAnswersTheEntryCalledBack(t *testing.T) {
	for name, location := range map[string]string{
		"a message on a thread of its own": "/messages/77/edit",
		"a reply":                          "/topics/9?expanded_draft=77",
		"an absolute URL":                  "https://app.hey.com/messages/77/edit?native_undo=true",
	} {
		t.Run(name, func(t *testing.T) {
			var method, path, authorization string
			client, requests := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
				method, path, authorization = r.Method, r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusSeeOther)
			})

			entryID, err := client.Topics().UndoSend(context.Background(), 9)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entryID != 77 {
				t.Errorf("expected entry 77 called back, got %d", entryID)
			}
			if method != http.MethodPost || path != "/topics/9/undo_send.json" {
				t.Errorf("expected POST /topics/9/undo_send.json, got %s %s", method, path)
			}
			if authorization != "Bearer t" {
				t.Errorf("expected the undo to be signed, got %q", authorization)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("expected the redirect to be the answer, not followed: %d requests", got)
			}
		})
	}
}

// A redirect that names no entry is still an undo: HEY acted. The caller learns only that
// it can't say which entry it was.
func TestTopicsService_UndoSendAnswersNoEntryWhenTheRedirectNamesNone(t *testing.T) {
	client, _ := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/somewhere/else")
		w.WriteHeader(http.StatusSeeOther)
	})

	entryID, err := client.Topics().UndoSend(context.Background(), 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entryID != 0 {
		t.Errorf("expected no entry named, got %d", entryID)
	}
}

// Once the delay has run out HEY answers 400: too late, told apart from an undo by its
// status, and never asked twice.
func TestTopicsService_UndoSendTooLate(t *testing.T) {
	client, requests := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	entryID, err := client.Topics().UndoSend(context.Background(), 9)
	if err == nil {
		t.Fatal("expected an error once nothing is left to undo")
	}
	if apiErr := AsError(err); apiErr.HTTPStatus != http.StatusBadRequest || apiErr.Code != CodeAPI {
		t.Errorf("expected an API error with status 400, got %+v", apiErr)
	}
	if entryID != 0 {
		t.Errorf("expected no entry, got %d", entryID)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected one request, got %d", got)
	}
}

// A second undo calls back the next delayed entry, so a failure is never sent again,
// whatever the client's retry budget.
func TestTopicsService_UndoSendIsNotResentAfterAFailure(t *testing.T) {
	client, requests := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	if _, err := client.Topics().UndoSend(context.Background(), 9); err == nil {
		t.Fatal("expected the 503 to surface")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected the undo to be sent once, got %d requests", got)
	}
}

func TestUndoneEntryID(t *testing.T) {
	for location, want := range map[string]int64{
		"/messages/77/edit":                                              77,
		"https://app.hey.com/messages/77/edit":                           77,
		"https://app.hey.com/messages/77/edit?native_undo=true&notice=x": 77,
		"/topics/9?expanded_draft=77":                                    77,
		"https://app.hey.com/topics/9?expanded_draft=77#__entry_77":      77,
	} {
		if got, ok := UndoneEntryID(location); !ok || got != want {
			t.Errorf("UndoneEntryID(%q) = %d, %v; want %d", location, got, ok, want)
		}
	}
	for _, location := range []string{
		"",
		"/topics/9", // where HEY's pages send a browser with nothing to undo
		"/topics/9?expanded_draft=",
		"/topics/9?expanded_draft=abc",
		"/topics/9?expanded_draft=-4",
		"/messages/77", // the message, not its draft
		"/messages/abc/edit",
		"/messages/0/edit",
		"/entries/77/edit",
		"/topics/9/entries?expanded_draft=77",
		"not a url at all: %",
	} {
		if got, ok := UndoneEntryID(location); ok {
			t.Errorf("UndoneEntryID(%q) = %d, want no entry", location, got)
		}
	}
}

// HEY answers an entry restore with a 303 to the thread. The redirect is the answer: a
// read of the thread that failed would report a restore that went through as one that
// did not.
func TestEntriesService_RestoreTakesTheRedirectAsTheAnswer(t *testing.T) {
	var method, path string
	client, requests := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		method, path = r.Method, r.URL.Path
		w.Header().Set("Location", "/topics/9")
		w.WriteHeader(http.StatusSeeOther)
	})

	if err := client.Entries().Restore(context.Background(), 77); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if method != http.MethodPut || path != "/entries/77/status/active.json" {
		t.Errorf("expected PUT /entries/77/status/active.json, got %s %s", method, path)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected the redirect to be the answer, not followed: %d requests", got)
	}
}

func TestEntriesService_RestoreSurfacesARefusal(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		client, _ := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})

		err := client.Entries().Restore(context.Background(), 77)
		if err == nil {
			t.Fatalf("expected HTTP %d to surface", status)
		}
		if apiErr := AsError(err); apiErr.HTTPStatus != status {
			t.Errorf("expected status %d on the error, got %+v", status, apiErr)
		}
	}
}

func TestPutFormCapturesTheRedirect(t *testing.T) {
	client, requests := redirectingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected a PUT, got %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Errorf("expected no content type on a PUT without a body, got %q", got)
		}
		w.Header().Set("Location", "/topics/9")
		w.WriteHeader(http.StatusSeeOther)
	})

	resp, err := client.PutForm(context.Background(), "/entries/77/status/active.json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Location != "/topics/9" {
		t.Errorf("expected the 303 and its Location, got %+v", resp)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("expected one request, got %d", got)
	}
}
