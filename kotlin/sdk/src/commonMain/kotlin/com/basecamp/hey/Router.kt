package com.basecamp.hey

import com.basecamp.hey.generated.Routes

/**
 * What a pasted HEY URL or path is: the pattern it matched, the part of HEY it belongs to,
 * every operation that pattern serves by HTTP method, and the parameters read out of it.
 *
 * ```kotlin
 * val match = router.recognize("https://app.hey.com/topics/456?x=1")!!
 * match.operation()   // "GetTopic"
 * match.resourceId()  // "456"
 * ```
 */
class RouteMatch internal constructor(
    /** The pattern the path matched, `{param}` placeholders included. */
    val pattern: String,
    /** The part of HEY the pattern belongs to, as the model titles it. */
    val resource: String,
    /** The operations the pattern serves, by HTTP method. */
    val operations: Map<Method, String>,
    /** The path parameters, in the order they appear in the pattern. */
    val params: List<Pair<String, String>>,
) {
    /** The operation a pasted URL most likely means: the GET when the pattern serves one, else the first by name. */
    fun operation(): String = operations[Method.GET] ?: operations.values.min()

    /** The id the path ends in, when it ends in one: the record the URL is of. */
    fun resourceId(): String? = params.lastOrNull()?.second

    override fun toString(): String = "RouteMatch(pattern=$pattern, operation=${operation()}, params=$params)"
}

/**
 * Recognises HEY URLs and paths against the route table, as Rust's and Go's routers do: a
 * whole URL or a bare path, with or without a `.json` suffix, a query, a fragment or a
 * trailing slash. Deeper patterns are tried first, so `/boxes/{boxId}/groups/{groupId}`
 * wins over anything shorter that would also fit.
 */
class Router(routes: List<Route> = Routes.ALL) {
    private class Pattern(val pattern: String, val routes: List<Route>)

    private val patterns: List<Pattern> = routes
        .groupBy { it.pattern }
        .map { (pattern, routes) -> Pattern(pattern, routes) }
        .sortedWith(compareByDescending<Pattern> { it.pattern.count { c -> c == '/' } }.thenBy { it.pattern })

    /** The route a URL or path names, or null when the table has none for it. */
    fun recognize(pathOrUrl: String): RouteMatch? {
        val raw = parseAbsoluteUrl(pathOrUrl)?.encodedPath ?: pathOrUrl.substringBefore('?').substringBefore('#')
        val path = raw.trimEnd('/').removeSuffix(".json")
        for (candidate in patterns) {
            val params = candidate.routes.first().recognize(path) ?: continue
            return RouteMatch(
                pattern = candidate.pattern,
                resource = candidate.routes.first().resource,
                operations = candidate.routes.associate { it.method to it.id },
                params = params.entries.map { (name, value) -> name to value },
            )
        }
        return null
    }
}

/** The router over every modelled route. */
val router: Router by lazy { Router() }
