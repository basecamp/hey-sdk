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
context that outlives the request that drew the 401, bound by the client's request timeout.

```go
cfg := hey.DefaultConfig()
client := hey.NewClient(cfg, &hey.StaticTokenProvider{Token: os.Getenv("HEY_TOKEN")})
boxes, err := client.Boxes().List(context.Background())
```

The [existing service tests](pkg/hey/services_test.go) are hermetic usage examples.
From the repository root, run `make go-check conformance-go`; the complete shipped
SDK gate is `make check` (requires TypeScript dependencies installed with `make ts-install`).
