# HEY Go SDK

```sh
go get github.com/basecamp/hey-sdk/go@latest
```

Requires Go 1.26+. See the [repository guide](../README.md) for authentication,
linked accounts, services, errors, pagination and resilience. Generated schema types
preserve the modeled wire representation, including nullable recording categories.

A 401 is answered by one credential refresh per set of credentials, shared by every
request signed with them and by every client derived from the same root; a refresh that
fails is shared too, and a request signed after it asks again. The refresh runs on a
context that outlives the request that drew the 401, bound by the client's request timeout;
a request whose own context ends while it waits returns its context's error and leaves the
refresh running. When the SDK's bearer strategy signed the request, the provider is asked
what it would sign with first: a token it has already renewed is resent without a refresh,
and a provider that cannot hand over a token at all is the refresh failing. A non-idempotent
write HEY answered with a redirect may already be made (a 302 or 303 can answer a write that
went through): a 401 on its hop refreshes, and is the caller's rather than a reason to risk
making the write twice. An idempotent operation is resent as any other.

```go
cfg := hey.DefaultConfig()
client := hey.NewClient(cfg, &hey.StaticTokenProvider{Token: os.Getenv("HEY_TOKEN")})
boxes, err := client.Boxes().List(context.Background())
```

HEY's token endpoint is `client.OAuth()`: `CreateToken` trades an authorization code or a
refresh token for tokens, and `RevokeToken` revokes one. Both go out unsigned — no
Authorization header, whatever the client holds — and a 401 is their answer, never a reason
to refresh; neither follows a redirect, so a 3xx is an error carrying its status and the
body's secrets go nowhere else. A client account-scoped with `ForAccount` sends them
unscoped. `NewClient` refuses a client given neither a token provider nor a strategy; one
meant to hold no credentials says so with `NoCredentials`, and refuses every other operation
with `auth_required` before sending it:

```go
client := hey.NewClient(cfg, nil, hey.WithAuthStrategy(hey.NoCredentials{}))
token, err := client.OAuth().CreateToken(ctx, generated.CreateTokenRequestContent{
	GrantType: "authorization_code", ClientId: clientID, InstallId: installID,
	Code: code, RedirectUri: redirectURI, CodeVerifier: verifier,
})
```

The `oauth` package's `Exchanger` remains the client for an authorization server the caller
discovered.

The [existing service tests](pkg/hey/services_test.go) are hermetic usage examples.
From the repository root, run `make go-check conformance-go`; the complete shipped
SDK gate is `make check` (requires TypeScript dependencies installed with `make ts-install`).
