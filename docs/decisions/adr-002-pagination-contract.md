# ADR-002: Pagination Contract

## Status

Accepted

## Context

Both iOS and Android HEY clients confirm the API uses **standard Link header pagination** (`Link: <url>; rel="next"`, `X-Total-Count`), same as Basecamp. The GearedPagination gem sets these headers.

Earlier drafts considered cursor-based body pagination, but that was incorrect. The `next_history_url` and `next_incremental_sync_url` fields in box responses are **sync bookmarks**, not pagination — they're application-level fields for the incremental sync protocol.

## Decision

### Pagination styles

- **`link`** — standard `Link: rel="next"` header (RFC 5988). Used by most list endpoints. The sdk/common pagination conformance tests apply directly with HEY operation/path substitutions.
- **`window`** — date-windowed (`starts_on`/`ends_on` query params). Calendar recordings only. Not paginated in the Link-header sense.

### Sync bookmarks (not pagination)

These are modeled as regular response fields on `BoxShowResponse`:

- `next_history_url` — URL to fetch older box postings
- `next_incremental_sync_url` — URL for incremental sync since last fetch
- `recording_changes_url` / `calendar_changes_url` — calendar sync URLs

### Conformance

sdk/common's `pagination.json` conformance tests apply as-is. No custom `pagination-cursor-body.json` needed. Only operation ID and path substitutions required.

## Consequences

- Pagination implementation in all 5 SDKs follows the identical pattern as basecamp-sdk.
- Sync bookmark URLs are opaque strings — the SDK passes them through to the caller.
- No `PageIterator` is needed for window-style calendar queries (they return full result sets within the date range).

### Gate targets

- **`conformance-mvp`**: runs MVP behavioral tests (including pagination) for Go only.
- **`conformance-full`**: runs MVP + full-surface tests for all 5 languages.
- Bare `conformance` alias maps to `conformance-full`.
