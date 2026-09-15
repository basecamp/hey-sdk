package com.basecamp.hey

import io.ktor.http.decodeURLQueryComponent

/** What a form request asks for: the browser's `Accept` a form-backed endpoint is reached under. */
internal const val BROWSER_ACCEPT_HEADER = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

/** The operations the hooks heard start and end, for a test of what a convenience announces itself as. */
internal class OperationLog : HeyHooks {
    val started = mutableListOf<String>()
    val ended = mutableListOf<String>()

    override fun onOperationStart(info: OperationInfo) {
        started += "${info.service}.${info.operation}:${info.resourceType}:${info.isMutation}:${info.resourceId}"
    }

    override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
        ended += "${info.service}.${info.operation}:${(result.error as? HeyException)?.code ?: result.error?.message}"
    }
}

/** A form body as the pairs it carries, in order, since a name may repeat for a list. */
internal fun formPairs(body: String): List<Pair<String, String>> =
    body.split('&').filter { it.isNotEmpty() }.map { pair ->
        val (key, value) = pair.split('=', limit = 2).let { it[0] to it.getOrElse(1) { "" } }
        key.decodeURLQueryComponent(plusIsSpace = true) to value.decodeURLQueryComponent(plusIsSpace = true)
    }
