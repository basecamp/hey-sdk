# ADR-004: Endpoint-Specific 404-as-Empty

## Status

Accepted

## Context

`GET /calendar/ongoing_time_track.json` returns HTTP 404 when no time track is currently active. This is not an error — it's a "none" state. The SDK's generic error mapping would surface this as a `not_found` error, causing a regression for the hey-cli (PR #2 maps this to "No active time track").

This is a semantic signal: the server uses 404 to mean "this resource does not exist right now" rather than "you requested an invalid URL."

## Decision

A dedicated **`@heyEmptyOn(statusCodes)`** Smithy trait, separate from `@heyRetry`. Retry policy and response semantics are orthogonal concerns.

`GetOngoingTimeTrack` is annotated:

```smithy
@heyEmptyOn(statusCodes: [404])
```

The behavior model emits this as `x-hey-empty-on: [404]`. Each SDK checks this extension before error mapping — a 404 on this operation returns a nil/zero-value result, not an error.

### SDK behavior per language

| Language | Behavior |
|----------|----------|
| **Go** | Returns `(nil, nil)` — no result and no error |
| **TypeScript** | Returns `undefined` (nullable return type) |
| **Ruby** | Returns `nil` |
| **Swift** | Returns `nil` (optional return type) |
| **Kotlin** | Returns `null` (nullable return type) |

### Conformance

`conformance/tests/empty-on-404.json` validates this behavior: mock a 404 for `GetOngoingTimeTrack`, assert no error and nil/empty result.

## Consequences

- The pattern is extensible. If other endpoints use status codes for state signaling, they get the same annotation.
- Generic error mapping runs *after* the `@heyEmptyOn` check — operations without this trait still surface 404 as errors.
- The trait is operation-level, not global. Each endpoint opts in explicitly.

### Drift check targets

- **`drift-check-mvp`**: runs forward + shape checks only.
- **`drift-check-full`**: runs forward + reverse + shape checks.
- `drift-check` alias maps to `drift-check-mvp`.
