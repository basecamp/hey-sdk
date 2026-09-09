package hey

import (
	"context"
	"net/http"
)

// AuthStrategy controls how authentication is applied to HTTP requests.
// The default strategy is BearerAuth, which uses a TokenProvider to set
// the Authorization header with a Bearer token.
type AuthStrategy interface {
	// Authenticate applies authentication to the given HTTP request.
	Authenticate(ctx context.Context, req *http.Request) error
}

// TokenRefresher renews the credentials a request is authenticated with, which is what
// lets a 401 be retried rather than surfaced. AuthManager is one, and so is any
// AuthStrategy or TokenProvider a caller brings that can renew what it hands out — a
// client that keeps its credentials somewhere else is exactly the case that needs this,
// since it has no AuthManager for the client to recognise.
type TokenRefresher interface {
	Refresh(ctx context.Context) error
}

// BearerAuth implements AuthStrategy using OAuth Bearer tokens.
// This is the default authentication strategy.
type BearerAuth struct {
	TokenProvider TokenProvider
}

// Authenticate sets the Authorization header with a Bearer token.
func (b *BearerAuth) Authenticate(ctx context.Context, req *http.Request) error {
	token, err := b.TokenProvider.AccessToken(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}
