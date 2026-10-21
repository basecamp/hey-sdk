package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A token request carries the authorization code with its verifier, or the refresh token, and
// the client secret in its body, which a 307 or 308 keeps. A token endpoint never redirects, so
// the exchanger follows none: the 3xx is an error, and the form goes nowhere else.
func TestTokenRequestsFollowNoRedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"theirs","token_type":"Bearer"}`))
	}))
	defer other.Close()

	var asked atomic.Int32
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Location", other.URL+"/oauth/tokens")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer token.Close()

	requests := map[string]func(*Exchanger) (*Token, error){
		"exchange": func(exchanger *Exchanger) (*Token, error) {
			return exchanger.Exchange(context.Background(), ExchangeRequest{
				TokenEndpoint: token.URL + "/oauth/tokens", Code: "code", RedirectURI: "heyemail://callback",
				ClientID: "client", ClientSecret: "secret", CodeVerifier: "verifier", InstallID: "install",
			})
		},
		"refresh": func(exchanger *Exchanger) (*Token, error) {
			return exchanger.Refresh(context.Background(), RefreshRequest{
				TokenEndpoint: token.URL + "/oauth/tokens", RefreshToken: "refresh", ClientID: "client",
				ClientSecret: "secret", InstallID: "install",
			})
		},
	}
	// The default client, and a caller's own whose policy would follow.
	clients := map[string]*http.Client{"default": nil, "supplied": {}}
	for clientName, client := range clients {
		for name, request := range requests {
			t.Run(clientName+"/"+name, func(t *testing.T) {
				asked.Store(0)
				elsewhere.Store(0)

				got, err := request(NewExchanger(client))

				if err == nil {
					t.Fatalf("a redirected token request succeeded with %+v", got)
				}
				if n := asked.Load(); n != 1 {
					t.Errorf("the token endpoint was asked %d times, want 1", n)
				}
				if n := elsewhere.Load(); n != 0 {
					t.Errorf("the Location's server was sent the form %d times, want 0", n)
				}
			})
		}
	}
}

// The caller's client is copied, not changed: its own requests still follow redirects.
func TestNewExchangerLeavesTheCallersClientAsItWas(t *testing.T) {
	client := &http.Client{}
	NewExchanger(client)
	if client.CheckRedirect != nil {
		t.Fatal("NewExchanger changed the redirect policy of the client it was given")
	}
}
