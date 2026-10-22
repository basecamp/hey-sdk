package generated

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
)

var (
	tokenGrant = CreateTokenRequestContent{
		GrantType: "refresh_token", ClientId: "client-1", InstallId: "install-1", RefreshToken: "refresh-1",
	}
	tokenRevocation = RevokeTokenRequestContent{ClientId: "client-1", Token: "token-1"}
)

// The model marks HEY's token endpoint, and nothing else, unsigned and redirect-free, and
// the metadata carries what it marks: an ordinary operation read as unsigned would go out
// without the caller's credentials.
func TestOnlyTheTokenOperationsAreUnsignedOrFollowNoRedirect(t *testing.T) {
	var unsigned, noRedirect []string
	for operation, meta := range operationMetadata {
		if meta.Unsigned {
			unsigned = append(unsigned, operation)
		}
		if meta.NoRedirect {
			noRedirect = append(noRedirect, operation)
		}
	}
	slices.Sort(unsigned)
	slices.Sort(noRedirect)
	want := []string{"CreateToken", "RevokeToken"}
	if !slices.Equal(unsigned, want) || !slices.Equal(noRedirect, want) {
		t.Errorf("unsigned %v, no-redirect %v; want %v for both", unsigned, noRedirect, want)
	}
}

// An unsigned operation's 401 judges what its body carries, not the client's credentials,
// so the AuthRefresher is never asked about it and the 401 is the answer, sent once. Any
// other operation still asks and is sent again.
func TestUnsignedOperationsNeverAskTheAuthRefresher(t *testing.T) {
	for name, send := range map[string]func(*ClientWithResponses) (int, error){
		"CreateToken": func(c *ClientWithResponses) (int, error) {
			resp, err := c.CreateTokenWithResponse(context.Background(), tokenGrant)
			if err != nil {
				return 0, err
			}
			return resp.StatusCode(), nil
		},
		"RevokeToken": func(c *ClientWithResponses) (int, error) {
			resp, err := c.RevokeTokenWithResponse(context.Background(), tokenRevocation)
			if err != nil {
				return 0, err
			}
			return resp.StatusCode(), nil
		},
		"GetIdentity": func(c *ClientWithResponses) (int, error) {
			resp, err := c.GetIdentityWithResponse(context.Background())
			if err != nil {
				return 0, err
			}
			return resp.StatusCode(), nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if requests.Add(1) == 1 {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"a","token_type":"bearer","id":1}`)
			}))
			t.Cleanup(server.Close)
			var asked atomic.Int32
			client, err := NewClientWithResponses(server.URL, WithAuthRefresher(func(context.Context) bool {
				asked.Add(1)
				return true
			}))
			if err != nil {
				t.Fatal(err)
			}

			status, err := send(client)
			if err != nil {
				t.Fatal(err)
			}
			unsigned := operationMetadata[name].Unsigned
			if unsigned && (status != 401 || requests.Load() != 1 || asked.Load() != 0) {
				t.Errorf("%s answered %d after %d sends, refresher asked %d times; want the 401, one send, never asked",
					name, status, requests.Load(), asked.Load())
			}
			if !unsigned && (status != 200 || requests.Load() != 2 || asked.Load() != 1) {
				t.Errorf("%s answered %d after %d sends, refresher asked %d times; want 200 after a refreshed resend",
					name, status, requests.Load(), asked.Load())
			}
		})
	}
}

// watchedBody is a 3xx's body that records whether it was read and closed.
type watchedBody struct {
	reads  int
	closed bool
}

func (b *watchedBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (b *watchedBody) Close() error {
	b.closed = true
	return nil
}

// redirectingDoer answers every request with a 307 whose body it watches, as a Doer that
// follows no redirect hands one back.
type redirectingDoer struct{ body *watchedBody }

func (d redirectingDoer) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusTemporaryRedirect,
		Status:     "307 Temporary Redirect",
		Header:     http.Header{"Location": {"https://elsewhere.example/oauth/tokens.json"}},
		Body:       d.body,
		Request:    req,
	}, nil
}

// A no-redirect operation's 3xx is its answer, and the body that came with it is not
// read: it is closed and the response handed back with its status.
func TestNoRedirectOperationsLeaveA3xxBodyUnread(t *testing.T) {
	for name, send := range map[string]func(*ClientWithResponses) (*http.Response, error){
		"CreateToken": func(c *ClientWithResponses) (*http.Response, error) {
			resp, err := c.CreateTokenWithResponse(context.Background(), tokenGrant)
			if err != nil {
				return nil, err
			}
			return resp.HTTPResponse, nil
		},
		"RevokeToken": func(c *ClientWithResponses) (*http.Response, error) {
			resp, err := c.RevokeTokenWithResponse(context.Background(), tokenRevocation)
			if err != nil {
				return nil, err
			}
			return resp.HTTPResponse, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := &watchedBody{}
			client, err := NewClientWithResponses("https://app.hey.com", WithHTTPClient(redirectingDoer{body}))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := send(client)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusTemporaryRedirect {
				t.Errorf("status = %d, want 307", resp.StatusCode)
			}
			if body.reads != 0 || !body.closed {
				t.Errorf("the 3xx's body was read %d times, closed %v; want unread and closed", body.reads, body.closed)
			}
			if rest, _ := io.ReadAll(resp.Body); len(rest) != 0 {
				t.Errorf("the response handed back a body %q, want none", rest)
			}
		})
	}
}
