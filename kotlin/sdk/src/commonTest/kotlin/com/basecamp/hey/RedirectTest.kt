package com.basecamp.hey

import com.basecamp.hey.generated.*
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.time.Duration

class RedirectTest {
    /** A strategy that signs with headers of its own naming, as a custom [AuthStrategy] may. */
    private class ApiKeyAuth : AuthStrategy {
        override suspend fun authenticate(request: HttpRequestBuilder) {
            request.header("X-Api-Key", "key-123")
            request.header(HttpHeaders.Authorization, "Bearer secret")
            request.header(HttpHeaders.Cookie, "session=abc")
        }
    }

    private fun client(hey: MockHey): HeyClient = HeyClient {
        auth(ApiKeyAuth())
        engine = hey.engine
        timeout = Duration.INFINITE
        maxRetryJitter = Duration.ZERO
    }

    @Test
    fun aHopToAnotherOriginGoesOutWithoutAnyHeaderTheStrategySet() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "https://files.example.com/boxes.json")), ok("[]"))
        client(hey).boxes.list()

        assertEquals(2, hey.requests.size)
        val hop = hey.requests[1]
        assertEquals("files.example.com", hop.url.host)
        assertNull(hop.header("X-Api-Key"))
        assertNull(hop.header("Authorization"))
        assertNull(hop.header("Cookie"))
        assertEquals("application/json", hop.header("Accept"))
        assertEquals(HeyConfig.DEFAULT_USER_AGENT, hop.header("User-Agent"))
    }

    @Test
    fun aHopOnTheSameOriginKeepsThem() = runTest {
        val hey = mockHey(status(302, headers = mapOf("Location" to "/boxes/all.json")), ok("[]"))
        client(hey).boxes.list()

        val hop = hey.requests[1]
        assertEquals("/boxes/all.json", hop.path)
        assertEquals("key-123", hop.header("X-Api-Key"))
        assertEquals("Bearer secret", hop.header("Authorization"))
        assertEquals("session=abc", hop.header("Cookie"))
    }
}
