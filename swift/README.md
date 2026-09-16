# HEY Swift SDK

The Swift client for the [HEY](https://www.hey.com) API. Models, routes and service methods are
generated from the Smithy model in the repository's `spec/` directory, so what the library
offers is what HEY serves. It is an `async`/`await` client on Foundation's `URLSession` with
strict Swift 6 concurrency.

```swift
dependencies: [
    .package(url: "https://github.com/basecamp/hey-sdk", from: "0.31.0"),
]
```

Requires Swift 6.0 or newer, on macOS 13, iOS 16, or Linux. See [Install](#install).

## Authenticate

A fixed token, for scripts and anything that already holds one:

```swift
import Hey

let client = try HeyClient(accessToken: ProcessInfo.processInfo.environment["HEY_TOKEN"] ?? "")
```

A blank token is refused as a usage error when the client is made, rather than failing on the
first request.

An application that keeps OAuth tokens hands the client a `TokenProvider` over them: the client
asks it for the token on every request and asks it to `refresh()` once when HEY answers 401,
then sends the request again. One refresh serves every request that was signed with the stale
credentials, whether it renews them or fails, and a provider that renews of its own accord —
handing over a new token ahead of expiry — is not asked to refresh for a 401 on the old one; the
request is simply resent. Anything that wants the request headers outright conforms to
`AuthStrategy` and passes it as `HeyClient(auth:)`.

```swift
struct StoredTokens: TokenProvider {
    let store: Store
    func accessToken() async throws -> String { try await store.accessToken() }
    func refresh() async throws -> Bool { try await store.renew() }
}

let client = try HeyClient(tokenProvider: StoredTokens(store: store))
```

## Use it

```swift
let boxes = try await client.boxes.list()              // a Page: .value is the list HEY answered
for box in boxes.value { print("\(box.name) (\(box.kind))") }
let imbox = try await client.boxes.getImbox()
print("\(imbox.postings?.count ?? 0) postings")

// Sending: recipients are required. HEY saves an unaddressed message as a draft.
try await client.messages.send(MessageContent(
    subject: "Subject",
    content: "<div>Body</div>",
    to: ["someone@example.com"]
))

// Replying: start from the prefill. It carries the subject, the acting sender and the
// recipients HEY resolved, which differ from the account default on shared addresses.
let prefill = try await client.entries.newReply(entryId: entryId)
try await client.entries.reply(entryId: entryId, reply: ReplyContent(
    actingSenderId: prefill.sender?.id ?? 0,
    subject: prefill.subject ?? "",
    content: "<div>Reply</div>",
    to: (prefill.addressed?.directly ?? []).compactMap { $0.emailAddress?.expose() }
))

// Postings are bulk operations, as they are in HEY. Moving by kind resolves the box index
// once per client.
try await client.postings.markPostingsSeen(postingIds: [a, b])
try await client.postings.moveToSetAside(postingIds: [a])
let trail = try await client.boxes.idByKind(.paperTrail)

// Calendar
let track = try await client.timeTracks.startTracking()
try await client.timeTracks.stop(timeTrackId: track.id)
let ongoing = try await client.timeTracks.getOngoing()  // nil when nothing is running
```

### Services

One handle per resource, all properties of the client: `attachments`, `boxes`, `bulkReplies`,
`calendarEvents`, `calendarPeriods`, `calendarTodos`, `calendars`, `clearances`, `clips`,
`collections`, `contacts`, `designations`, `entries`, `extenzions`, `folders`, `habits`,
`identity`, `journal`, `messages`, `postings`, `publications`, `search`, `snippets`,
`stickies`, `timeTracks`, `topics`, `workflows`; and `world`, for HEY World, which the model
has no route for.

Every method the model describes is generated, and named for the operation with the service's
noun dropped: `ListBoxes` is `client.boxes.list()`, `GetBoxPostingChanges` is
`client.postings.getBoxChanges(boxId:since:options:)`. Optional query parameters arrive in an
optional `<Operation>Options` struct
(`client.contacts.get(contactId: id, options: GetContactOptions(page: cursor))`); a request
body is the model's own request type. Operation ids, methods and paths are all in `Routes`.

On top of those, hand-written extensions of the generated services add conveniences for every
service the Go, Rust and Kotlin SDKs write them for, so they sit beside the generated methods
on the same handle. They take the arguments a caller has rather than a request body, and cover
the parts of HEY the model cannot describe: the browser forms that create a calendar event or
a workflow, publish a topic, upload an attachment, or post to HEY World, and the change feeds
a sync walks (`postings.changes`, `calendars.recordingChanges`), which answer a cursor to
resume from and say when HEY wants the box read in full. A hand-written method keeps the plain
name where the generated service leaves it free, and takes the model's own name for the
operation where it does not: `postings.markPostingsSeen(postingIds:)` alongside the generated
`postings.markSeen(body:)`. One that changes the shape of the call may take a descriptive name
instead: `timeTracks.startTracking()` names the conflict a running track answers with.

Every model is a `Codable`, `Sendable`, `Equatable` struct. Its required members are
non-optional, so a body that leaves one out fails to decode as a non-retryable `api_error`
rather than reading as a fabricated zero; its optional members are optionals, so an absent
member stays distinct from a present-but-empty one. Strings the model marks sensitive — email
addresses — are `SensitiveString`s that print as `[REDACTED]` and give up their value with
`expose()`. Two models are renamed away from names Swift already has: HEY's `Calendar` is
`HeyCalendar` and its `Collection` is `HeyCollection`, so importing `Hey` shadows neither
Foundation's calendar nor the standard library's protocol.

### Form-backed writes

Parts of HEY have no JSON surface: they are browser forms that answer a redirect. The calendar
event update is one, and the SDK covers it; for an endpoint nothing covers, `client.form` builds
the request — the path as written, a browser's `Accept`, the redirect captured rather than
followed, never retried — and `client.sendForm` sends it:

```swift
var operation = client.form(.post, "/workflows")
operation.info = writeInfo(service: "Workflows", operation: "CreateWorkflow", resourceType: "workflow")
operation.form([("workflow[name]", "Launch")])
let answer = try await client.sendForm(operation)
let workflowId = try answer.extractId()     // the rightmost number in the redirect's Location
```

The model describes none of those paths, so say what the call means with `writeInfo` before
sending it, or the hooks only hear that something raw went out.

### Pagination

A paginated read answers a `Page`: the value HEY answered, the cursor for the page after it,
and the `X-Total-Count` header when the read carried one. Nothing is followed on its own.

```swift
let first = try await client.contacts.list()
try await client.eachPage(first) { page in
    page.value.forEach { print($0.name ?? "") }
    return true                                          // false stops the walk
}
for try await page in client.pages(from: first) { /* ... */ }  // the same walk as a stream
let second = try await client.nextPage(first)            // one page on, or nil at the end
```

A `Link` pointing off the HEY origin — another host, or a downgrade to plain HTTP — is refused
rather than followed. A list walk that reaches the client's `maxPages` with pages still to read
fails rather than answering a shorter list that looks complete; a change-feed walk
(`allChanges`, `allCalendarChanges`, `allRecordingChanges`) stops there and names the page it
did not read in `nextPage`, which a complete answer never carries.

### Linked accounts

A root client presents mail from All Accounts. Derive one for a linked account to present that
account's mail and act as its user and default sender; it adds HEY's `filtered_account_id` to
every request on the HEY origin, the next pages of a walk included, except an attachment's
upload, whose URL authenticates itself and goes exactly as storage named it. It checks the
account against the identity first so a stale id fails on derivation rather than on the first
read.

```swift
let work = try await client.forAccount(42)
let boxes = try await work.boxes.list()
let sender = try await work.defaultSenderId()
```

Calendar, journal, habits and time tracking belong to the identity, so they read the same
through a scoped client.

## Errors

Every failure is a `HeyError`, an enum, so a `switch` over it is exhaustive: `usage`,
`notFound`, `auth`, `forbidden`, `rateLimit`, `network`, `api`, `validation`, `conflict`,
`ambiguous`. Each carries the category as `code` (the string every HEY SDK uses, `not_found`,
`rate_limit`), an `exitCode` for a command-line tool, the HTTP status when HEY answered,
whether the call `isRetryable`, the `X-Request-Id` HEY answered with, HEY's own message as the
`hint`, and the failure body up to a megabyte. An HTML error page is never echoed into a
message. Cancelling the task a call runs in cancels the call, its waits included, and the
cancellation is thrown as it arrived — a `CancellationError`, or the transport's `URLError` with
the `.cancelled` code — rather than as a `HeyError`.

```swift
do {
    _ = try await client.boxes.get(boxId: id)
} catch let HeyError.notFound(_, detail) {
    print("no such box: \(detail.requestId ?? "")")
} catch let error as HeyError {
    exit(Int32(error.exitCode))
}
```

## Retries

Each route carries the retry policy the model gives it: how many sends in all, which statuses
earn another, and the wait before the second. The client's own settings only make that
gentler — `maxRetries` caps the sends, `baseRetryDelay` holds the first wait up,
`maxRetryDelay` holds every wait down — so an operation the model calls non-idempotent is sent
once whatever the client says, and one whose policy allows two sends gets two however high the
client's ceiling. A `Retry-After` on a 429 is honoured as given. A 401 is answered by one
credential refresh and one resend, for any operation.

## Cache

Set `enableCache: true` on the `HeyConfig` and every JSON read goes out conditional on the
`ETag` it last saw, with a 304 answered from the cache under the headers the body was first read
with, so a revalidated page keeps its `Link` and `X-Total-Count`. An answer marked
`Cache-Control: no-store` is not held. Entries are held in memory, keyed by the URL and the
credential together, so one identity's reading is never answered to another; `cache:` on the
client swaps in a `ResponseCache` of your own.

## Hooks

`hooks:` on the client takes a `HeyHooks`: told when an operation starts and how it ended, when
each request goes out and what it answered, and about each resend before it is made. Several
sets go on as one with `ChainHooks`; `ConsoleHooks` prints them. An operation ends the way the
caller sees it end: a change feed's 409, handed back as a full-sync answer, is a success, and
the durations come from a monotonic clock.

```swift
let client = try HeyClient(
    accessToken: token,
    config: HeyConfig(enableCache: true, maxRetries: 2),
    hooks: ChainHooks(ConsoleHooks(), metrics)
)
```

## Install

Swift has no package registry to publish to: Swift Package Manager resolves the package straight
from this repository's `vX.Y.Z` tags and reads the `Package.swift` at its root, which builds the
`Hey` library alone. In Xcode, choose **File > Add Package Dependencies**, enter
`https://github.com/basecamp/hey-sdk`, and add the `Hey` product. In a `Package.swift`:

```swift
dependencies: [
    .package(url: "https://github.com/basecamp/hey-sdk", from: "0.31.0"),
],
targets: [
    .target(name: "YourApp", dependencies: [.product(name: "Hey", package: "hey-sdk")]),
]
```

On Linux the library uses `FoundationNetworking`, which ships with the Swift toolchain.
`make swift-consumer-check` is that path end to end: it tags a clone of the repository,
resolves the package from it by URL and version, and compiles the examples in this README
against it.

## Versioning

The library shares one version and one `vX.Y.Z` tag with the Go module, the Rust crate and the
Kotlin library, and follows a pre-1.0 policy: a breaking change bumps the minor
version and an additive one the patch; the default is append-only, but a minor may break source
compatibility to correct the model, and says so in the release notes. `from:` in a
`Package.swift` accepts every later version below the next major, so pin with
`.upToNextMinor(from:)` to take patches only while the SDK is pre-1.0. `HeyConfig.version` and
`HeyConfig.apiVersion` say which library and which API contract a client is working from, and go
out in every request's `User-Agent`.

## Development

```sh
make swift-generate        # regenerate swift/Sources/Hey/Generated
make swift-check           # build and tests with warnings as errors, generator and runner tests
make swift-check-drift     # fail if the generated tree is stale
make conformance-swift     # the shared fixtures under conformance/tests through the SDK
make swift-consumer-check  # resolve the package from a tag and compile this README's examples
```

Everything builds and runs on Linux as well as macOS. `swift/Package.swift` is the development
package: the library, the generator (`HeyGenerator`) and both test targets. The generator reads
`openapi.json`, `behavior-model.json` and `swift/names.toml`, where a method name the derivation
gets wrong, an operation filed under another service, or a schema renamed away from a Swift
collision is settled. The conformance runner in `conformance/runner/swift` depends on the
library through the root `Package.swift`, the manifest an app resolves.
