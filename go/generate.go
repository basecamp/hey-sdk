// Package generate provides code generation directives for the HEY SDK.
//
// Run `go generate ./...` from the go directory to regenerate the client code.
//
// The generator reads openapi.json with each operation's idempotency decided from
// the x-hey-idempotent override, behavior-model.json and the verb, written onto the
// operation as x-go-idempotent by scripts/merge-behavior-model. The merged document is
// a build intermediate and is not checked in.
//
//go:generate ../scripts/merge-behavior-model ../openapi.json ../behavior-model.json openapi.behavior.json
//go:generate go tool oapi-codegen -config oapi-codegen.yaml openapi.behavior.json
package generate
