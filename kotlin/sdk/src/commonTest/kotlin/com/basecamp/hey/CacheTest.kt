package com.basecamp.hey

import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull

class CacheTest {
    @Test
    fun aCachedReadRevalidatesAndReadsThe304FromTheCache() = runTest {
        val hey = mockHey(
            ok("""[{"id":7,"kind":"imbox","name":"Imbox"}]""", mapOf("ETag" to "\"v1\"")),
            status(304, headers = mapOf("ETag" to "\"v1\"")),
            ok("""[{"id":8,"kind":"imbox","name":"Imbox"}]""", mapOf("ETag" to "\"v2\"")),
            status(304, headers = mapOf("ETag" to "\"v2\"")),
        )
        val fromCache = mutableListOf<Boolean>()
        val client = hey.client {
            enableCache = true
            hooks = object : HeyHooks {
                override fun onRequestEnd(info: RequestInfo, result: RequestResult) {
                    fromCache += result.fromCache
                }
            }
        }
        assertEquals(7L, client.boxes.list().value.single().id)
        assertEquals(7L, client.boxes.list().value.single().id)
        assertEquals(8L, client.boxes.list().value.single().id)
        assertEquals(8L, client.boxes.list().value.single().id)

        assertNull(hey.requests[0].header("If-None-Match"))
        assertEquals("\"v1\"", hey.requests[1].header("If-None-Match"))
        assertEquals("\"v1\"", hey.requests[2].header("If-None-Match"))
        assertEquals("\"v2\"", hey.requests[3].header("If-None-Match"))
        assertEquals(listOf(false, true, false, true), fromCache)
    }

    @Test
    fun aClientWithoutACacheNeverSendsAConditionalRequest() = runTest {
        val hey = mockHey(ok("[]", mapOf("ETag" to "\"v1\"")), ok("[]"))
        val client = hey.client()
        client.boxes.list()
        client.boxes.list()
        assertNull(hey.requests[1].header("If-None-Match"))
    }

    @Test
    fun a304WithNothingCachedIsAnError() = runTest {
        val hey = mockHey(status(304))
        val error = assertFailsWith<HeyException.Api> { hey.client { enableCache = true }.boxes.list() }
        assertEquals(304, error.httpStatus)
    }

    @Test
    fun theKeyNeverHoldsTheCredential() {
        val key = cacheKey("https://app.hey.com/boxes.json", "Bearer secret")
        assertEquals(64, key.length)
        assertEquals(false, key.contains("secret"))
        assertEquals(key, cacheKey("https://app.hey.com/boxes.json", "Bearer secret"))
        assertEquals(false, key == cacheKey("https://app.hey.com/boxes.json", "Bearer other"))
    }
}
