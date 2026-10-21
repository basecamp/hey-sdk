package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/basecamp/hey-sdk/go/pkg/hey"
)

// Discoverer fetches OAuth 2.0 server configuration from discovery endpoints.
type Discoverer struct {
	httpClient *http.Client
}

// NewDiscoverer creates a Discoverer with the given HTTP client, or http.DefaultClient's
// settings. Either way the discoverer follows no redirect: the metadata names the endpoints
// a code, its verifier, the client secret and refresh tokens go to, and a redirect would take
// them from whatever answered it. The issuer fixes the metadata's URL, so a 3xx is an error.
// The caller's client is copied, not changed.
func NewDiscoverer(httpClient *http.Client) *Discoverer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	unredirected := *httpClient
	unredirected.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Discoverer{httpClient: &unredirected}
}

// Discover fetches the metadata the issuer at baseURL publishes (RFC 8414). The issuer must be
// HTTPS, with no query, fragment or credentials, and is refused before anything is sent. The
// metadata is used only when it names that same issuer and HTTPS endpoints for it.
//
// HEY publishes no metadata; its endpoints are /oauth/authorizations/new and /oauth/tokens.
func (d *Discoverer) Discover(ctx context.Context, baseURL string) (*Config, error) {
	issuer, err := url.Parse(baseURL)
	if err != nil || issuer.Host == "" || issuer.User != nil || issuer.RawQuery != "" || issuer.ForceQuery || issuer.Fragment != "" {
		return nil, fmt.Errorf("OAuth issuer %q must be an absolute URL without a query, fragment or credentials", baseURL)
	}
	if err := hey.RequireSecureEndpoint(baseURL); err != nil {
		return nil, fmt.Errorf("OAuth issuer validation failed: %w", err)
	}
	// The well-known path goes between the host and the issuer's own path (RFC 8414 §3.1).
	discoveryURL := url.URL{
		Scheme: issuer.Scheme,
		Host:   issuer.Host,
		Path:   "/.well-known/oauth-authorization-server" + strings.TrimSuffix(issuer.Path, "/"),
	}

	req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discovery request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("discovery failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading discovery response: %w", err)
	}
	if int64(len(body)) > maxTokenResponseBytes {
		return nil, fmt.Errorf("discovery response body exceeds %d byte limit", maxTokenResponseBytes)
	}

	var config Config
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("parsing discovery response: %w", err)
	}
	if err := config.validate(baseURL); err != nil {
		return nil, err
	}

	return &config, nil
}

// validate holds the metadata to RFC 8414 §3.3: it names the issuer it was asked for, or it
// is not used, which keeps one server from answering for another. Endpoints may sit on other
// hosts, but every one must be HTTPS, and the three a client needs must be there.
func (c *Config) validate(issuer string) error {
	if c.Issuer == "" || c.AuthorizationEndpoint == "" || c.TokenEndpoint == "" {
		return fmt.Errorf("incomplete discovery response: issuer, authorization_endpoint and token_endpoint are required")
	}
	endpoints := []string{c.Issuer, c.AuthorizationEndpoint, c.TokenEndpoint}
	if c.RegistrationEndpoint != "" {
		endpoints = append(endpoints, c.RegistrationEndpoint)
	}
	for _, endpoint := range endpoints {
		if parsed, err := url.Parse(endpoint); err != nil || parsed.Host == "" {
			return fmt.Errorf("invalid discovery response: %q is not an absolute URL", endpoint)
		}
		if err := hey.RequireSecureEndpoint(endpoint); err != nil {
			return fmt.Errorf("invalid discovery response: %w", err)
		}
	}
	if c.Issuer != issuer {
		return fmt.Errorf("discovery issuer %q does not match %q", c.Issuer, issuer)
	}
	return nil
}
