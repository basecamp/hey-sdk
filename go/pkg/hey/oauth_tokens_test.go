package hey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// tokenRequest is one request the token endpoint was sent.
type tokenRequest struct {
	method, path, rawQuery string
	header                 http.Header
	body                   map[string]any
}

// tokenAnswer is what the token endpoint answers one request with.
type tokenAnswer struct {
	status   int
	body     string
	location string
}

// tokenEndpoint serves the answers in turn, the last one for every request after them,
// and records each request.
func tokenEndpoint(t *testing.T, answers ...tokenAnswer) (*httptest.Server, func() []tokenRequest) {
	t.Helper()
	var mu sync.Mutex
	var requests []tokenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		requests = append(requests, tokenRequest{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone(), body})
		answer := answers[min(len(requests), len(answers))-1]
		mu.Unlock()
		if answer.location != "" {
			w.Header().Set("Location", answer.location)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(answer.status)
		_, _ = io.WriteString(w, answer.body)
	}))
	t.Cleanup(server.Close)
	return server, func() []tokenRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]tokenRequest(nil), requests...)
	}
}

// proofAuth signs as a DPoP-bound strategy does, with a proof beside the Authorization
// header, and renews whenever it is asked. It counts both.
type proofAuth struct {
	signings, refreshes atomic.Int32
}

func (a *proofAuth) Authenticate(_ context.Context, req *http.Request) error {
	a.signings.Add(1)
	req.Header.Set("Authorization", "DPoP held-token")
	req.Header.Set("DPoP", "held-proof")
	return nil
}

func (a *proofAuth) Refresh(context.Context) error {
	a.refreshes.Add(1)
	return nil
}

// heldTokens is a token provider that can renew what it hands out, counting both.
type heldTokens struct {
	asked, refreshes atomic.Int32
}

func (p *heldTokens) AccessToken(context.Context) (string, error) {
	p.asked.Add(1)
	return "held-token", nil
}

func (p *heldTokens) Refresh(context.Context) error {
	p.refreshes.Add(1)
	return nil
}

var (
	codeGrant = generated.CreateTokenRequestContent{
		GrantType:    "authorization_code",
		ClientId:     "client-1",
		InstallId:    "install-1",
		Code:         "code-1",
		RedirectUri:  "heyapp://oauth/callback",
		CodeVerifier: "verifier-1",
	}
	revocation = generated.RevokeTokenRequestContent{ClientId: "client-1", Token: "token-1"}
)

// tokenOperations are the two operations HEY takes unsigned, as a caller sends them.
func tokenOperations(client *Client) map[string]func() error {
	return map[string]func() error{
		"CreateToken": func() error {
			_, err := client.OAuth().CreateToken(context.Background(), codeGrant)
			return err
		},
		"RevokeToken": func() error {
			return client.OAuth().RevokeToken(context.Background(), revocation)
		},
	}
}

const grantedToken = `{"access_token":"access-1","token_type":"bearer","expires_in":1209600,"refresh_token":"refresh-1"}`

// HEY judges the token endpoint's body, not the caller, and the credentials a client
// would sign with may be the very ones being exchanged or revoked: neither operation
// carries them, nor asks the strategy or the provider for them.
func TestOAuthOperationsGoUnsignedWhateverTheClientHolds(t *testing.T) {
	server, requests := tokenEndpoint(t, tokenAnswer{status: 200, body: grantedToken})
	strategy := &proofAuth{}
	provider := &heldTokens{}
	for name, client := range map[string]*Client{
		"a strategy": NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(strategy), WithMaxRetries(0)),
		"a provider": NewClient(&Config{BaseURL: server.URL}, provider, WithMaxRetries(0)),
	} {
		for operation, send := range tokenOperations(client) {
			t.Run(name+"/"+operation, func(t *testing.T) {
				before := len(requests())
				if err := send(); err != nil {
					t.Fatalf("%s: %v", operation, err)
				}
				sent := requests()[before:]
				if len(sent) != 1 {
					t.Fatalf("sent %d requests, want 1", len(sent))
				}
				for _, header := range []string{"Authorization", "DPoP"} {
					if got := sent[0].header.Values(header); len(got) > 0 {
						t.Errorf("%s went out with %s %q", operation, header, got)
					}
				}
				if got := sent[0].header.Get("User-Agent"); got != DefaultUserAgent {
					t.Errorf("User-Agent = %q, want %q", got, DefaultUserAgent)
				}
			})
		}
	}
	if got := strategy.signings.Load(); got != 0 {
		t.Errorf("the strategy was asked to sign %d times, want none", got)
	}
	if got := provider.asked.Load(); got != 0 {
		t.Errorf("the provider was asked for a token %d times, want none", got)
	}
}

