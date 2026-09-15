# HEY Kotlin SDK

The Kotlin client for the [HEY](https://www.hey.com) API. Models, routes and service methods
are generated from the Smithy model in the repository's `spec/` directory, so what the library
offers is what HEY serves. It is a Kotlin Multiplatform library with a JVM target, on Ktor and
kotlinx.serialization, laid out and behaving the way the
[basecamp-sdk](https://github.com/basecamp/basecamp-sdk) Kotlin SDK does.

```kotlin
dependencies {
    implementation("com.basecamp:hey-sdk:0.31.0")
}
```

The library is published to GitHub Packages, which wants a token for every download, public
packages included. See [Install](#install). Requires JDK 17 and Kotlin 2.3 or newer.

## Authenticate

A fixed token, for scripts and anything that already holds one:

```kotlin
import com.basecamp.hey.HeyClient

val client = HeyClient { accessToken(System.getenv("HEY_TOKEN")) }
```

An application that keeps OAuth tokens hands the client a `TokenProvider` over them: the
client asks it for the token on every request and asks it to `refresh()` once when HEY answers
401, then sends the request again. Anything that wants the request headers outright implements
`AuthStrategy` and passes it with `auth(strategy)`.

```kotlin
val client = HeyClient {
    accessToken(object : TokenProvider {
        override suspend fun accessToken() = store.accessToken
        override suspend fun refresh(): Boolean = store.renew()
    })
}
```

## Use it

```kotlin
import com.basecamp.hey.generated.*                // the service accessors: client.boxes, client.messages, ...
import com.basecamp.hey.services.*                 // MessageContent, ReplyContent, BoxKind, ...

val boxes = client.boxes.list()                    // a Page: .value is the list HEY answered
for (box in boxes.value) println("${box.name} (${box.kind})")
val imbox = client.boxes.getImbox()
println("${imbox.postings?.size} postings")

// Sending: recipients are required. HEY saves an unaddressed message as a draft.
client.messages.send(MessageContent(
    subject = "Subject",
    content = "<div>Body</div>",
    to = listOf("someone@example.com"),
))

// Replying: start from the prefill. It carries the subject, the acting sender and the
// recipients HEY resolved, which differ from the account default on shared addresses.
val prefill = client.entries.newReply(entryId)
client.entries.reply(entryId, ReplyContent(
    actingSenderId = prefill.sender?.id ?: 0,
    subject = prefill.subject.orEmpty(),
    content = "<div>Reply</div>",
    to = prefill.addressed?.directly.orEmpty().mapNotNull { it.emailAddress?.expose() },
))

// Postings are bulk operations, as they are in HEY. Moving by kind resolves the box index
// once per client.
client.postings.markPostingsSeen(listOf(a, b))
client.postings.moveToSetAside(listOf(a))
val trail = client.boxes.idByKind(BoxKind.PAPER_TRAIL)

// Calendar
val track = client.timeTracks.startTracking()
client.timeTracks.stop(track.id)
val ongoing = client.timeTracks.getOngoing()       // null when nothing is running
```

### Services

One handle per resource, all extension properties of the client in
`com.basecamp.hey.generated`: `attachments`, `boxes`,
`bulkReplies`, `calendarEvents`, `calendarPeriods`, `calendarTodos`, `calendars`,
`clearances`, `clips`, `collections`, `contacts`, `designations`, `entries`, `extenzions`,
`folders`, `habits`, `identity`, `journal`, `messages`, `postings`, `publications`,
`search`, `snippets`, `stickies`, `timeTracks`, `topics`, `workflows`; and `world`, in
`com.basecamp.hey.services`, for HEY World, which the model has no route for.

Every method the model describes is generated, and named for the operation with the
service's noun dropped: `ListBoxes` is `client.boxes.list()`, `GetBoxPostingChanges` is
`client.postings.getBoxChanges(..)`. Optional query parameters arrive in a nullable
`<Operation>Options` data class (`client.contacts.get(id, GetContactOptions(page = cursor))`);
a request body is the model's own request type from `com.basecamp.hey.generated.models`.
Operation ids, methods and paths are all in `com.basecamp.hey.generated.Routes`.

On top of those, `com.basecamp.hey.services` holds hand-written subclasses of the generated
services, one for every service the Go and Rust SDKs write conveniences for — `attachments`,
`boxes`, `bulkReplies`, `calendarEvents`, `calendarPeriods`, `calendarTodos`, `calendars`,
`clearances`, `clips`, `collections`, `contacts`, `designations`, `entries`, `extenzions`,
`habits`, `identity`, `journal`, `messages`, `postings`, `publications`, `search`, `snippets`,
`stickies`, `timeTracks`, `topics` and `workflows`, plus `WorldService` — which the accessors
hand out, so their conveniences sit beside the generated methods without a further import.
They take the arguments a caller has rather than a request body, and cover the parts of HEY
the model cannot describe: the browser forms that create a calendar event or a workflow,
publish a topic, upload an attachment, or post to HEY World, and the change feeds a sync
walks (`postings.changes`, `calendars.recordingChanges`), which answer a cursor to resume
from and say when HEY wants the box read in full. A hand-written method keeps the plain name
where the generated service leaves it free, and takes the model's own name for the operation
where it does not: `postings.markPostingsSeen(ids)` alongside the generated
`postings.markSeen(body)`. One that changes the shape of the call may take a descriptive
name instead: `timeTracks.startTracking()` names the conflict a running track answers with.

Every model is a `@Serializable` data class. Its required members come first, without
defaults, so a body that leaves one out fails to decode as a non-retryable `api_error` rather
than reading as a fabricated zero; its optional members are nullable and default to null, so
an absent member stays distinct from a present-but-empty one. Strings the model marks
sensitive — email addresses — are `SensitiveString`s that print as `[REDACTED]` and give up
their value with `expose()`.

### Form-backed writes

Parts of HEY have no JSON surface: they are browser forms that answer a redirect. The
calendar event update is one, and the SDK covers it; for an endpoint nothing covers,
`client.form` builds the request — the path as written, a browser's `Accept`, the redirect
captured rather than followed, never retried — and `client.sendForm` sends it:

```kotlin
val operation = client.form(Method.POST, "/workflows")
    .info(writeInfo("Workflows", "CreateWorkflow", "workflow"))
    .form(listOf("workflow[name]" to "Launch"))
val answer = client.sendForm(operation)
val workflowId = answer.extractId()      // the rightmost number in the redirect's Location
```

The model describes none of those paths, so say what the call means with `writeInfo` before
sending it, or the hooks only hear that something raw went out.

### Pagination

A paginated read answers a `Page`: the value HEY answered, the cursor for the page after it,
and the `X-Total-Count` header when the read carried one. Nothing is followed on its own.

```kotlin
val first = client.contacts.list()
client.eachPage(first) { page ->
    page.value.forEach { println(it.name) }
    true                                            // false stops the walk
}
client.pages(first).collect { page -> /* ... */ }  // the same walk as a Flow
val second = client.nextPage(first)                 // one page on, or null at the end
```

A `Link` pointing off the HEY origin — another host, or a downgrade to plain HTTP — is refused
rather than followed. A list walk that reaches the client's `maxPages` with pages still to
read fails rather than answering a shorter list that looks complete; a change-feed walk
(`allChanges`, `allCalendarChanges`, `allRecordingChanges`) stops there and names the page it
did not read in `nextPage`, which a complete answer never carries.

### Linked accounts

A root client presents mail from All Accounts. Derive one for a linked account to present that
account's mail and act as its user and default sender; it adds HEY's `filtered_account_id` to
every request on the HEY origin, the next pages of a walk included, and checks the account
against the identity first so a stale id fails on derivation rather than on the first read.

```kotlin
val work = client.forAccount(42)
val boxes = work.boxes.list()
val sender = work.defaultSenderId()
```

Calendar, journal, habits and time tracking belong to the identity, so they read the same
through a scoped client.

## Errors

Every failure is a `HeyException`, a sealed class, so a `when` over it is exhaustive:
`Usage`, `NotFound`, `Auth`, `Forbidden`, `RateLimit`, `Network`, `Api`, `Validation`,
`Conflict`, `Ambiguous`. Each carries the category as `code` (the string every HEY SDK uses,
`not_found`, `rate_limit`), an `exitCode` for a command-line tool, the HTTP status when HEY
answered, whether the call is `retryable`, the `X-Request-Id` HEY answered with, HEY's own
message as the `hint`, and the failure body up to a megabyte. An HTML error page is never
echoed into a message.

```kotlin
try {
    client.boxes.get(id)
} catch (e: HeyException.NotFound) {
    println("no such box: ${e.requestId}")
} catch (e: HeyException) {
    exitProcess(e.exitCode)
}
```

## Retries

Each route carries the retry policy the model gives it: how many sends in all, which statuses
earn another, and the wait before the second. The client's own settings only make that
gentler — `maxRetries` caps the sends, `baseRetryDelay` holds the first wait up,
`maxRetryDelay` holds every wait down — so an operation the model calls non-idempotent is sent once whatever the
client says, and one whose policy allows two sends gets two however high the client's ceiling.
A `Retry-After` on a 429 is honoured as given. A 401 is answered by one credential refresh and
one resend, for any operation.

## Cache

Set `enableCache = true` on the builder and every JSON read goes out conditional on the
`ETag` it last saw, with a 304 answered from the cache under the headers the body was first
read with, so a revalidated page keeps its `Link` and `X-Total-Count`. An answer marked
`Cache-Control: no-store` is not held. Entries are held in memory, keyed by the URL and the
credential together, so one identity's reading is never answered to another; `cache` on the
builder swaps in a `ResponseCache` of your own.

## Hooks

`hooks` on the builder takes a `HeyHooks`: told when an operation starts and how it ended,
when each request goes out and what it answered, and about each resend before it is made.
Several sets go on as one with `chainHooks`; `consoleHooks()` prints them. A hook that throws
is ignored. An operation ends the way the caller sees it end: a change feed's 409, handed
back as a full-sync answer, is a success, and the durations come from a monotonic clock.

## Install

The library is published to [GitHub Packages](https://github.com/basecamp/hey-sdk/packages),
which requires a token for every download, public packages included. Publishing is switched
off until it is sorted out (`.github/kotlin-publish-enabled`; see CONTRIBUTING.md), so until
the first release lands there, build from a checkout and consume it from your local Maven
repository, with no token at all:

```bash
cd kotlin && ./gradlew :hey-sdk:publishToMavenLocal
```

```kotlin
repositories {
    mavenLocal()
    mavenCentral()
}

dependencies {
    implementation("com.basecamp:hey-sdk:0.31.0")
}
```

`make kt-consumer-check` is that path end to end: it publishes to a scratch repository and
compiles a consumer against it. Once the library is on GitHub Packages, create a
[classic personal access token](https://github.com/settings/tokens) with the `read:packages`
scope, keep it in `~/.gradle/gradle.properties` as `gpr.user` and `gpr.key`, and declare the
repository:

```kotlin
repositories {
    mavenCentral()
    maven {
        url = uri("https://maven.pkg.github.com/basecamp/hey-sdk")
        credentials {
            username = project.findProperty("gpr.user") as String? ?: System.getenv("GITHUB_USER")
            password = project.findProperty("gpr.key") as String? ?: System.getenv("GITHUB_ACCESS_TOKEN")
        }
    }
}

dependencies {
    implementation("com.basecamp:hey-sdk:0.31.0")
}
```

`mavenCentral()` is for the library's own dependencies: Ktor, kotlinx.serialization and
kotlinx.coroutines. Kotlin 2.3 is the oldest compiler that can read what the jar and those
dependencies carry; `make kt-consumer-check` compiles a consumer with it, so the number here
is one the artifact keeps. Maven users depend on `com.basecamp:hey-sdk-jvm`, not `hey-sdk`: Gradle
reads the module metadata beside the root artifact and redirects to the JVM variant, Maven
does not, and the root jar it would resolve holds no classes. Declare a `<repository>` and a
`<server>` in `settings.xml` carrying the same token.

## Versioning

The library shares one version and one `vX.Y.Z` tag with the Go module and the Rust crate,
and follows basecamp-sdk's pre-1.0 policy: a breaking change bumps the minor version and an
additive one the patch; the default is append-only, but a minor may break source
compatibility to correct the model, and says so in the release notes. Binary compatibility
across versions is not promised — recompile against each release. `HeyConfig.VERSION` and
`HeyConfig.API_VERSION` say which library and which API contract a client is working from,
and go out in every request's `User-Agent`.

## Development

```sh
make kt-generate        # regenerate kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated
make kt-check           # build, unit tests, generator tests, runner tests
make kt-check-drift     # fail if the generated tree is stale
make conformance-kt     # the shared fixtures under conformance/tests through the SDK
```

The build wants JDK 17; `.mise.toml` at the repository root pins one for mise users. The
generator reads `openapi.json`, `behavior-model.json` and `kotlin/generator/names.toml`,
where a method name the derivation gets wrong, an operation filed under another service, or a
schema renamed away from a Kotlin collision is settled.
