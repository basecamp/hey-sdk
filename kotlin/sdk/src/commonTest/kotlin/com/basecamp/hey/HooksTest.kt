package com.basecamp.hey

import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull

class HooksTest {
    private class Recording(val log: MutableList<String>, val name: String) : HeyHooks {
        override fun onOperationStart(info: OperationInfo) {
            log += "$name:start:${info.service}.${info.operation}:${info.resourceType}:${info.isMutation}:${info.resourceId}"
        }

        override fun onOperationEnd(info: OperationInfo, result: OperationResult) {
            log += "$name:end:${info.operation}:${(result.error as? HeyException)?.code}"
        }

        override fun onRequestStart(info: RequestInfo) {
            log += "$name:request:${info.method}:${info.attempt}"
        }

        override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
            log += "$name:response:${result.statusCode}:${(result.error as? HeyException)?.code}"
        }
    }

    @Test
    fun anOperationAndItsRequestsAreReported() = runTest {
        val hey = mockHey(status(503), ok("""{"id":5,"kind":"imbox","name":"Imbox"}"""), status(404))
        val log = mutableListOf<String>()
        val client = hey.client { hooks = Recording(log, "a") }
        client.boxes.get(5)
        assertEquals(
            listOf(
                "a:start:Boxes.GetBox:box:false:5",
                "a:request:GET:1",
                "a:response:503:api_error",
                "a:request:GET:2",
                "a:response:200:null",
                "a:end:GetBox:null",
            ),
            log,
        )
        log.clear()
        assertFailsWith<HeyException.NotFound> { client.boxes.get(6) }
        assertEquals("a:end:GetBox:not_found", log.last())
        assertEquals("a:response:404:not_found", log[log.size - 2])
    }

    @Test
    fun chainedHooksNestAndAThrowingHookIsIgnored() = runTest {
        val hey = mockHey(ok("[]"))
        val log = mutableListOf<String>()
        val throwing = object : HeyHooks {
            override fun onOperationStart(info: OperationInfo) = throw IllegalStateException("boom")
        }
        hey.client { hooks = chainHooks(Recording(log, "a"), NoopHooks, throwing, Recording(log, "b")) }.boxes.list()
        assertEquals(listOf("a:start", "b:start", "a:request", "b:request", "b:response", "a:response", "b:end", "a:end"), log.map { it.split(':').take(2).joinToString(":") })
        assertNotNull(chainHooks(NoopHooks) as? NoopHooks)
        assertNull((chainHooks(Recording(log, "x")) as? ChainHooks))
    }
}
