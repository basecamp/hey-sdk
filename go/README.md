# HEY Go SDK

```sh
go get github.com/basecamp/hey-sdk/go@latest
```

Requires Go 1.26+. See the [repository guide](../README.md) for authentication,
linked accounts, services, errors, pagination and resilience. This remains the same
public Go API; adding TypeScript does not regenerate or change the Go client.

```go
cfg := hey.DefaultConfig()
client := hey.NewClient(cfg, &hey.StaticTokenProvider{Token: os.Getenv("HEY_TOKEN")})
boxes, err := client.Boxes().List(context.Background())
```

The [existing service tests](pkg/hey/services_test.go) are hermetic usage examples.
From the repository root, run `make go-check conformance-go`; the complete shipped
SDK gate is `make check` (requires TypeScript dependencies installed with `make ts-install`).
