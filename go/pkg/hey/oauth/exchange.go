package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/hey"
)

// Exchanger handles OAuth 2.0 token exchange and refresh operations.
type Exchanger struct {
	httpClient *http.Client
}

// NewExchanger creates an Exchanger with the given HTTP client.
func NewExchanger(httpClient *http.Client) *Exchanger {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Exchanger{httpClient: httpClient}
}

// Exchange exchanges an authorization code for access and refresh tokens.
func (e *Exchanger) Exchange(ctx context.Context, req ExchangeRequest) (*Token, error) {
	if req.TokenEndpoint == "" {
		return nil, fmt.Errorf("token endpoint is required")
	}
	if req.Code == "" {
		return nil, fmt.Errorf("authorization code is required")
	}
	if req.RedirectURI == "" {
		return nil, fmt.Errorf("redirect URI is required")
	}
	if req.ClientID == "" {
		return nil, fmt.Errorf("client ID is required")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", req.Code)
	data.Set("redirect_uri", req.RedirectURI)
	data.Set("client_id", req.ClientID)
	if req.ClientSecret != "" {
		data.Set("client_secret", req.ClientSecret)
	}
	if req.CodeVerifier != "" {
		data.Set("code_verifier", req.CodeVerifier)
	}

	return e.doTokenRequest(ctx, req.TokenEndpoint, data)
}

// Refresh exchanges a refresh token for a new access token.
func (e *Exchanger) Refresh(ctx context.Context, req RefreshRequest) (*Token, error) {
	if req.TokenEndpoint == "" {
		return nil, fmt.Errorf("token endpoint is required")
	}
	if req.RefreshToken == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", req.RefreshToken)
	if req.ClientID != "" {
		data.Set("client_id", req.ClientID)
	}
	if req.ClientSecret != "" {
		data.Set("client_secret", req.ClientSecret)
	}

	return e.doTokenRequest(ctx, req.TokenEndpoint, data)
}

const maxTokenResponseBytes int64 = 1 * 1024 * 1024
const maxErrorMessageLen = 500

func (e *Exchanger) doTokenRequest(ctx context.Context, tokenEndpoint string, data url.Values) (*Token, error) {
	if err := hey.RequireSecureEndpoint(tokenEndpoint); err != nil {
		return nil, fmt.Errorf("token endpoint validation failed for %q: %w", tokenEndpoint, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	lr := io.LimitReader(resp.Body, maxTokenResponseBytes+1)
	body, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}
	if int64(len(body)) > maxTokenResponseBytes {
		return nil, fmt.Errorf("token response body exceeds %d byte limit", maxTokenResponseBytes)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			desc := errResp.ErrorDescription
			if len(desc) > maxErrorMessageLen {
				desc = desc[:maxErrorMessageLen-3] + "..."
			}
			if desc != "" {
				return nil, fmt.Errorf("token error: %s - %s", errResp.Error, desc)
			}
			return nil, fmt.Errorf("token error: %s", errResp.Error)
		}
		bodyStr := string(body)
		if len(bodyStr) > maxErrorMessageLen {
			bodyStr = bodyStr[:maxErrorMessageLen-3] + "..."
		}
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, bodyStr)
	}

	var token Token
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}
