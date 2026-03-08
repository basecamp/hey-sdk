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