// The generated methods send the method, path and JSON body the model documents: the path's
// own .json is not doubled, and the token stays out of the query, where logs would keep it.
func TestOAuthOperationsSendTheirDocumentedRequests(t *testing.T) {
	server, requests := tokenEndpoint(t, tokenAnswer{status: 200, body: grantedToken}, tokenAnswer{status: 200, body: `{}`})
	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "held"}, WithMaxRetries(0))

	token, err := client.OAuth().CreateToken(context.Background(), codeGrant)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	want := generated.OAuthToken{AccessToken: "access-1", TokenType: "bearer", ExpiresIn: 1209600, RefreshToken: "refresh-1"}
	if token == nil || *token != want {
		t.Errorf("CreateToken = %+v, want %+v", token, want)
	}
	if err := client.OAuth().RevokeToken(context.Background(), revocation); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	sent := requests()
	if len(sent) != 2 {
		t.Fatalf("sent %d requests, want 2", len(sent))
	}
	for i, want := range []struct {
		method string
		body   map[string]any
	}{
		{http.MethodPost, map[string]any{
			"grant_type": "authorization_code", "client_id": "client-1", "install_id": "install-1",
			"code": "code-1", "redirect_uri": "heyapp://oauth/callback", "code_verifier": "verifier-1",
		}},
		{http.MethodDelete, map[string]any{"client_id": "client-1", "token": "token-1"}},
	} {
		got := sent[i]
		if got.method != want.method || got.path != "/oauth/tokens.json" || got.rawQuery != "" {
			t.Errorf("request %d = %s %s?%s, want %s /oauth/tokens.json with no query", i, got.method, got.path, got.rawQuery, want.method)
		}
		if ct := got.header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("request %d Content-Type = %q, want application/json", i, ct)
		}
		if !reflect.DeepEqual(got.body, want.body) {
			t.Errorf("request %d body = %v, want %v", i, got.body, want.body)
		}
	}
}

// A 401 from the token endpoint judges the code, the refresh token or the token in the
// body, not credentials the client could renew: it is the answer, sent once, and nothing
// is refreshed — with a strategy or a provider that would renew if asked.
func TestOAuthOperationsAnswerA401WithoutRefreshing(t *testing.T) {
	for _, holder := range []string{"strategy", "provider"} {
		for _, operation := range []string{"CreateToken", "RevokeToken"} {
			t.Run(holder+"/"+operation, func(t *testing.T) {
				server, requests := tokenEndpoint(t,
					tokenAnswer{status: 401, body: `{"error":"invalid_grant","error_description":"Invalid refresh token."}`},
					tokenAnswer{status: 200, body: grantedToken})
				strategy := &proofAuth{}
				provider := &heldTokens{}
				client := NewClient(&Config{BaseURL: server.URL}, provider, WithMaxRetries(3))
				if holder == "strategy" {
					client = NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(strategy), WithMaxRetries(3))
				}

				err := tokenOperations(client)[operation]()
				var apiErr *Error
				if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth || apiErr.HTTPStatus != 401 {
					t.Fatalf("%s = %v, want the 401 as an auth_required error", operation, err)
				}
				if sent := len(requests()); sent != 1 {
					t.Errorf("sent %d requests, want 1", sent)
				}
				if got := strategy.refreshes.Load() + provider.refreshes.Load(); got != 0 {
					t.Errorf("refreshed %d times, want none", got)
				}
			})
		}
	}
}

// The token operations' 401 being the answer is theirs alone: an ordinary operation still
// refreshes on one and is sent again.
func TestSignedOperationsStillRefreshOnA401(t *testing.T) {
	server, requests := tokenEndpoint(t,
		tokenAnswer{status: 401, body: `{}`},
		tokenAnswer{status: 200, body: `{"id":1}`})
	strategy := &proofAuth{}
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(strategy), WithMaxRetries(0))

	if _, err := client.Identity().GetIdentity(context.Background()); err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if got := strategy.refreshes.Load(); got != 1 {
		t.Errorf("refreshed %d times, want 1", got)
	}
	sent := requests()
	if len(sent) != 2 || sent[1].header.Get("DPoP") == "" {
		t.Errorf("sent %d requests, want the 401 and a signed resend", len(sent))
	}
}

