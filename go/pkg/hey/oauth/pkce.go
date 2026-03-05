package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PKCE holds a code verifier and its corresponding challenge for OAuth 2.0 PKCE flow.
type PKCE struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE returns a cryptographically secure PKCE code verifier and its SHA256 code challenge.
func GeneratePKCE() (*PKCE, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}

	verifier := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &PKCE{
		Verifier:  verifier,
		Challenge: challenge,
	}, nil
}

// GenerateState returns a cryptographically secure OAuth state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
