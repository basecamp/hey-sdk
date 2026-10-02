package generated

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// parseUpdateClearance parses a JSON-typed answer; the Parse function closes the body.
func parseUpdateClearance(status int, body string) (*UpdateClearanceResponse, error) {
	return ParseUpdateClearanceResponse(&http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	})
}

// HEY refuses some requests with an empty body under a JSON content type (a CSRF 403, for
// one). The status is the answer; the body that fails to decode must not replace it.
func TestParseErrorStatusWithUndecodableBodyKeepsTheStatus(t *testing.T) {
	for _, body := range []string{"", "<html>Forbidden</html>"} {
		resp, err := parseUpdateClearance(http.StatusForbidden, body)
		if err != nil {
			t.Fatalf("body %q: expected the response, got %v", body, err)
		}
		if resp.StatusCode() != http.StatusForbidden || resp.JSON403 != nil || string(resp.Body) != body {
			t.Errorf("body %q: got status %d, JSON403 %+v, body %q", body, resp.StatusCode(), resp.JSON403, resp.Body)
		}
	}
}

func TestParseErrorStatusWithJSONBodyStillDecodes(t *testing.T) {
	resp, err := parseUpdateClearance(http.StatusForbidden, `{"message":"Not yours"}`)
	if err != nil {
		t.Fatal(err)
	}
	if resp.JSON403 == nil || resp.JSON403.Message != "Not yours" {
		t.Errorf("expected the decoded 403 body, got %+v", resp.JSON403)
	}
}

func TestParseSuccessWithUndecodableBodyIsStillAnError(t *testing.T) {
	if _, err := parseUpdateClearance(http.StatusOK, ""); err == nil {
		t.Error("expected a decode error for an empty 200")
	}
}
