# HEY TypeScript conformance

`make conformance-ts` runs all `conformance/tests/*.json` against a real loopback HTTP
server through generated `HeyClient` methods. Install once with `make ts-install`.
The private runner manifest delegates to the SDK's single frozen toolchain; it has
no separate runtime dependencies. `make conformance-mvp` aggregates Go, Rust and TypeScript.

Current coverage is **228 applicable, 9 explicitly not applicable, 237 total**. The exact
file/name/operation/reason exclusions are in `not-applicable.json`: three cover Go's
unmodeled `UpdateCalendarEvent` form convenience, one covers the handwritten view that
Go, Rust, Kotlin and Swift reduce the modeled `GetWorkflowStage` JSON to, and five cover
redirects, since the TypeScript client follows none: a write's redirect hop refused 401, the
path cases for `UndoTopicSend` and `RestoreEntry`, which HEY answers with a 303 the other
generated clients follow, and the two cases for the hand-written Undo Send that reads the
entry out of that 303. The TypeScript client reports such a 303 as an error with its status;
`typescript/tests/client.test.ts` holds that. The TypeScript
operation returns the stage as HEY answers it, typed by the model.
New unknown operations/assertions/config keys, missing or duplicate fixtures, and
stale/duplicate exclusions fail. No API token is needed or read from the environment.

The fixtures use language-neutral arguments. The harness adapts their flattened body
fields to modeled request envelopes, as the Go runner does. Draft convenience aliases
invoke generated CreateMessage/UpdateMessage/CreateReply with real draft/status/sender
payloads; Go-layer `.json` representation fixtures use the SDK's `format: 'json'` option.
Every substantive wire, error, timing, body and pagination assertion is evaluated.
The SDK unit suite separately covers every modeled operation (including operations
without a shared fixture), multipage behavior, OAuth, account selection and uploads.
