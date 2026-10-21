package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// metadataOf is what an issuer at issuer publishes about itself.
func metadataOf(issuer string) map[string]any {
	return map[string]any{
		"issuer":                 issuer,
		"authorization_endpoint": issuer + "/authorize",
		"token_endpoint":         issuer + "/token",
	}
}

// serveMetadata answers every request with the metadata write fills in once the server's
// URL is known, and counts the requests.
func serveMetadata(t *testing.T, write func(url string) map[string]any) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var asked atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(write(server.URL))
	}))
	t.Cleanup(server.Close)
	return server, &asked
}

func TestDiscoverReadsTheMetadataOfTheIssuerItWasGiven(t *testing.T) {
	server, _ := serveMetadata(t, func(url string) map[string]any {
		metadata := metadataOf(url)
		metadata["registration_endpoint"] = "https://register.example/oauth"
		metadata["scopes_supported"] = []string{"read", "write"}
		return metadata
	})

	config, err := NewDiscoverer(nil).Discover(context.Background(), server.URL)

	if err != nil {
		t.Fatal(err)
	}
	if config.Issuer != server.URL || config.TokenEndpoint != server.URL+"/token" {
		t.Errorf("got %+v", config)
	}
	if config.RegistrationEndpoint != "https://register.example/oauth" || len(config.ScopesSupported) != 2 {
		t.Errorf("got %+v", config)
	}
}

// The metadata names the endpoints a code, its verifier, the client secret and refresh tokens
// go to. A redirect would take them from whatever answered it, so discovery follows none: the
// metadata URL is fixed by the issuer, and a 3xx is an error.
func TestDiscoverFollowsNoRedirect(t *testing.T) {
	// The Location's server would vouch for the issuer that sent the client there.
	var issuer *httptest.Server
	other, elsewhere := serveMetadata(t, func(string) map[string]any { return metadataOf(issuer.URL) })
	var asked atomic.Int32
	issuer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		w.Header().Set("Location", other.URL+"/.well-known/oauth-authorization-server")
		w.WriteHeader(http.StatusFound)
	}))
	defer issuer.Close()

	// The default client, and a caller's own whose policy would follow.
	for name, client := range map[string]*http.Client{"default": nil, "supplied": {}} {
		t.Run(name, func(t *testing.T) {
			asked.Store(0)
			elsewhere.Store(0)

			config, err := NewDiscoverer(client).Discover(context.Background(), issuer.URL)

			if err == nil {
				t.Fatalf("a redirected discovery succeeded with %+v", config)
			}
			if n := asked.Load(); n != 1 {
				t.Errorf("the issuer was asked %d times, want 1", n)
			}
			if n := elsewhere.Load(); n != 0 {
				t.Errorf("the Location's server was asked %d times, want 0", n)
			}
		})
	}
}

// Over plain HTTP anyone on the network could answer with endpoints of their own, so an
// issuer must be HTTPS, as the token endpoint already must, and is refused before anything
// is sent. A query, fragment or credentials have no place in an issuer.
func TestDiscoverRefusesAnInsecureIssuer(t *testing.T) {
	var sent atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		return nil, http.ErrHandlerTimeout
	})}

	for _, issuer := range []string{
		"http://evil.example",
		"https://app.hey.com?tenant=1",
		"https://app.hey.com#fragment",
		"https://app.hey.com?",
		"https://app.hey.com#",
		"https://user:secret@app.hey.com",
		"app.hey.com",
	} {
		t.Run(issuer, func(t *testing.T) {
			if _, err := NewDiscoverer(client).Discover(context.Background(), issuer); err == nil {
				t.Fatal("an insecure issuer was accepted")
			}
		})
	}
	if n := sent.Load(); n != 0 {
		t.Errorf("%d requests were sent for insecure issuers, want 0", n)
	}
}

// An error is logged, so a password in the issuer stays out of it.
func TestDiscoverKeepsAnIssuersCredentialsOutOfItsError(t *testing.T) {
	for _, issuer := range []string{"https://user:secret@app.hey.com", "http://user:secret@evil.example"} {
		_, err := NewDiscoverer(nil).Discover(context.Background(), issuer)

		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("got %v", err)
		}
	}
}

