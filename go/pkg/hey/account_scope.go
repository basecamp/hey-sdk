package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

const filteredAccountIDParameter = "filtered_account_id"

// ForAccount returns an immutable client that presents HEY's mail data for one
// accessible linked account. It verifies the account against the authenticated
// identity before returning. The returned client shares transport,
// authentication, hooks, logging, configuration, and HTTP cache with its source
// while maintaining its own generated client, services, and account-sensitive
// caches.
//
// HEY applies account scope to mail-oriented operations. Identity-owned
// operations, including Calendar and Journal, retain their identity-wide
// semantics. Account scope is a presentation and acting-account context, not an
// authorization boundary.
func (c *Client) ForAccount(ctx context.Context, accountID int64) (*Client, error) {
	if accountID <= 0 {
		return nil, ErrUsage(fmt.Sprintf("account ID must be positive, got %d", accountID))
	}

	identityClient := &Client{clientShared: c.clientShared, httpClient: c.httpClient}
	identity, err := identityClient.Identity().GetIdentity(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch identity for account validation: %w", err)
	}
	if identity == nil {
		return nil, ErrAPI(0, "could not fetch identity")
	}
	if !identityHasAccessibleAccount(identity, accountID) {
		return nil, ErrNotFound("accessible account", strconv.FormatInt(accountID, 10))
	}

	client := &Client{
		clientShared: c.clientShared,
		httpClient:   accountScopedHTTPClient(c.httpClient),
		accountID:    accountID,
	}
	client.seedAccountIdentity(identity)
	return client, nil
}

// AccountID returns the linked account selected for this client. The boolean is
// false for an All Accounts client.
func (c *Client) AccountID() (accountID int64, ok bool) {
	return c.accountID, c.accountID > 0
}

// AccountUserID returns the authenticated identity's user ID in the selected
// linked account.
func (c *Client) AccountUserID(ctx context.Context) (int64, error) {
	if c.accountID == 0 {
		return 0, ErrUsage("account user ID requires an account-scoped client")
	}

	c.accountUserMu.Lock()
	defer c.accountUserMu.Unlock()

	if c.accountUserDone {
		return c.accountUserID, nil
	}

	identity, err := c.Identity().GetIdentity(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch identity for account user ID: %w", err)
	}
	if identity == nil {
		return 0, ErrAPI(0, "could not fetch identity")
	}
	for _, user := range identity.AllUsers {
		if user.AccountId == c.accountID {
			c.accountUserID = user.Id
			c.accountUserDone = true
			return c.accountUserID, nil
		}
	}

	return 0, ErrNotFound("user for account", strconv.FormatInt(c.accountID, 10))
}

func identityHasAccessibleAccount(identity *generated.Identity, accountID int64) bool {
	for _, account := range identity.Accounts {
		if account.Id == accountID && accountIsAccessible(account) {
			return true
		}
	}
	return false
}

func accountIsAccessible(account generated.Account) bool {
	return account.Status == "active" ||
		(account.Status == "inactive" && (account.Purpose == "work" || account.Purpose == "domains"))
}

func (c *Client) seedAccountIdentity(identity *generated.Identity) {
	for _, sender := range identity.Senders {
		if sender.AccountId == c.accountID && sender.Default {
			c.senderID = sender.Id
			c.senderDone = true
			break
		}
	}
	if !c.senderDone {
		for _, sender := range identity.Senders {
			if sender.AccountId == c.accountID {
				c.senderID = sender.Id
				c.senderDone = true
				break
			}
		}
	}
	for _, user := range identity.AllUsers {
		if user.AccountId == c.accountID {
			c.accountUserID = user.Id
			c.accountUserDone = true
			break
		}
	}
}

// prepareAPIRequest applies the client-wide properties shared by generated,
// document, and form requests.
func (c *Client) prepareAPIRequest(ctx context.Context, req *http.Request) error {
	c.applyAccountScope(req.URL)
	if err := c.authStrategy.Authenticate(ctx, req); err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	return nil
}

func (c *Client) accountScopedURL(rawURL string) (string, error) {
	if c.accountID == 0 {
		return rawURL, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	c.applyAccountScope(parsed)
	return parsed.String(), nil
}

func (c *Client) applyAccountScope(requestURL *url.URL) {
	if c.accountID == 0 || requestURL == nil || c.cfg.BaseURL == "" || !isSameOrigin(c.cfg.BaseURL, requestURL.String()) {
		return
	}

	query := requestURL.Query()
	query.Set(filteredAccountIDParameter, strconv.FormatInt(c.accountID, 10))
	requestURL.RawQuery = query.Encode()
}

func accountScopedHTTPClient(source *http.Client) *http.Client {
	client := *source
	transport := source.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = &filteredAccountCookieTransport{inner: transport}
	return &client
}

type filteredAccountCookieTransport struct {
	inner http.RoundTripper
}

func (t *filteredAccountCookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.Header = req.Header.Clone()
	cookies := request.Cookies()
	request.Header.Del("Cookie")
	for _, cookie := range cookies {
		if cookie.Name != filteredAccountIDParameter {
			request.AddCookie(cookie)
		}
	}
	return t.inner.RoundTrip(request)
}
