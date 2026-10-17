# ADR-005: A Delivery's Success Is Not Undone by Its Answer

## Status

Accepted

## Context

`CreateMessage`, `CreateReply` and `UpdateMessage` deliver mail. By the time HEY answers a
2xx the message has gone out — or, under Undo Send, is on its way. HEY's answer names the entry
that went out and its thread, but it is a report, not the write: a truncated body, an answer
that is not JSON, or a body the client will not hold says nothing about whether the message
was sent.

Decoding that answer strictly turned those cases into errors. A caller told a delivered
message failed sends it again, and the recipient gets it twice. The hand-written conveniences
read the answer leniently, but the generated operations did not, and TypeScript has no
conveniences at all.

## Decision

A dedicated **`@heyLenientSuccess`** Smithy trait marks an operation whose work is done once
HEY answers a success. It is emitted as `x-hey-lenient-success`, carried onto each SDK's
route metadata, and honoured by the transport, so every entry point — generated operation or
convenience — behaves the same:

- A 2xx whose body is empty, cannot be read (past the response size limit, or lost mid-read)
  or does not decode is an **empty result**, not an error.
- A status outside 2xx is an error exactly as before, whatever its body.
- An operation without the trait decodes strictly, as before.

`CreateMessage`, `CreateReply` and `UpdateMessage` carry it. Their draft saves answer 204 with
no body, which reads as an empty result too.

### SDK behavior per language

| Language | Empty result |
|----------|--------------|
| **Go** | The generated `Parse*Response` returns no error and leaves the typed payload (`JSON200`) nil; the delivery wrappers answer an empty `SentMessage` |
| **TypeScript** | `data` is `undefined`, as for any empty success |
| **Rust** | What `{}` decodes to — a `SentMessage` with every field `None` |
| **Kotlin** | What `{}` decodes to — `SentMessage()` |
| **Swift** | What `{}` decodes to — `SentMessage()` |

The empty value comes from decoding `{}` rather than a per-type default, so it applies to any
output whose members are all optional. An operation whose output has required members should
not carry the trait: there is no empty value to answer.

### Raw requests

A raw request — Go's `Client.Post`, `PostMutation`, `Put`, `Patch`, `Delete` and the form
helpers, Rust's `raw.rs` helpers and `Operation::raw`, Kotlin's `HeyClient.request` and
Swift's `HeyClient.request` — has no operation to carry the trait, yet it is how hey-cli and
hey-mcp-server send mail. So a raw request with any method but GET (or HEAD) treats a 2xx
whose body cannot be read, because it was lost mid-read or ran past the response size limit,
as the success it was. The response comes back with no body and the read failure attached,
and the request is sent once and never resent:

| Language | Attached failure |
|----------|------------------|
| **Go** | `Response.BodyErr` / `FormResponse.BodyErr`, a `*BodyReadError`; also logged as a warning |
| **Rust** | `Response.body_error`; also a trace warning. A raw form's `FormResponse` comes back with an empty body and only the warning, since it derives `Eq` |
| **Kotlin** | `Response.bodyError` / `FormResponse.bodyError` |
| **Swift** | `Response.bodyError`. A raw form's `FormResponse` comes back with an empty body, since it is `Equatable` |

A call that only needs the write to have landed (`execute`, the raw helpers, Rust's
`send_unit`, Kotlin's `sendUnit`, Swift's `sendVoid`, the form calls) succeeds. A call that
asks for the decoded body (`send<T>`, `send_text`/`sendText`) answers the attached failure,
since the body it asked for never arrived.

Decoding such a response (`UnmarshalData`, `json`) answers the attached failure rather than a
decode error. A raw GET whose body breaks is still an error, since nothing happened that
fetching again would repeat. So is a read the caller's own cancellation ends, so a cancelled
send is never reported delivered. A raw 2xx body that arrives whole but is not JSON is
returned as it is: decoding it is the caller's business. TypeScript has no raw requests.

### Conformance

`conformance/tests/sent-messages.json` serves a delivery an HTML 200 on the generated
operation and on the draft-send convenience and asserts no error, and serves a refused
delivery an HTML 422 and asserts the validation error is kept.

## Consequences

- A delivered message is never reported as failed because of what HEY said about it. The cost
  is the ids: a caller that needs the thread falls back to finding it.
- `CreateBulkReply` also delivers, but its answer's members are required — the delivery id is
  what undoes it — so it does not carry the trait.
- The trait is operation-level. Each write opts in explicitly.