// RFC 8414 §3.3: the issuer the metadata names must be the one it was asked for, or the
// metadata is not used, which keeps one server from answering for another.
func TestDiscoverRefusesMetadataOfAnotherIssuer(t *testing.T) {
	server, _ := serveMetadata(t, func(string) map[string]any { return metadataOf("https://issuer.example") })

	_, err := NewDiscoverer(nil).Discover(context.Background(), server.URL)

	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("got %v, want an issuer mismatch", err)
	}
}

// Endpoints may sit on other hosts, but every one of them must be HTTPS, and the three a
// client needs must be there.
func TestDiscoverRefusesIncompleteOrInsecureMetadata(t *testing.T) {
	cases := map[string]func(metadata map[string]any){
		"no token endpoint":         func(m map[string]any) { delete(m, "token_endpoint") },
		"no authorization endpoint": func(m map[string]any) { m["authorization_endpoint"] = "" },
		"an HTTP token endpoint":    func(m map[string]any) { m["token_endpoint"] = "http://evil.example/token" },
		"an HTTP authorization endpoint": func(m map[string]any) {
			m["authorization_endpoint"] = "http://evil.example/authorize"
		},
		"an HTTP registration endpoint": func(m map[string]any) {
			m["registration_endpoint"] = "http://evil.example/register"
		},
		"a token endpoint with no host": func(m map[string]any) { m["token_endpoint"] = "https:/token" },
		"a token endpoint with credentials": func(m map[string]any) {
			m["token_endpoint"] = "https://user:secret@token.example/oauth"
		},
		"a token endpoint with a fragment": func(m map[string]any) {
			m["token_endpoint"] = "https://token.example/oauth#fragment"
		},
		"a non-HTTP endpoint on localhost": func(m map[string]any) { m["token_endpoint"] = "ftp://localhost/token" },
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			server, _ := serveMetadata(t, func(url string) map[string]any {
				metadata := metadataOf(url)
				spoil(metadata)
				return metadata
			})

			if config, err := NewDiscoverer(nil).Discover(context.Background(), server.URL); err == nil {
				t.Fatalf("accepted %+v", config)
			}
		})
	}

	server, _ := serveMetadata(t, func(url string) map[string]any {
		metadata := metadataOf(url)
		metadata["token_endpoint"] = "https://token.example/oauth"
		return metadata
	})
	if _, err := NewDiscoverer(nil).Discover(context.Background(), server.URL); err != nil {
		t.Errorf("an HTTPS endpoint on another host was refused: %v", err)
	}
}

// RFC 8414 §3.1: the well-known path goes between the host and the issuer's own path.
func TestDiscoverAsksBeforeTheIssuersPath(t *testing.T) {
	var requested string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadataOf(server.URL + "/tenant"))
	}))
	defer server.Close()

	if _, err := NewDiscoverer(nil).Discover(context.Background(), server.URL+"/tenant"); err != nil {
		t.Fatal(err)
	}
	if requested != "/.well-known/oauth-authorization-server/tenant" {
		t.Errorf("asked for %q", requested)
	}
}

// An escaped slash in the issuer's path stays escaped, as it is one segment.
func TestDiscoverKeepsTheIssuersPathEscaped(t *testing.T) {
	var requested string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadataOf(server.URL + "/a%2Fb"))
	}))
	defer server.Close()

	if _, err := NewDiscoverer(nil).Discover(context.Background(), server.URL+"/a%2Fb"); err != nil {
		t.Fatal(err)
	}
	if requested != "/.well-known/oauth-authorization-server/a%2Fb" {
		t.Errorf("asked for %q", requested)
	}
}

func TestDiscoverLimitsTheMetadataItReads(t *testing.T) {
	server, _ := serveMetadata(t, func(url string) map[string]any {
		metadata := metadataOf(url)
		metadata["padding"] = strings.Repeat("x", int(maxTokenResponseBytes))
		return metadata
	})

	if _, err := NewDiscoverer(nil).Discover(context.Background(), server.URL); err == nil {
		t.Fatal("metadata over the limit was read")
	}
}

// The caller's client is copied, not changed: its own requests still follow redirects.
func TestNewDiscovererLeavesTheCallersClientAsItWas(t *testing.T) {
	client := &http.Client{}
	NewDiscoverer(client)
	if client.CheckRedirect != nil {
		t.Fatal("NewDiscoverer changed the redirect policy of the client it was given")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
