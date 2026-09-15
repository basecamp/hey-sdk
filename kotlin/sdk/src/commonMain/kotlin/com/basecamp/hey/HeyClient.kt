package com.basecamp.hey

import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.Account
import com.basecamp.hey.generated.models.Identity
import io.ktor.client.HttpClient
import io.ktor.client.HttpClientConfig
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.client.request.prepareRequest
import io.ktor.client.request.request
import io.ktor.client.request.setBody
import io.ktor.client.request.url
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsChannel
import io.ktor.http.Headers
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpMethod
import io.ktor.http.URLBuilder
import io.ktor.http.Url
import io.ktor.http.encodedPath
import io.ktor.http.parseQueryString
import io.ktor.http.takeFrom
import io.ktor.utils.io.readAvailable
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.serializer
import kotlin.concurrent.Volatile
import kotlin.random.Random
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

/** The wait before the first resend of a path the model says nothing about; each one after doubles it. */
private val DEFAULT_BASE_DELAY: Duration = 1.seconds

/** The statuses a request the model says nothing about — a path the caller wrote — is resent on. */
private val RETRYABLE_STATUSES = listOf(429, 500, 502, 503, 504)
private const val ACCOUNT_FILTER_PARAMETER = "filtered_account_id"
private const val MAX_REDIRECTS = 10
private val REDIRECT_STATUSES = setOf(301, 302, 303, 307, 308)

/** The redirects a browser form answers a completed write with. A 301, 307 or 308 is a request to send it again, not an answer. */
private val FORM_ANSWER_STATUSES = setOf(302, 303)

/**
 * Builder DSL for configuring a [HeyClient].
 *
 * ```kotlin
 * val client = HeyClient {
 *     accessToken("your-token")
 *     userAgent = "my-app/1.0"
 *     enableCache = true
 *     hooks = consoleHooks()
 * }
 * ```
 */
class HeyClientBuilder {
    /** Token provider for authentication. Set via [accessToken]. */
    var tokenProvider: TokenProvider? = null

    /** Custom authentication strategy. Mutually exclusive with [tokenProvider]. */
    var authStrategy: AuthStrategy? = null

    /** Base URL for the API. Defaults to HEY itself. Plain HTTP is refused anywhere but this machine. */
    var baseUrl: String = HeyConfig.DEFAULT_BASE_URL

    /** User-Agent header. */
    var userAgent: String = HeyConfig.DEFAULT_USER_AGENT

    /** Enable ETag-based HTTP caching. [cache] chooses the store; without one, entries are kept in memory. */
    var enableCache: Boolean = false

    /** Enable automatic retry of transient failures. */
    var enableRetry: Boolean = true

    /**
     * The most times any operation is resent after a transient failure. A modelled route is
     * resent as many times as its own policy allows and no more; this only lowers that.
     */
    var maxRetries: Int = HeyConfig.DEFAULT_MAX_RETRIES

    /** Maximum pages a walk reads before it stops (safety cap). */
    var maxPages: Int = HeyConfig.DEFAULT_MAX_PAGES

    /** Request timeout. Use [Duration.INFINITE] to disable. */
    var timeout: Duration = HeyConfig.DEFAULT_TIMEOUT

    /** The least the client waits before the first resend. A route's own policy holds it up when longer. */
    var baseRetryDelay: Duration? = null

    /** The most the client waits between attempts, jitter included. The wait a `Retry-After` names is honoured as given. */
    var maxRetryDelay: Duration = HeyConfig.DEFAULT_MAX_RETRY_DELAY

    /** The most added at random to each wait. */
    var maxRetryJitter: Duration = HeyConfig.DEFAULT_MAX_RETRY_JITTER

    /** The most a JSON or HTML answer may deliver before the client refuses to hold it. */
    var maxResponseBodyBytes: Int = HeyConfig.DEFAULT_MAX_RESPONSE_BODY_BYTES

    /** The store [enableCache] reads and writes. Defaults to an [InMemoryCache]. */
    var cache: ResponseCache? = null

    /** Observability hooks. */
    var hooks: HeyHooks = NoopHooks

    /**
     * Custom Ktor [HttpClientEngine] (e.g., for testing with MockEngine). The client built
     * on it is the SDK's own: unlike basecamp-sdk there is no way to hand over a configured
     * [HttpClient], since a plugin on one — a retry, a default request, redirect following,
     * response validation — would run ahead of the retry policy, the credential handling on
     * redirects, the error mapping and the timeout this client is responsible for.
     */
    var engine: HttpClientEngine? = null

    /** Set a static access token. */
    fun accessToken(token: String) {
        tokenProvider = StaticTokenProvider(token)
    }

    /** Set a dynamic access token provider, which is also asked to refresh after a 401. */
    fun accessToken(provider: TokenProvider) {
        tokenProvider = provider
    }

    /** Set a custom authentication strategy. */
    fun auth(strategy: AuthStrategy) {
        authStrategy = strategy
    }

