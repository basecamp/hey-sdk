# HEY TypeScript conformance

`make conformance-ts` runs all `conformance/tests/*.json` against a real loopback HTTP
server through generated `HeyClient` methods. Install once with `make ts-install`.
The private runner manifest delegates to the SDK's single frozen toolchain; it has
no separate runtime dependencies. `make conformance-mvp` aggregates Go, Rust and TypeScript.

Current coverage is **191 applicable, 4 explicitly not applicable, 195 total**. The exact
file/name/operation/reason exclusions are in `not-applicable.json`: three cover Go's
unmodeled `UpdateCalendarEvent` form convenience, and one covers the handwritten parsed
view that Go and Rust layer over the modeled `GetWorkflowStage` HTML blob. The TypeScript
operation returns that blob as a typed string and has direct transport regression coverage.
New unknown operations/assertions/config keys, missing or duplicate fixtures, and
stale/duplicate exclusions fail. No API token is needed or read from the environment.

The fixtures use language-neutral arguments. The harness adapts their flattened body
fields to modeled request envelopes, as the Go runner does. Draft convenience aliases
invoke generated CreateMessage/UpdateMessage/CreateReply with real draft/status/sender
payloads; Go-layer `.json` representation fixtures use the SDK's `format: 'json'` option.
Every substantive wire, error, timing, body and pagination assertion is evaluated.
The SDK unit suite separately covers every modeled operation (including operations
without a shared fixture), multipage behavior, OAuth, account selection and uploads.