// A redirect would carry the code and its verifier, or the token, wherever it points. The
// token endpoint never redirects, so a 3xx is the answer: an error carrying its status,
// with nothing sent to the Location — from the SDK's own HTTP client or one the caller
// supplied, whose redirect policy would follow it.
func TestOAuthOperationsFollowNoRedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, grantedToken)
	}))
	t.Cleanup(other.Close)

	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, location := range []string{other.URL + "/oauth/tokens.json", "/elsewhere/oauth/tokens.json"} {
			for _, supplied := range []bool{false, true} {
				for _, operation := range []string{"CreateToken", "RevokeToken"} {
					name := fmt.Sprintf("%d/%s/supplied=%v/%s", status, location, supplied, operation)
					t.Run(name, func(t *testing.T) {
						server, requests := tokenEndpoint(t,
							tokenAnswer{status: status, location: location},
							tokenAnswer{status: 200, body: grantedToken})
						opts := []ClientOption{WithMaxRetries(3)}
						if supplied {
							opts = append(opts, WithHTTPClient(&http.Client{}))
						}
						client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "held"}, opts...)

						err := tokenOperations(client)[operation]()
						var apiErr *Error
						if !errors.As(err, &apiErr) || apiErr.HTTPStatus != status {
							t.Fatalf("%s = %v, want an error carrying HTTP %d", operation, err, status)
						}
						if sent := requests(); len(sent) != 1 || sent[0].path != "/oauth/tokens.json" {
							t.Errorf("sent %d requests to the endpoint's server, want the one", len(sent))
						}
					})
				}
			}
		}
	}
	if got := elsewhere.Load(); got != 0 {
		t.Errorf("the Location's server was sent %d requests, want none", got)
	}
}

// NoCredentials is the deliberate way to build a client with nothing to sign with: it sends
// the operations HEY takes unsigned, and refuses every other before anything is sent.
func TestNoCredentialsClientSendsOnlyUnsignedOperations(t *testing.T) {
	server, requests := tokenEndpoint(t, tokenAnswer{status: 200, body: grantedToken})
	client := NewClient(&Config{BaseURL: server.URL}, nil, WithAuthStrategy(NoCredentials{}), WithMaxRetries(0))

	for operation, send := range tokenOperations(client) {
		if err := send(); err != nil {
			t.Errorf("%s: %v", operation, err)
		}
	}
	if sent := len(requests()); sent != 2 {
		t.Fatalf("sent %d requests, want the two token requests", sent)
	}

	for name, signed := range map[string]func() error{
		"a generated operation": func() error {
			_, err := client.Identity().GetIdentity(context.Background())
			return err
		},
		"a hand-written request": func() error {
			_, err := client.Get(context.Background(), "/my/navigation.json")
			return err
		},
		"a form request": func() error {
			_, err := client.PostForm(context.Background(), "/topics/1/trash", nil)
			return err
		},
	} {
		err := signed()
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != CodeAuth || !strings.Contains(apiErr.Message, "no credentials") {
			t.Errorf("%s = %v, want auth_required for a client with no credentials", name, err)
		}
	}
	if sent := len(requests()); sent != 2 {
		t.Errorf("sent %d requests, want only the two token requests", sent)
	}
}

// A client given neither a token provider nor a strategy is a forgotten token, refused
// when it is built rather than at its first send.
func TestNewClientRefusesAClientWithNothingToSignWith(t *testing.T) {
	refused := func(build func()) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		build()
		return false
	}
	cfg := &Config{BaseURL: "https://app.hey.com"}
	if !refused(func() { NewClient(cfg, nil) }) {
		t.Error("NewClient with no provider and no strategy was built")
	}
	if refused(func() { NewClient(cfg, nil, WithAuthStrategy(NoCredentials{})) }) {
		t.Error("NewClient refused a client built with NoCredentials")
	}
}

// Account scope is the presentation context of an authenticated identity: an unsigned
// request carries no identity, so a client scoped to one account sends the token
// operations unscoped, and the hooks hear them, a resend included, as they went out.
func TestAccountScopedClientSendsOAuthOperationsUnscoped(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/oauth/tokens.json" {
			_, _ = io.WriteString(w, `{"id":1,"accounts":[{"id":11,"status":"active"}]}`)
			return
		}
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		first := len(queries) == 1
		mu.Unlock()
		// The revocation is sent first and refused once with a status its policy resends.
		if first {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, grantedToken)
	}))
	t.Cleanup(server.Close)
	hooks := &accountScopeRecordingHooks{}
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "held"}, WithMaxRetries(1), WithHooks(hooks))
	scoped := mustForAccount(t, root, 11)

	for _, operation := range []string{"RevokeToken", "CreateToken"} {
		if err := tokenOperations(scoped)[operation](); err != nil {
			t.Fatalf("%s: %v", operation, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 3 || queries[0] != "" || queries[1] != "" || queries[2] != "" {
		t.Errorf("token requests went out with queries %q, want three with none", queries)
	}
	for _, heard := range append(hooks.requestURLs(), hooks.retryURL()) {
		if strings.Contains(heard, "/oauth/") && strings.Contains(heard, filteredAccountIDParameter) {
			t.Errorf("the hooks heard %s, which did not go out", heard)
		}
	}
	if hooks.retryURL() == "" {
		t.Error("the hooks heard no resend")
	}
}