    internal fun build(): HeyClient {
        if (tokenProvider != null && authStrategy != null) {
            throw HeyException.Usage("Cannot set both accessToken and auth. Use one or the other.")
        }
        if (timeout != Duration.INFINITE && !timeout.isPositive()) {
            throw HeyException.Usage("timeout must be positive or Duration.INFINITE, got: $timeout")
        }
        val resolvedAuth = authStrategy
            ?: tokenProvider?.let { BearerAuth(it) }
            ?: throw HeyException.Usage("Authentication must be configured. Use accessToken(\"token\") or auth(strategy).")
        val parsed = parseAbsoluteUrl(baseUrl) ?: throw HeyException.Usage("Invalid base URL: $baseUrl")
        requireSecureEndpoint(parsed)

        val config = try {
            HeyConfig(
                baseUrl = baseUrl,
                userAgent = userAgent,
                enableCache = enableCache,
                enableRetry = enableRetry,
                timeout = timeout,
                maxRetries = maxRetries,
                maxPages = maxPages,
                baseRetryDelay = baseRetryDelay,
                maxRetryDelay = maxRetryDelay,
                maxRetryJitter = maxRetryJitter,
                maxResponseBodyBytes = if (maxResponseBodyBytes <= 0) HeyConfig.DEFAULT_MAX_RESPONSE_BODY_BYTES else maxResponseBodyBytes,
            )
        } catch (error: IllegalArgumentException) {
            throw HeyException.Usage(error.message ?: "invalid configuration")
        }

        val http = engine?.let { HttpClient(it) { configure(timeout) } } ?: HttpClient { configure(timeout) }
        val shared = Shared(
            config = config,
            baseUrl = parsed,
            http = http,
            auth = resolvedAuth,
            cache = if (enableCache) cache ?: InMemoryCache() else null,
            hooks = hooks,
        )
        return HeyClient(shared, null, ScopeState(), root = true)
    }

    private fun HttpClientConfig<*>.configure(timeout: Duration) {
        expectSuccess = false
        followRedirects = false
        if (timeout.isFinite()) {
            install(HttpTimeout) {
                requestTimeoutMillis = timeout.inWholeMilliseconds
                connectTimeoutMillis = timeout.inWholeMilliseconds
                socketTimeoutMillis = timeout.inWholeMilliseconds
            }
        }
    }
}

/**
 * Creates a [HeyClient] using the builder DSL.
 *
 * ```kotlin
 * val client = HeyClient { accessToken("your-token") }
 * val boxes = client.boxes.list()
 * ```
 */
fun HeyClient(block: HeyClientBuilder.() -> Unit): HeyClient = HeyClientBuilder().apply(block).build()

internal class Shared(
    val config: HeyConfig,
    val baseUrl: Url,
    val http: HttpClient,
    val auth: AuthStrategy,
    val cache: ResponseCache?,
    val hooks: HeyHooks,
) {
    /** How many times the credentials have been refreshed, so a 401 answered after someone else refreshed is resent rather than refreshed again. */
    @Volatile
    var refreshes: Long = 0

    /** One refresh at a time. */
    val refreshing = Mutex()

    /** The refresh in flight, for every stale request to wait on; null between refreshes. */
    var refresh: Deferred<Boolean>? = null

    /** Where the client runs what outlives a request: a refresh. Cancelled when the root client closes. */
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
}

/**
 * What a client works out about the identity it presents and keeps for as long as it lives.
 * A client derived with [HeyClient.forAccount] starts an empty one of its own.
 */
internal class ScopeState {
    val lock = Mutex()
    var defaultSenderId: Long? = null
    var accountUserId: Long? = null
    var boxKinds: Map<String, Long>? = null
}

/**
 * Root client for the HEY API: one authenticated identity, presenting mail from All Accounts
 * unless derived for one linked account with [forAccount].
 *
 * Every service the model describes is an extension property in `com.basecamp.hey.generated`
 * — `client.boxes`, `client.messages`, `client.timeTracks` — created on first use and cached
 * for the lifetime of the client. Thread-safe after construction.
 *
 * ```kotlin
 * import com.basecamp.hey.generated.boxes
 *
 * val client = HeyClient { accessToken("your-token") }
 * val boxes = client.boxes.list()
 * ```
 */
