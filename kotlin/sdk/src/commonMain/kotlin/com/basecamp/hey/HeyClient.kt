package com.basecamp.hey

import com.basecamp.hey.generated.Routes
import com.basecamp.hey.generated.models.Account
import com.basecamp.hey.generated.models.Identity
import io.ktor.client.HttpClient
import io.ktor.client.HttpClientConfig
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpRequestTimeoutException
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
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

    /** Custom Ktor [HttpClientEngine] (e.g., for testing with MockEngine). */
    var engine: HttpClientEngine? = null

    /** Pre-configured Ktor [HttpClient] to use instead of creating one internally. It must not follow redirects on its own. */
    var httpClient: HttpClient? = null

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
        if (httpClient != null && engine != null) {
            throw HeyException.Usage("Cannot set both httpClient and engine. Use one or the other.")
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

        val ownsHttpClient = httpClient == null
        val http = httpClient ?: (engine?.let { HttpClient(it) { configure(timeout) } } ?: HttpClient { configure(timeout) })
        val shared = Shared(
            config = config,
            baseUrl = parsed,
            http = http,
            ownsHttpClient = ownsHttpClient,
            auth = resolvedAuth,
            cache = if (enableCache) cache ?: InMemoryCache() else null,
            hooks = hooks,
        )
        return HeyClient(shared, null, ScopeState())
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
    val ownsHttpClient: Boolean,
    val auth: AuthStrategy,
    val cache: ResponseCache?,
    val hooks: HeyHooks,
) {
    /** How many times the credentials have been refreshed, so a 401 answered after someone else refreshed is resent rather than refreshed again. */
    @Volatile
    var refreshes: Long = 0

    /** One refresh at a time. */
    val refreshing = Mutex()
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
        (serviceCache.getOrPut(key) { factory() } as T)

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
        val response = execute(operation)
        return decode(response, deserializer, label)
    }

    /** Sends an operation and decodes its JSON body as [T]. */
    suspend inline fun <reified T> send(operation: Operation): T = send(operation, serializer<T>())

    /** Sends an operation whose answer carries no body worth reading. */
    suspend fun sendUnit(operation: Operation) {
        execute(operation)
    }

    /** Sends an operation and reads its body as text: the HTML page a route serves no JSON for. */
    suspend fun sendText(operation: Operation): String = execute(operation).text()

    /** Sends an operation that answers a status meaning "nothing there" with null. */
    suspend fun <T> sendOptional(operation: Operation, deserializer: DeserializationStrategy<T>): T? {
        val label = operation.label()
        val response = execute(operation)
        return if (response.empty) null else decode(response, deserializer, label)
    }

    /** Sends an operation that answers a status meaning "nothing there" with null. */
    suspend inline fun <reified T> sendOptional(operation: Operation): T? = sendOptional(operation, serializer<T>())

    /** Sends a paginated read and keeps the cursor HEY answered with, and the route, so the next page is read under the same policy. */
    suspend fun <T> sendPage(operation: Operation, deserializer: DeserializationStrategy<T>): Page<T> {
        val label = operation.label()
        val info = operation.info
        val route = operation.route
        val response = execute(operation)
        return Page.of(decode(response, deserializer, label), response, info, route, deserializer)
    }

    /** Sends a paginated read and keeps the cursor HEY answered with. */
    suspend inline fun <reified T> sendPage(operation: Operation): Page<T> = sendPage(operation, serializer<T>())

    /** Sends a form request and reads the redirect it answered with. */
    suspend fun sendForm(operation: Operation): FormResponse = FormResponse.of(execute(operation))

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
    suspend fun execute(operation: Operation): Response {
        if (operation.quiet) return dispatch(operation)
        val hooks = shared.hooks
        val info = operation.info
        val started = currentTimeMillis()
        hooks.safeOperationStart(info)
        try {
            val response = dispatch(operation)
            hooks.safeOperationEnd(info, OperationResult(elapsedSince(started)))
            return response
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
        val url: Url,
        val response: HttpResponse,
        val status: Int,
        val cached: Pair<String, CachedResponse>?,
        val info: RequestInfo,
        val duration: Duration,
    )

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
            val signedUnder = shared.refreshes
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

            val (finalUrl, response) = sent.getOrThrow()
            val status = response.status.value
            val retryable = status in budget.retryOn
            val renewed = status == 401 && !refreshed && try {
                refreshCredentials(signedUnder)
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
                val cause = HeyException.fromResponse(status, operation.method, response.headers, ByteArray(0))
                val retryAfter = if (status == 429) retryAfterSeconds(response.headers["Retry-After"]) else null
                val wait = if (retryAfter != null && retryAfter > 0) retryAfter.seconds else waitFor(delay)
                hooks.safeRequestEnd(info, RequestResult(status, duration, error = cause))
                hooks.safeRetry(info, attempt + 1, cause, wait.inWholeMilliseconds)
                delay(wait)
                delay = nextDelay(delay)
                attempt += 1
                continue
            }
            return Answered(finalUrl, response, status, cached, info, duration)
        }
    }

    private fun networkFailure(error: Exception): HeyException =
        HeyException.Network(
            "Network error",
            hint = HeyException.truncateMessage(error.message ?: error::class.simpleName ?: "unknown"),
            cause = error,
            // An attempt that ran the whole request budget out is a slowness a resend tends to repeat.
            retryable = error !is HttpRequestTimeoutException,
        )

    /**
     * Answers a 401 with fresh credentials, once for all the requests the stale ones earned
     * it on. Refreshes go one at a time, and a request that was signed before the last
     * refresh is simply resent: the credentials it will pick up are already the new ones.
     */
    private suspend fun refreshCredentials(signedUnder: Long): Boolean = shared.refreshing.withLock {
        when {
            shared.refreshes != signedUnder -> true
            shared.auth.refresh() -> {
                shared.refreshes += 1
                true
            }
            else -> false
        }
    }

    internal fun urlFor(operation: Operation): Url {
        val builder = operation.url?.let { URLBuilder(it) } ?: run {
            val (path, query) = operation.path.split('?', limit = 2).let { it[0] to it.getOrNull(1) }
            val full = if (operation.jsonSuffix) withJsonExtension(path) else path
            URLBuilder(shared.baseUrl).apply {
                encodedPath = shared.baseUrl.encodedPath.trimEnd('/') + "/" + full.trimStart('/')
                if (!query.isNullOrEmpty()) encodedParameters.appendAll(parseQueryString(query))
            }
        }
        for ((name, value) in operation.query) builder.parameters.append(name, value)
        val accountId = accountId
        if (accountId != null && isSameOrigin(builder.build(), shared.baseUrl)) {
            builder.parameters.remove(ACCOUNT_FILTER_PARAMETER)
            builder.parameters.append(ACCOUNT_FILTER_PARAMETER, accountId.toString())
        }
        return builder.build()
    }

    private class Prepared(
        val request: HttpRequestBuilder,
        val cached: Pair<String, CachedResponse>?,
        /** The headers the auth strategy put on the request, lowercased: what a hop to another origin must not carry. */
        val credentialHeaders: Set<String>,
    )

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
        val beforeAuth = request.headers.build()
        shared.auth.authenticate(request)
        val credentialHeaders = request.headers.names()
            .filter { name -> request.headers.getAll(name) != beforeAuth.getAll(name) }
            .map { it.lowercase() }
            .toSet()

        val cache = cacheFor(operation)
        val credential = request.headers[HttpHeaders.Authorization]
        var cached = previous
        if (cache != null && credential != null) {
            val key = cacheKey(url.toString(), credential)
            if (cached == null || cached.first != key) cached = lookUp(cache, key)
        } else {
            cached = null
        }
        cached?.second?.etag?.takeIf { it.isNotEmpty() }?.let { request.header(HttpHeaders.IfNoneMatch, it) }
        return Prepared(request, cached, credentialHeaders)
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
     */
    private suspend fun transmit(operation: Operation, start: Url, prepared: Prepared): Pair<Url, HttpResponse> {
        var url = start
        var request = prepared.request
        var hops = 0
        while (true) {
            val response = shared.http.request(request)
            val next = if (operation.captureRedirects) null else redirectTarget(url, response)
            if (next == null) return url to response
            if (hops == MAX_REDIRECTS) {
                throw HeyException.Network("${operation.label()} redirected more than $MAX_REDIRECTS times", retryable = false)
            }
            requireSecureEndpoint(next)
            request = redirected(request, response.status.value, url, next, prepared.credentialHeaders)
            url = next
            hops += 1
        }
    }

    private fun redirectTarget(url: Url, response: HttpResponse): Url? {
        if (response.status.value !in REDIRECT_STATUSES) return null
        val location = response.headers[HttpHeaders.Location] ?: return null
        return runCatching { URLBuilder(url).takeFrom(location).build() }.getOrNull()
    }

    private fun redirected(outgoing: HttpRequestBuilder, status: Int, from: Url, to: Url, credentialHeaders: Set<String>): HttpRequestBuilder {
        val request = HttpRequestBuilder()
        val keepBody = status == 307 || status == 308 || outgoing.method == HttpMethod.Get || outgoing.method == HttpMethod.Head
        request.method = if (keepBody) outgoing.method else HttpMethod.Get
        request.url(to)
        val sameOrigin = isSameOrigin(from, to)
        outgoing.headers.entries().forEach { (name, values) ->
            if (!sameOrigin && (isSensitiveHeader(name) || name.lowercase() in credentialHeaders)) return@forEach
            if (!keepBody && (name.equals(HttpHeaders.ContentType, true) || name.equals(HttpHeaders.ContentLength, true))) return@forEach
            values.forEach { request.headers.append(name, it) }
        }
        if (keepBody) request.setBody(outgoing.body)
        return request
    }

    private suspend fun finish(operation: Operation, answered: Answered): Response {
        val response = answered.response
        val status = answered.status
        val headers: Headers = response.headers
        if (status == 304) {
            val entry = answered.cached?.second
            return if (entry != null && entry.etag.isNotEmpty()) {
                Response(200, entry.headersUpdatedBy(headers), entry.body, answered.url, fromCache = true, empty = false)
            } else {
                throw HeyException.Api("304 received but no cached response available", httpStatus = 304, retryable = false)
            }
        }
        val parsed = operation.accept == "application/json" || operation.accept == "text/html"
        val bound = if (parsed) shared.config.maxResponseBodyBytes else HeyConfig.MAX_RESPONSE_BODY_BYTES
        val body = try {
            readBody(response, bound)
        } catch (refusal: HeyException) {
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
        if (status in 200..299) {
            val key = answered.cached?.first
            val cache = shared.cache
            if (key != null && cache != null) {
                val etag = headers[HttpHeaders.ETag]
                when {
                    // An answer HEY says not to keep is not kept, and neither is what it replaced.
                    forbidsStoring(headers) -> cache.invalidate(key)
                    etag != null && body.isNotEmpty() -> cache.set(key, CachedResponse(etag, body, storableHeaders(headers)))
                }
            }
            return Response(status, headers, body, answered.url, fromCache = false, empty = false)
        }
        if (status in operation.emptyOn || (operation.captureRedirects && status in REDIRECT_STATUSES)) {
            return Response(status, headers, body, answered.url, fromCache = false, empty = true)
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
                throw HeyException.Api("response body exceeds $bound bytes", httpStatus = null, retryable = false, responseTooLarge = true)
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
        val root = HeyClient(shared, null, ScopeState())
        val identity = root.identity()
        val accessible = identity.accounts.orEmpty().any { it.id == accountId && accountIsAccessible(it) }
        if (!accessible) throw HeyException.NotFound("accessible account not found: $accountId")
        val scope = ScopeState()
        scope.defaultSenderId = defaultSenderFor(identity, accountId)
        scope.accountUserId = identity.allUsers.orEmpty().firstOrNull { it.accountId == accountId }?.id
        return HeyClient(shared, accountId, scope)
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

    /** Shuts down the HTTP client, when the SDK created it. A caller-provided one stays the caller's. */
    fun close() {
        if (shared.ownsHttpClient) shared.http.close()
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
