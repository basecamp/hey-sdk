package hey

import (
	"context"
	"fmt"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// OAuthService is HEY's own token endpoint, /oauth/tokens: CreateToken trades an
// authorization code or a refresh token for tokens, and RevokeToken ends the session behind
// a token. HEY judges what the body carries, not the caller, so both go out unsigned — no
// Authorization header, whatever the client holds — and a 401 is the answer rather than a
// reason to refresh the client's credentials. Neither follows a redirect, since the body's
// secrets would go with it: a 3xx is an error carrying its status.
//
// A client built with WithAuthStrategy(NoCredentials{}) sends these and nothing else, which
// is what an app signing in, or one revoking the tokens it is about to forget, has to hand.
// The oauth package's Exchanger is for an authorization server the caller discovered.
type OAuthService struct {
	client *Client
}

// NewOAuthService creates an OAuthService.
func NewOAuthService(client *Client) *OAuthService {
	return &OAuthService{client: client}
}

// CreateToken trades a grant for tokens. GrantType names the grant: "authorization_code"
// sends Code, RedirectUri and CodeVerifier, and "refresh_token" sends RefreshToken. A
// refresh answers the refresh token it was sent beside the new access token. It is sent
// once: a code is redeemed once, so a failure is never resent.
func (s *OAuthService) CreateToken(ctx context.Context, body generated.CreateTokenRequestContent) (result *generated.OAuthToken, err error) {
	op := OperationInfo{
		Service: "OAuth", Operation: "CreateToken",
		ResourceType: "token", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, requestErr := s.client.genClient().CreateTokenWithResponse(ctx, body)
		if requestErr != nil {
			return requestErr
		}
		if responseErr := CheckResponse(resp.HTTPResponse); responseErr != nil {
			return responseErr
		}
		if resp.JSON200 == nil {
			return fmt.Errorf("HEY returned an empty token response")
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// RevokeToken revokes an access or a refresh token the client was handed, which is what a
// sign-out asks. The token goes in the body, never the query, where logs would keep it. HEY
// answers a token that names nothing as revoked, and refuses one another client was handed
// with a 401.
func (s *OAuthService) RevokeToken(ctx context.Context, body generated.RevokeTokenRequestContent) error {
	op := OperationInfo{
		Service: "OAuth", Operation: "RevokeToken",
		ResourceType: "token", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().RevokeTokenWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}