class HeyClient internal constructor(
    internal val shared: Shared,
    /** The linked account this client presents, or null for All Accounts. */
    val accountId: Long?,
    internal val scope: ScopeState,
    /** Whether this is the client the builder made, which owns the transport; a client derived from it does not. */
    private val root: Boolean,
) {
    @PublishedApi
    internal val serviceCache: MutableMap<String, Any> = createServiceCache()

    /** The configuration this client was built from. */
    val config: HeyConfig get() = shared.config

    /** Where HEY is. */
    val baseUrl: Url get() = shared.baseUrl

    /** The hooks this client reports to. */
    val hooks: HeyHooks get() = shared.hooks

    /**
     * Gets or creates a cached service instance. This is the extension point for external
     * modules to add services without subclassing:
     * ```kotlin
     * val HeyClient.customService: CustomService
     *     get() = service("custom") { CustomService(this) }
     * ```
     */
    inline fun <reified T : Any> service(key: String, crossinline factory: () -> T): T =
        @Suppress("UNCHECKED_CAST")
        (serviceCache.getOrCreate(key) { factory() } as T)

    /** Starts a request for one of the modelled routes. Generated service methods call this. */
    fun operation(route: Route, params: List<Any>): Operation = Operation.forRoute(route, params)

    /**
     * Starts a request for a path the model does not cover. The path is relative to the base
     * URL and gets the same credentials, `.json` suffix, account scope and retry treatment as
     * a modelled one.
     */
    fun request(method: Method, path: String): Operation = Operation.raw(method, path)

    /**
     * A request to one of the endpoints HEY serves only as a browser form: the path as the
     * caller wrote it, a browser's `Accept`, and the redirect taken for the answer rather than
     * followed. It is not retried, whatever its method; a 401 is the exception, and is
     * answered by a credential refresh and one resend like every other request.
     *
     * The model describes none of these paths, so say what the call means with
     * [Operation.info] and [writeInfo] before sending it.
     */
    fun form(method: Method, path: String): Operation =
        request(method, path).formRepresentation().captureRedirects().idempotent(false)

    /** Sends an operation and decodes its JSON body. */
    suspend fun <T> send(operation: Operation, deserializer: DeserializationStrategy<T>): T {
        val label = operation.label()
        return execute(operation) { response -> decode(response, deserializer, label) }
    }

    /** Sends an operation and decodes its JSON body as [T]. */
    suspend inline fun <reified T> send(operation: Operation): T = send(operation, serializer<T>())

    /** Sends an operation whose answer carries no body worth reading. */
    suspend fun sendUnit(operation: Operation) {
        execute(operation)
    }

    /** Sends an operation and reads its body as text: the HTML page a route serves no JSON for. */
    suspend fun sendText(operation: Operation): String = execute(operation) { it.text() }

    /** Sends an operation that answers a status meaning "nothing there" with null. */
    suspend fun <T> sendOptional(operation: Operation, deserializer: DeserializationStrategy<T>): T? {
        val label = operation.label()
        return execute(operation) { response -> if (response.empty) null else decode(response, deserializer, label) }
    }

    /** Sends an operation that answers a status meaning "nothing there" with null. */
    suspend inline fun <reified T> sendOptional(operation: Operation): T? = sendOptional(operation, serializer<T>())

    /** Sends a paginated read and keeps the cursor HEY answered with, and the route, so the next page is read under the same policy. */
    suspend fun <T> sendPage(operation: Operation, deserializer: DeserializationStrategy<T>): Page<T> {
        val label = operation.label()
        val info = operation.info
        val route = operation.route
        val noCache = operation.noCache
        return execute(operation) { response -> Page.of(decode(response, deserializer, label), response, shared.baseUrl, info, route, noCache, deserializer) }
    }

    /** Sends a paginated read and keeps the cursor HEY answered with. */
    suspend inline fun <reified T> sendPage(operation: Operation): Page<T> = sendPage(operation, serializer<T>())

    /** Sends a form request and reads the redirect it answered with. */
    suspend fun sendForm(operation: Operation): FormResponse = execute(operation) { FormResponse.of(it) }

    private fun <T> decode(response: Response, deserializer: DeserializationStrategy<T>, label: String): T =
        try {
            response.json(deserializer)
        } catch (error: HeyException.Api) {
            throw HeyException.Api(
                "$label: ${error.message}",
                httpStatus = error.httpStatus,
                hint = error.hint,
                retryable = false,
                requestId = error.requestId,
                cause = error.cause,
            )
        }

    /**
     * Sends an operation: applies credentials and account scope, retries transient failures
     * when the operation is idempotent, resends once after a refreshed 401, and answers a
     * cached body on 304. Non-2xx statuses become errors unless the operation treats them as
     * empty.
     */
    suspend fun execute(operation: Operation): Response = execute(operation) { it }

    /**
     * Sends an operation and reads its answer with [transform] — a decode, a parse — inside
     * the operation the hooks hear, so an answer that will not read ends the operation with
     * the error the caller gets, and its duration counts the reading.
     */
    suspend fun <T> execute(operation: Operation, transform: (Response) -> T): T {
        if (operation.quiet) return transform(dispatch(operation))
        val hooks = shared.hooks
        val info = operation.info
        val started = currentTimeMillis()
        hooks.safeOperationStart(info)
        try {
            val value = transform(dispatch(operation))
            hooks.safeOperationEnd(info, OperationResult(elapsedSince(started)))
            return value
        } catch (error: Throwable) {
            // Whatever ended the operation — an error from HEY, a cancellation, a token
            // provider that threw — the hooks hear the end of what they heard the start of.
            hooks.safeOperationEnd(info, OperationResult(elapsedSince(started), reported(error, "operation cancelled")))
            throw error
        }
    }

    /** What the hooks are told an operation or request failed with: a cancellation is named as one. */
    private fun reported(error: Throwable, cancelled: String): Throwable =
        if (error is CancellationException) HeyException.Network(cancelled, retryable = false) else error

    private fun elapsedSince(started: Long): Duration = (currentTimeMillis() - started).milliseconds

    private suspend fun dispatch(operation: Operation): Response {
        val url = urlFor(operation)
        val answered = attempt(operation, url)
        val outcome = runCatching { finish(operation, answered) }
        shared.hooks.safeRequestEnd(
            answered.info,
            RequestResult(
                statusCode = answered.status,
                duration = answered.duration,
                fromCache = outcome.getOrNull()?.fromCache ?: false,
                error = outcome.exceptionOrNull(),
            ),
        )
        return outcome.getOrThrow()
    }

    /**
     * What the retry loop may spend on one operation. A modelled route brings its own policy
     * from the model; the client's settings only make that gentler. A path the caller wrote
     * runs on the client's settings alone. Whatever the policy, an operation that is not
     * idempotent is sent once, and so is everything when retries are off.
     */
    private fun budget(operation: Operation): Budget {
        val config = shared.config
        val ceiling = config.maxRetries + 1
        val policy = operation.route?.retry
        val (attempts, retryOn, delay) = when {
            policy != null && policy.max > 0 -> Triple(
                minOf(policy.max, ceiling),
                policy.retryOn,
                maxOf(policy.baseDelayMs.milliseconds, config.baseRetryDelay ?: Duration.ZERO),
            )
            policy != null -> Triple(1, emptyList(), DEFAULT_BASE_DELAY)
            else -> Triple(ceiling, RETRYABLE_STATUSES, config.baseRetryDelay ?: DEFAULT_BASE_DELAY)
        }
        return Budget(
            attempts = if (operation.idempotent && config.enableRetry) attempts else 1,
            retryOn = retryOn,
            delay = minOf(delay, config.maxRetryDelay),
        )
    }

    private class Budget(val attempts: Int, val retryOn: List<Int>, val delay: Duration)

    private class Answered(
        val received: Received,
        val cached: Pair<String, CachedResponse>?,
        val info: RequestInfo,
        val duration: Duration,
    ) {
        val status: Int get() = received.status
    }

    /**
     * An answer read whole while the connection was still live, which is all the SDK keeps
     * of a response: the status and headers, the body up to its bound, and the refusal when
     * the body ran past it, so the transport can be done with the response before the retry
     * loop looks at any of it.
     */
    private class Received(
        val url: Url,
        val status: Int,
        val headers: Headers,
        val body: ByteArray,
        val refusal: HeyException?,
        /** Whether a redirect was followed to get here: the answer is then another resource's, not the one the cache entry was for. */
        val redirected: Boolean,
        /** Whether the request this answers went out with the credentials: a hop to another origin drops them, and they do not come back. */
        val authenticated: Boolean,
        /** How many refreshes had happened when the request this answers was signed — the last signing, when a hop was signed again. */
        val signedUnder: Long,
    )

    /** The auth strategy failed to sign a hop; the failure is passed on as the strategy threw it. */
    private class Unsigned(val failure: Throwable) : RuntimeException(failure)

    /** What one send came back with: the answer, or the place a redirect points. */
    private sealed class Outcome {
        class Answer(val received: Received) : Outcome()
        class Redirect(val next: Url, val status: Int) : Outcome()
    }

    /** Sends the operation as many times as its retry budget and HEY's answers call for, and hands back the answer it stopped on. */
    private suspend fun attempt(operation: Operation, url: Url): Answered {
        val hooks = shared.hooks
        val budget = budget(operation)
        var attempts = budget.attempts
        var attempt = 1
        var delay = budget.delay
        var refreshed = false
        var cached: Pair<String, CachedResponse>? = null

        while (true) {
            val prepared = prepare(operation, url, cached)
            cached = prepared.cached
            val info = RequestInfo(operation.method.name, url.toString(), attempt)
            hooks.safeRequestStart(info)
            val started = currentTimeMillis()
            val sent = try {
                Result.success(transmit(operation, url, prepared))
            } catch (error: CancellationException) {
                hooks.safeRequestEnd(info, RequestResult(0, elapsedSince(started), error = reported(error, "request cancelled")))
                throw error
            } catch (unsigned: Unsigned) {
                // The strategy could not sign a hop: its failure, as it would be on the
                // first request, not a network failure a resend would repeat.
                hooks.safeRequestEnd(info, RequestResult(0, elapsedSince(started), error = unsigned.failure))
                throw unsigned.failure
            } catch (error: HeyException) {
                Result.failure(error)
            } catch (error: Exception) {
                Result.failure(networkFailure(error))
            }
            val duration = elapsedSince(started)

            val failure = sent.exceptionOrNull()
            if (failure != null) {
                val error = failure as HeyException
                hooks.safeRequestEnd(info, RequestResult(0, duration, error = error))
                if (error.retryable && attempt < attempts) {
                    val wait = waitFor(delay)
                    hooks.safeRetry(info, attempt + 1, error, wait.inWholeMilliseconds)
                    delay(wait)
                    delay = nextDelay(delay)
                    attempt += 1
                    continue
                }
                throw error
            }

            val received = sent.getOrThrow()
            val status = received.status
            val retryable = status in budget.retryOn
            // A 401 from a hop that carried no credentials rejected none of HEY's: there is
            // nothing to refresh, and nothing a resend would change.
            val renewed = status == 401 && received.authenticated && !refreshed && try {
                refreshCredentials(received.signedUnder)
            } catch (error: Throwable) {
                hooks.safeRequestEnd(info, RequestResult(status, duration, error = reported(error, "request cancelled")))
                throw error
            }
            if (renewed) {
                val cause = HeyException.Auth("Token refreshed")
                hooks.safeRequestEnd(info, RequestResult(status, duration, error = cause))
                hooks.safeRetry(info, attempt + 1, cause, 0)
                refreshed = true
                attempt += 1
                attempts = maxOf(attempts, attempt)
                continue
            }
            if (retryable && attempt < attempts) {
                val cause = HeyException.fromResponse(status, operation.method, received.headers, ByteArray(0))
                val retryAfter = if (status == 429) retryAfterSeconds(received.headers["Retry-After"]) else null
                val wait = if (retryAfter != null && retryAfter > 0) retryAfter.seconds else waitFor(delay)
                hooks.safeRequestEnd(info, RequestResult(status, duration, error = cause))
                hooks.safeRetry(info, attempt + 1, cause, wait.inWholeMilliseconds)
                delay(wait)
                delay = nextDelay(delay)
                attempt += 1
                continue
            }
            // An answer reached through a redirect is another resource's: the entry looked up
            // for the URL asked for neither satisfies its 304 nor takes its body.
            return Answered(received, cached.takeUnless { received.redirected }, info, duration)
        }
    }

    /**
     * A transport failure as the SDK reports it: what went wrong, with any URL the transport
     * quoted cut back to its origin, since a redirect target can carry a signed query. The
     * transport's own exception is not kept, for the same reason. A timeout is as retryable
     * as any other failure to get an answer; the operation's idempotency and its budget say
     * whether it is resent.
     */
    private fun networkFailure(error: Exception): HeyException =
        HeyException.Network(
            "Network error",
            hint = HeyException.truncateMessage(redactUrls(error.message ?: error::class.simpleName ?: "unknown")),
            retryable = true,
        )

    /**
     * Answers a 401 with fresh credentials, once for all the requests the stale ones earned
     * it on. Refreshes go one at a time, and a request that was signed before the last
     * refresh is simply resent: the credentials it will pick up are already the new ones.
     *
     * The refresh runs in the client's own scope rather than the request's, so a request
     * cancelled while waiting for it leaves it running: a refresh half done is a rotated
     * token nobody holds, and every other stale request is waiting on the same one.
     */
    private suspend fun refreshCredentials(signedUnder: Long): Boolean {
        val refresh = shared.refreshing.withLock {
            if (shared.refreshes != signedUnder) return true
            shared.refresh ?: shared.scope.async {
                // Under the lock for its whole run, so no request is signed while the
                // credentials are changing hands, and the count moves with them.
                shared.refreshing.withLock {
                    try {
                        val renewed = shared.auth.refresh()
                        if (renewed) shared.refreshes += 1
                        renewed
                    } finally {
                        shared.refresh = null
                    }
                }
            }.also { shared.refresh = it }
        }
        return refresh.await()
    }

    internal fun urlFor(operation: Operation): Url {
        val builder = operation.url?.let { URLBuilder(it) } ?: run {
            val (path, query) = operation.path.split('?', limit = 2).let { it[0] to it.getOrNull(1) }
            val full = if (operation.jsonSuffix) withJsonExtension(path) else path
            URLBuilder(shared.baseUrl).apply {
                encodedPath = shared.baseUrl.encodedPath.trimEnd('/') + "/" + full.trimStart('/')
                // The caller's query goes out as written; decoding it here would turn a %26 into a second parameter.
                if (!query.isNullOrEmpty()) encodedParameters.appendAll(parseQueryString(query, decode = false))
            }
        }
        for ((name, value) in operation.query) builder.parameters.append(name, value)
        return scoped(builder.build())
    }

    /**
     * The URL with the client's account scope on it, when it has one and the URL is HEY's:
     * whatever the URL carried for the filter already, the scope wins, on the first request
     * and on every redirect that stays on the origin.
     */
    private fun scoped(url: Url): Url {
        val accountId = accountId ?: return url
        if (!isSameOrigin(url, shared.baseUrl)) return url
        return URLBuilder(url).apply {
            parameters.remove(ACCOUNT_FILTER_PARAMETER)
            parameters.append(ACCOUNT_FILTER_PARAMETER, accountId.toString())
        }.build()
    }

    private class Prepared(
        val request: HttpRequestBuilder,
        val cached: Pair<String, CachedResponse>?,
        /** What the auth strategy put on the request: the headers a hop to another origin must not carry, and what partitions the cache. */
        val credentials: Credentials,
        /** How many refreshes had happened when the request was signed, so a 401 knows whether its credentials are already stale. */
        val signedUnder: Long,
    )

    /** The headers an auth strategy added or changed on a request, by lowercased name, with their values. */
    private class Credentials(val headers: Map<String, List<String>>) {
        val names: Set<String> get() = headers.keys

        /**
         * What partitions the cache: every credential header, canonically ordered, so a
         * cookie-signed identity is kept apart from a bearer-signed one and two identities
         * that share a bearer but differ in another header are kept apart too. Empty when
         * the strategy set nothing, in which case nothing is cached.
         */
        val partition: String? get() =
            headers.entries.sortedBy { it.key }.joinToString("\n") { (name, values) -> "$name: ${values.joinToString(", ")}" }.ifEmpty { null }
    }

    /**
     * Signs a request with the auth strategy, under the refresh lock so no refresh lands
     * between the signing and the count that says which credentials went out, and answers
     * what the strategy put on it. A hop that stays on the origin is signed again for its
     * own URL and method, since a strategy may sign those, once the previous signature is
     * off; a hop to another origin is never signed.
     */
    private suspend fun sign(request: HttpRequestBuilder): Pair<Credentials, Long> {
        val before = request.headers.build()
        val signedUnder = shared.refreshing.withLock {
            shared.auth.authenticate(request)
            shared.refreshes
        }
        val added = request.headers.names()
            .filter { name -> request.headers.getAll(name) != before.getAll(name) }
            .associate { name -> name.lowercase() to request.headers.getAll(name).orEmpty().toList() }
        return Credentials(added) to signedUnder
    }

    /** Builds the request for one attempt, and looks the response cache up the first time it is asked for a key. */
    private suspend fun prepare(operation: Operation, url: Url, previous: Pair<String, CachedResponse>?): Prepared {
        val request = HttpRequestBuilder()
        request.method = operation.method.ktor()
        request.url(url)
        request.header(HttpHeaders.UserAgent, shared.config.userAgent)
        request.header(HttpHeaders.Accept, operation.accept)
        operation.body?.let { body ->
            request.header(HttpHeaders.ContentType, body.contentType)
            request.setBody(body.bytes)
        }
        val (credentials, signedUnder) = sign(request)

        val cache = cacheFor(operation)
        val partition = credentials.partition
        var cached = previous
        if (cache != null && partition != null) {
            val key = cacheKey(url.toString(), partition)
            if (cached == null || cached.first != key) cached = lookUp(cache, key)
        } else {
            cached = null
        }
        cached?.second?.etag?.takeIf { it.isNotEmpty() }?.let { request.header(HttpHeaders.IfNoneMatch, it) }
        return Prepared(request, cached, credentials, signedUnder)
    }

    private fun lookUp(cache: ResponseCache, key: String): Pair<String, CachedResponse> {
        val entry = cache.get(key)
        return when {
            entry == null -> key to CachedResponse("", ByteArray(0))
            entry.body.size <= shared.config.maxResponseBodyBytes -> key to entry
            else -> {
                cache.invalidate(key)
                key to CachedResponse("", ByteArray(0))
            }
        }
    }

    /** The cache the operation reads and writes, when there is one to use. Only a JSON GET is cached. */
    private fun cacheFor(operation: Operation): ResponseCache? =
        if (!operation.noCache && operation.method == Method.GET && operation.accept == "application/json") shared.cache else null

    /**
     * Sends one request and follows the redirects it is answered with, up to [MAX_REDIRECTS]
     * hops, unless the operation is one that takes the redirect for its answer. Credentials
     * stay on the origin they were meant for: a hop to another origin goes out without the
     * headers the auth strategy set, whatever it called them, and without the usual suspects.
     * A hop that stays on HEY keeps the client's account scope.
     *
     * Each response is read through Ktor's streaming form, so the body is bounded while it
     * is still arriving rather than after the transport has held all of it.
     */
    private suspend fun transmit(operation: Operation, start: Url, prepared: Prepared): Received {
        var url = start
        var request = prepared.request
        var credentials = prepared.credentials
        var signedUnder = prepared.signedUnder
        var hops = 0
        var authenticated = true
        while (true) {
            val outcome = shared.http.prepareRequest(request).execute { response ->
                val next = if (operation.captureRedirects) null else redirectTarget(url, response)
                if (next != null) Outcome.Redirect(next, response.status.value) else Outcome.Answer(receive(operation, url, response, hops > 0, authenticated, signedUnder))
            }
            when (outcome) {
                is Outcome.Answer -> return outcome.received
                is Outcome.Redirect -> {
                    if (hops == MAX_REDIRECTS) {
                        throw HeyException.Network("${operation.label()} redirected more than $MAX_REDIRECTS times", retryable = false)
                    }
                    val next = scoped(outcome.next)
                    requireSecureEndpoint(next)
                    val sameOrigin = isSameOrigin(url, next)
                    if (!sameOrigin) authenticated = false
                    request = redirected(request, outcome.status, url, next, credentials.names)
                    // The signature was for the URL and method the hop left behind; on the
                    // origin it is made again for the ones it goes to, and off it never is. The
                    // count moves with it: a 401 on the hop is about the credentials it carried.
                    if (sameOrigin && authenticated) {
                        val (signed, count) = try {
                            sign(request)
                        } catch (error: CancellationException) {
                            throw error
                        } catch (error: Throwable) {
                            throw Unsigned(error)
                        }
                        credentials = signed
                        signedUnder = count
                    }
                    url = next
                    hops += 1
                }
            }
        }
    }

    /** Reads what the SDK keeps of a response while the connection is live: a 304 has no body to read, and a body past its bound is refused there and then. */
    private suspend fun receive(operation: Operation, url: Url, response: HttpResponse, redirected: Boolean, authenticated: Boolean, signedUnder: Long): Received {
        val status = response.status.value
        val headers = response.headers
        if (status == 304) return Received(url, status, headers, ByteArray(0), refusal = null, redirected, authenticated, signedUnder)
        val bound = if (isParsed(operation.accept)) shared.config.maxResponseBodyBytes else HeyConfig.MAX_RESPONSE_BODY_BYTES
        return try {
            Received(url, status, headers, readBody(response, bound), refusal = null, redirected, authenticated, signedUnder)
        } catch (refusal: HeyException) {
            Received(url, status, headers, ByteArray(0), refusal, redirected, authenticated, signedUnder)
        }
    }

    private fun redirectTarget(url: Url, response: HttpResponse): Url? {
        if (response.status.value !in REDIRECT_STATUSES) return null
        val location = response.headers[HttpHeaders.Location] ?: return null
        return resolveReference(url, location)
    }

    /**
     * The request for the hop: the outgoing one's headers less the validator, less the
     * strategy's own headers (they are put back by a fresh signing when the hop stays on the
     * origin), and less anything credential-like when it does not.
     */
    private fun redirected(outgoing: HttpRequestBuilder, status: Int, from: Url, to: Url, credentialHeaders: Set<String>): HttpRequestBuilder {
        val request = HttpRequestBuilder()
        val keepBody = status == 307 || status == 308 || outgoing.method == HttpMethod.Get || outgoing.method == HttpMethod.Head
        request.method = if (keepBody) outgoing.method else HttpMethod.Get
        request.url(to)
        val sameOrigin = isSameOrigin(from, to)
        outgoing.headers.entries().forEach { (name, values) ->
            // The validator was the resource asked for's; the one pointed to has its own.
            if (name.equals(HttpHeaders.IfNoneMatch, true)) return@forEach
            if (name.lowercase() in credentialHeaders) return@forEach
            if (!sameOrigin && isSensitiveHeader(name)) return@forEach
            if (!keepBody && (name.equals(HttpHeaders.ContentType, true) || name.equals(HttpHeaders.ContentLength, true))) return@forEach
            values.forEach { request.headers.append(name, it) }
        }
        if (keepBody) request.setBody(outgoing.body)
        return request
    }

    private fun finish(operation: Operation, answered: Answered): Response {
        val received = answered.received
        val status = received.status
        val headers = received.headers
        if (status == 304) {
            val (key, entry) = answered.cached ?: (null to null)
            if (entry == null || entry.etag.isEmpty()) {
                throw HeyException.Api("304 received but no cached response available", httpStatus = 304, retryable = false)
            }
            // The 304 answers with the cached body under the cached headers, updated by what
            // it carried, and the entry keeps that update: a validator or cursor HEY moved
            // is what the next read goes out with, and a no-store on the 304 ends the entry.
            val merged = entry.headersUpdatedBy(headers)
            val cache = shared.cache
            if (cache != null && key != null) {
                if (forbidsStoring(merged)) {
                    cache.invalidate(key)
                } else {
                    cache.set(key, CachedResponse(merged[HttpHeaders.ETag] ?: entry.etag, entry.body, storableHeaders(merged)))
                }
            }
            // The caller gets a copy: the entry's bytes are the cache's, and a Response's body is the caller's to do with as it likes.
            return Response(200, merged, entry.body.copyOf(), received.url, fromCache = true, empty = false)
        }
        received.refusal?.let { refusal ->
            if (status in 200..299) throw refusal
            val error = HeyException.fromResponse(status, operation.method, headers, ByteArray(0))
            throw HeyException.Api(
                error.message ?: "API error: $status",
                httpStatus = status,
                hint = refusal.message,
                retryable = error.retryable,
                requestId = error.requestId,
                cause = refusal,
                responseTooLarge = true,
            )
        }
        val body = received.body
        if (status in 200..299) {
            val key = answered.cached?.first
            val cache = shared.cache
            if (key != null && cache != null) {
                val etag = headers[HttpHeaders.ETag]
                when {
                    // An answer HEY says not to keep is not kept, and neither is what it replaced.
                    forbidsStoring(headers) -> cache.invalidate(key)
                    etag != null && body.isNotEmpty() -> cache.set(key, CachedResponse(etag, body.copyOf(), storableHeaders(headers)))
                }
            }
            return Response(status, headers, body, received.url, fromCache = false, empty = false)
        }
        if (status in operation.emptyOn || (operation.captureRedirects && status in FORM_ANSWER_STATUSES)) {
            return Response(status, headers, body, received.url, fromCache = false, empty = true)
        }
        if (operation.captureRedirects && status in REDIRECT_STATUSES) {
            throw HeyException.Api(
                "${operation.label()} answered $status; a form write completes with a 302 or 303, and a $status asks for the request again",
                httpStatus = status,
                retryable = false,
                requestId = headers["X-Request-Id"],
            )
        }
        throw HeyException.fromResponse(status, operation.method, headers, body)
    }

    private suspend fun readBody(response: HttpResponse, bound: Int): ByteArray {
        val channel = response.bodyAsChannel()
        var out = ByteArray(16 * 1024)
        var size = 0
        while (true) {
            if (size == out.size) out = out.copyOf(out.size * 2)
            val read = channel.readAvailable(out, size, out.size - size)
            if (read < 0) break
            size += read
            if (size > bound) {
                val refusal = HeyException.Api("response body exceeds $bound bytes", httpStatus = null, retryable = false, responseTooLarge = true)
                // Let the transport go of the rest right here, rather than leaving a body it
                // will never be read for the connection's cleanup to drain or wait on.
                channel.cancel(refusal)
                throw refusal
            }
        }
        return out.copyOf(size)
    }

    private fun waitFor(delay: Duration): Duration {
        val jitter = shared.config.maxRetryJitter
        val added = if (jitter.isPositive()) Random.nextLong(jitter.inWholeMilliseconds + 1).milliseconds else Duration.ZERO
        return minOf(delay + added, shared.config.maxRetryDelay)
    }

    private fun nextDelay(delay: Duration): Duration = minOf(delay * 2, shared.config.maxRetryDelay)

    /**
     * Derives a client that presents mail from one linked account and acts as that account's
     * user and default sender. The account is checked against the identity first, so a stale
     * or foreign id fails here rather than on the first read.
     *
     * Calendar, journal, habits and time tracking belong to the identity, so they read the
     * same through a scoped client.
     */
    suspend fun forAccount(accountId: Long): HeyClient {
        if (accountId <= 0) throw HeyException.Usage("account id must be positive")
        val unscoped = HeyClient(shared, null, ScopeState(), root = false)
        val identity = unscoped.identity()
        val accessible = identity.accounts.orEmpty().any { it.id == accountId && accountIsAccessible(it) }
        if (!accessible) throw HeyException.NotFound("accessible account not found: $accountId")
        val scope = ScopeState()
        scope.defaultSenderId = defaultSenderFor(identity, accountId)
        scope.accountUserId = identity.allUsers.orEmpty().firstOrNull { it.accountId == accountId }?.id
        return HeyClient(shared, accountId, scope, root = false)
    }

    /**
     * The sender a message goes out as when the caller names none: the scoped account's
     * default sender, or the identity's default sender for All Accounts.
     */
    suspend fun defaultSenderId(): Long = scope.lock.withLock {
        scope.defaultSenderId?.let { return it }
        val identity = identity()
        val id = defaultSenderFor(identity, accountId)
            ?: when (accountId) {
                null -> identity.primaryContact?.id ?: throw HeyException.Api("no sender found in identity", httpStatus = null, retryable = false)
                else -> throw HeyException.NotFound("sender for account not found: $accountId")
            }
        scope.defaultSenderId = id
        id
    }

    /** The identity's user in the scoped account, which is what a record is filed under. */
    suspend fun accountUserId(): Long {
        val accountId = accountId ?: throw HeyException.Usage("account user id needs an account-scoped client")
        return scope.lock.withLock {
            scope.accountUserId?.let { return it }
            val identity = identity()
            val user = identity.allUsers.orEmpty().firstOrNull { it.accountId == accountId }
                ?: throw HeyException.NotFound("user for account not found: $accountId")
            scope.accountUserId = user.id
            user.id
        }
    }

    /** The identity read the client makes for itself, sent as the `GetIdentity` operation. */
    private suspend fun identity(): Identity = send(operation(Routes.GET_IDENTITY, emptyList()))

    /**
     * Shuts down the HTTP client. Only the client the builder made does so: one derived with
     * [forAccount] shares the transport with the client it came from and its siblings, and
     * closing it closes nothing.
     */
    fun close() {
        if (!root) return
        shared.scope.cancel()
        shared.http.close()
    }
}

