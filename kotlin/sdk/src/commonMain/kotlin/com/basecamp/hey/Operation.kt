package com.basecamp.hey

import io.ktor.http.Url
import io.ktor.http.encodeURLParameter
import kotlinx.serialization.serializer

/** A body an operation carries, as it goes over the wire. */
class Body internal constructor(
    /** The `Content-Type` the body is sent as. */
    val contentType: String,
    /** The bytes. */
    val bytes: ByteArray,
) {
    override fun toString(): String = "Body($contentType, ${bytes.size} bytes)"
}

private const val BROWSER_ACCEPT = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

/**
 * A request the client has not sent yet. Generated service methods build one from a
 * [Route] with [HeyClient.operation]; [HeyClient.request] builds one for a path the model
 * does not cover, and [HeyClient.form] one for a browser form.
 *
 * An operation prints what it is and where it goes, not what it carries: the query's names
 * without their values, the body's shape without its bytes.
 */
class Operation internal constructor(
    internal val id: String,
    /** What the call means, as the hooks hear it. */
    var info: OperationInfo,
    route: Route?,
    /** The HTTP method the operation is sent with. */
    val method: Method,
    /** The path the operation is sent to, parameters already filled in. */
    val path: String,
    internal var url: Url?,
) {
    /** The modelled route this sends, whose retry policy the client honours. A path the caller wrote has none; a page after the first carries the first's. */
    var route: Route? = route
        internal set

    internal val query: MutableList<Pair<String, String>> = mutableListOf()
    internal val headers: MutableList<Pair<String, String>> = mutableListOf()
    internal var body: Body? = null
    internal var idempotent: Boolean = route?.idempotent ?: (method == Method.GET || method == Method.PUT || method == Method.DELETE)
    internal var emptyOn: List<Int> = route?.emptyOn ?: emptyList()
    internal var accept: String = if (route?.html == true) "text/html" else "application/json"

    /** HEY answers JSON to paths that end in `.json`, so a modelled route gets one put back on. */
    internal var jsonSuffix: Boolean = route?.html != true
    internal var noCache: Boolean = false
    internal var captureRedirects: Boolean = false
    internal var quiet: Boolean = false
    internal var unsigned: Boolean = false

    /** Adds a query parameter. The same name may be added more than once. */
    fun query(name: String, value: Any): Operation {
        query += name to value.toString()
        return this
    }

    /** Adds a query parameter when there is a value for it, and nothing otherwise. */
    fun queryOptional(name: String, value: Any?): Operation {
        if (value != null) query(name, value)
        return this
    }

    /** A JSON body, already encoded. [json] encodes a model for you. */
    fun jsonBody(encoded: String): Operation {
        body = Body("application/json", encoded.encodeToByteArray())
        return this
    }

    /** A form-encoded body, as a browser would post it. A name may repeat, for a list. */
    fun form(fields: List<Pair<String, String>>): Operation {
        val encoded = fields.joinToString("&") { (name, value) ->
            "${name.encodeURLParameter()}=${value.encodeURLParameter()}"
        }
        body = Body("application/x-www-form-urlencoded", encoded.encodeToByteArray())
        return this
    }

    /** A body of the caller's own, with its content type. */
    fun bodyBytes(contentType: String, bytes: ByteArray): Operation {
        body = Body(contentType, bytes)
        return this
    }

    /** Adds a header HEY handed the request — the storage put's — as the SDK's own business, not a caller's: a credential is the auth strategy's to add, and a header the cache would need to key on is none of these. */
    internal fun header(name: String, value: String): Operation {
        headers += name to value
        return this
    }

    /** Says what the call means, for the hooks: a hand-written wrapper announces itself here. */
    fun info(info: OperationInfo): Operation {
        this.info = info
        return this
    }

    /** Renames the operation the hooks hear, keeping the rest of what it means. */
    fun operationName(name: String): Operation {
        info = info.copy(operation = name)
        return this
    }

    /** Names the record the call acts on, for the hooks. */
    fun resourceId(id: Long): Operation {
        info = info.copy(resourceId = id)
        return this
    }

    /** Whether the client may resend the operation after a transient failure. */
    fun idempotent(idempotent: Boolean): Operation {
        this.idempotent = idempotent
        return this
    }

    /** What the operation asks for in `Accept`. */
    fun accept(accept: String): Operation {
        this.accept = accept
        return this
    }

    /** Takes a redirect for the answer rather than following it, as a form post wants. */
    fun captureRedirects(): Operation {
        captureRedirects = true
        return this
    }

    /**
     * Sends the path as written, with no `.json` put on it, and leaves `Accept` alone: for an
     * endpoint that answers JSON only under its bare path — an autocomplete list — or streams
     * a file rather than a document.
     */
    fun withoutJsonSuffix(): Operation {
        jsonSuffix = false
        return this
    }

    /** Sends the path as written, with a browser's `Accept`: the way in to an endpoint HEY serves only as a form. */
    fun formRepresentation(): Operation {
        accept = BROWSER_ACCEPT
        jsonSuffix = false
        return this
    }

    /** Leaves the response cache alone for this read. */
    fun noCache(): Operation {
        noCache = true
        return this
    }

    /**
     * One request inside another operation rather than an operation of its own: the
     * operation hooks are skipped while the request hooks still fire, so a read a wrapper
     * makes on the way to its own answer shows up as part of that answer.
     */
    fun quiet(): Operation {
        quiet = true
        return this
    }

    /**
     * Sends the request without the client's credentials or account scope, and takes a 401
     * as the answer it is rather than a reason to refresh them: for a URL that authenticates
     * itself, which the storage service's may do on HEY's own origin. The URL and its hops go
     * exactly as named, and the hooks hear the URL cut back to its origin, since such a URL
     * carries its signature in the open.
     */
    internal fun unsigned(): Operation {
        unsigned = true
        return this
    }

    /** What an error or a log calls the operation. */
    internal fun label(): String = if (info.service == "Raw") method.name else info.operation

    override fun toString(): String =
        "Operation(id=${id.substringBefore('?')}, method=$method, path=${path.substringBefore('?')}, query=${query.map { it.first }}, body=$body)"

    internal companion object {
        fun forRoute(route: Route, params: List<Any>): Operation = Operation(
            id = route.id,
            info = OperationInfo(
                service = route.service,
                operation = route.id,
                resourceType = route.resourceType,
                isMutation = !route.readonly,
            ),
            route = route,
            method = route.method,
            path = route.fill(params),
            url = null,
        )

        fun raw(method: Method, path: String): Operation {
            val id = "$method $path"
            return Operation(
                id = id,
                info = OperationInfo(service = "Raw", operation = id, resourceType = "raw", isMutation = method != Method.GET),
                route = null,
                method = method,
                path = path,
                url = null,
            )
        }

        /** A read of a URL HEY handed out, sent under [route]'s policy when the read that got it had one. */
        fun at(method: Method, url: Url, route: Route? = null): Operation {
            val operation = raw(method, url.encodedPath)
            operation.url = url
            operation.route = route
            return operation
        }
    }
}

/** Encodes a model as the operation's JSON body, which is what every modelled write sends. */
inline fun <reified T> Operation.json(body: T): Operation = jsonBody(heyJson.encodeToString(serializer<T>(), body))
