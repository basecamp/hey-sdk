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
//
// The client asks for one refresh per set of credentials, however many requests were
// signed with them: the requests whose 401s arrive while it runs wait for its answer,
// and the ones whose 401s arrive after it has ended take that answer, a failure
// included, so a refresh token is rotated once and an outage at the issuer costs one
// call. A request signed after a failed refresh asks again. No request is signed while
// Refresh runs, and Refresh runs on a context that outlives the request that drew the
// 401, bound by the client's request timeout rather than the request's own cancellation.
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
	req.Header.Set("Authorization", bearerCredential(token))
	return nil
}