private fun accountIsAccessible(account: Account): Boolean {
    val status = account.status.orEmpty()
    val purpose = account.purpose.orEmpty()
    return status == "active" || (status == "inactive" && (purpose == "work" || purpose == "domains"))
}

private fun defaultSenderFor(identity: Identity, accountId: Long?): Long? {
    val senders = identity.senders.orEmpty().filter { accountId == null || it.accountId == accountId }
    return (senders.firstOrNull { it.default == true } ?: senders.firstOrNull())?.id
}

/** HEY answers JSON to paths ending in `.json`; a path whose last segment has no extension gets one. */
internal fun withJsonExtension(path: String): String {
    val lastSegment = path.substringAfterLast('/')
    return if (path.isEmpty() || path.endsWith("/") || lastSegment.contains('.')) path else "$path.json"
}

internal fun Method.ktor(): HttpMethod = when (this) {
    Method.GET -> HttpMethod.Get
    Method.POST -> HttpMethod.Post
    Method.PUT -> HttpMethod.Put
    Method.PATCH -> HttpMethod.Patch
    Method.DELETE -> HttpMethod.Delete
}

/** Whether the answer carries `Cache-Control: no-store`, which forbids holding any part of it. */
private fun forbidsStoring(headers: Headers): Boolean =
    headers.getAll(HttpHeaders.CacheControl).orEmpty().any { value ->
        value.split(',').any { directive -> directive.substringBefore('=').trim().equals("no-store", ignoreCase = true) }
    }

/** The headers a cache entry keeps beside the body: everything HEY sent that is not a credential. */
private fun storableHeaders(headers: Headers): Map<String, List<String>> =
    headers.entries().filter { (name, _) -> !isSensitiveHeader(name) }.associate { (name, values) -> name to values.toList() }

/**
 * Whether the answer to a request that asked for this is a document the SDK holds whole and
 * goes on to parse — JSON, a `+json` type, or HTML, anywhere in the `Accept` list — and so is
 * held to the configured cap. Anything else, a blob or an export, is held to the fixed one.
 */
internal fun isParsed(accept: String): Boolean =
    accept.isEmpty() || accept.split(',').any { part ->
        val mediaType = part.substringBefore(';').trim()
        mediaType == "application/json" || mediaType.endsWith("+json") || mediaType == "text/html"
    }
