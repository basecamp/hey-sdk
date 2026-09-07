# @37signals/hey

TypeScript SDK for HEY, generated from the same Smithy/OpenAPI contract as the Go SDK.
All **130 modeled operations** have typed lower-camel-case methods on `HeyClient`.

## Install and runtimes

```sh
npm install @37signals/hey
```

**Registry provisioning is pending:** the proposed name is `@37signals/hey`; until
maintainers activate publishing, install a locally built `npm pack` tarball instead.
A repository release does not imply this package is on npm. See [release setup](../TYPESCRIPT_RELEASE.md).

Supported: **Node.js 22.12+ (22.x), 24.x and 26.x**, native ESM, TypeScript 5.9+.
CI tests those Node majors. No browser, React Native, Deno, Bun or CommonJS support is claimed.
Uses native Fetch, AbortSignal and Node crypto; no API token is needed for tests.

## Read and send

```ts
import { HeyClient, HeyError } from '@37signals/hey';

const hey = new HeyClient({ token: process.env.HEY_TOKEN! });
const { data: boxes } = await hey.listBoxes();
const { data: box } = await hey.getBox({ path: { boxId: 123 } });

// Use an identity sender explicitly; unlike Go's convenience wrapper, a root
// generated call does not fetch the default acting sender for you.
const { data: identity } = await hey.getIdentity();
const sender = identity?.senders?.find(s => s.default) ?? identity?.senders?.[0];
if (!sender?.id) throw new Error('No sender');
await hey.createMessage({ body: {
  acting_sender_id: sender.id,
  message: { subject: 'Hello', content: 'Hello from HEY' },
  entry: { addressed: { directly: ['someone@example.com'] } },
} });
```

Inputs use `{ path, query, body }`; fields keep HEY's wire spelling. Output is
`{ data, status, headers, totalCount?, nextPage?, nextUrl? }`. Bodyless success and
`GetOngoingTimeTrack`'s annotated 404 produce `data: undefined`; other 404s throw.
No redirect is followed, including same-origin redirects. Use `{format: 'json'}`
as the second argument to explicitly select Rails' `.json` representation.

**Full modeled coverage is not full Go-wrapper parity.** Calendar event **updates
are unavailable**: their form PATCH/PUT routes are excluded from the model. Other
unmodeled HTML/form conveniences in Go are likewise not invented here. There is no
HTML scraping or generic arbitrary-path API. Modeled deletes and JSON reads work.
Drafting uses generated `createMessage`/`updateMessage` with `entry.status: 'drafted'`;
delivery omits that status and supplies recipients. HEY may otherwise save a draft.
Use `newEntryReply` to fetch the actual sender/subject/recipient prefill for replies.
An empty success's `Location` remains available through `headers`; it is never followed.

## Accounts

```ts
const work = await hey.forAccount(42); // verifies accessible account via identity
await work.createMessage({ body: {
  acting_sender_id: 0, // selects this linked account's default sender, or errors
  message: { subject: 'Work', content: 'Hello' },
  entry: { addressed: { directly: ['someone@example.com'] } },
} });
```

Root clients present All Accounts. Derived clients are immutable scopes and add exactly
one `filtered_account_id` to same-origin requests, including pagination and retries.
They select acting senders/users from the account when the body supplies zero (or an
optional acting field is absent), and fail rather than fall back to a different account.
Explicit acting IDs remain caller-controlled, as in Go; use `accountSenderId()` and
`accountUserId()` to inspect the selected defaults. Scope is mail filtering, **not an
authorization boundary**: Calendar and Journal remain identity-wide. Re-derive after
membership changes. Separate unlinked identities require separate root token providers.

## Pagination and data fidelity

Each method fetches **one page**; metadata and envelopes remain intact. To follow Link
pagination explicitly, preserving relative URLs and query cursors:

```ts
for await (const page of hey.pages('ListContacts', { query: { q: 'Jane' } })) {
  console.log(page.data);
}
```

Only Link-paginated operations support `pages`. Calendar window queries are single
requests; pass `starts_on`, `ends_on` and optionally `page` yourself. Sync bookmark
fields such as `next_history_url` are data, never pagination. Foreign-origin links,
credential-bearing links, cycles and exceeding `maxPages` (default 100) throw rather
than silently truncate. This differs intentionally from Go's automatic aggregation.

64-bit integers use `number | bigint`: safe values are numbers, larger integer JSON
literals are bigints. Supply bigint for large IDs; unsafe input numbers are rejected
before sending. Unsafe exponent/decimal encodings of large integral values are rejected
instead of rounded. Dates remain strings; optional fields stay absent and JSON null remains
null. Use a bigint-aware serializer for application persistence (`JSON.stringify`
cannot serialize bigint). Generated discriminator guards narrow flat Posting/Recording
objects without making optional variant fields required.

## Auth, errors and limits

`token` can be a string or `{ getToken(), refresh?() }`. A definite 401 triggers at most
one refresh and resend, even for mutations; concurrent refreshes coalesce. Refresh
failure propagates. Store OAuth credentials outside the SDK. `@37signals/hey/oauth`
provides HEY discovery, PKCE, authorization URL, code exchange and refresh helpers:
standard `authorization_code`/`refresh_token` form grants, not Basecamp's auth hosts.
Verify the returned OAuth `state` in your callback before exchanging the code; persist
rotated refresh tokens and use provider `refresh` to renew them. See [example](examples/oauth.ts).

`HeyError` has `code`, `httpStatus`, `retryable`, `requestId`, `hint` and `cause`.
Error messages are bounded and HTML error pages are not echoed. `retryable` describes
the error, not permission to replay a mutation. Only model-declared safe operations
retry 429/503 with Retry-After/exponential delay. Explicit `natural: false` overrides
HTTP method inference (notably `UpdateMessage` PUT). Ambiguous network errors are not
replayed. There are no invented idempotency headers. Redirects are surfaced as errors.

HTTPS is mandatory except localhost HTTP. Default timeout is 30 seconds per operation,
including retries; pass `{ signal }` for cancellation. JSON/error bodies are capped at
16 MiB of decompressed bytes (`maxResponseBodyBytes`, positive and finite). Options
include `maxRetries`, `maxPages`, `timeoutMs`, `fetch` and opt-in `cache` (bounded ETag
revalidation partitioned by current token and scoped URL). No Go circuit breaker,
bulkhead, hooks, persistent token store, or streaming download parity is claimed.

## Attachments

Call generated `createDirectUpload` with the filename, byte size, base64 MD5 checksum
(Active Storage's integrity requirement), and content type. Then call
`uploadBytes(result.data!, bytes)` with the returned target. Signed upload URLs receive
only returned headers (Authorization/Cookie removed) and the exact bytes, never the
HEY token or account filter. Redirects and insecure targets are refused. There is no
automatic signing/download helper: returned signed download URLs remain opaque data.

## Development

```sh
npm ci
npm run generate          # OpenAPI + behavior model -> types, methods, metadata, guards
npm run check:generated   # deterministic byte freshness + exact operation coverage
npm run build
npm run typecheck
npm test
npm run conformance
npm run smoke             # pack, isolated offline install, ESM + consumer typecheck
```

The generator adapts Basecamp's openapi-typescript/metadata/service pipeline, not its
account-prefix stripping, service definitions, traits or operation maps. Generated
files are committed; never edit them directly. Conformance uses real HTTP loopback
requests and fails closed on unknown fixtures/assertions. Exactly three named Go-only
calendar-update form fixtures are explicitly not applicable; see the runner manifest.
