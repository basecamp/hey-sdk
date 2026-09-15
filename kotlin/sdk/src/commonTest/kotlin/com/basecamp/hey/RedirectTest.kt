package com.basecamp.hey

import com.basecamp.hey.generated.*
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
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

    @Test
    fun aHopOnHeyKeepsTheAccountScopeWhateverTheLocationSaid() = runTest {
        val hey = mockHey(
            ok(IDENTITY),
            status(302, headers = mapOf("Location" to "/boxes/all.json")),
            ok("[]"),
            status(302, headers = mapOf("Location" to "/boxes/all.json?filtered_account_id=7")),
            ok("[]"),
        )
        val work = hey.client().forAccount(42)
        work.boxes.list()
        work.boxes.list()

        assertEquals("42", hey.requests[1].query("filtered_account_id"))
        assertEquals("42", hey.requests[2].query("filtered_account_id"), "the hop is scoped when the Location leaves the filter off")
        assertEquals("42", hey.requests[4].query("filtered_account_id"), "and when the Location names another account")
        assertEquals(listOf("42"), hey.requests[4].url.parameters.getAll("filtered_account_id"))
    }

    @Test
    fun aHopOffHeyCarriesNoAccountScope() = runTest {
        val hey = mockHey(ok(IDENTITY), status(302, headers = mapOf("Location" to "https://files.example.com/export.json")), ok("[]"))
        hey.client().forAccount(42).boxes.list()
        assertNull(hey.requests[2].query("filtered_account_id"))
    }

    @Test
    fun aRedirectNeitherCarriesNorTakesTheCacheEntryOfTheUrlAskedFor() = runTest {
        val hey = mockHey(
            ok("""{"which":"a"}""", mapOf("ETag" to "\"x\"")),
            status(302, headers = mapOf("Location" to "/b.json")),
            ok("""{"which":"b"}""", mapOf("ETag" to "\"x\"")),
            status(304, headers = mapOf("ETag" to "\"x\"")),
        )
        val client = hey.client { enableCache = true }
        val a = client.request(Method.GET, "/a")
        assertEquals("""{"which":"a"}""", client.execute(a).text())
        assertEquals("""{"which":"b"}""", client.execute(client.request(Method.GET, "/a")).text(), "the answer reached through the redirect is b's")
        assertNull(hey.requests[2].header("If-None-Match"), "b is not asked to validate a's entry")
        assertEquals("/b.json", hey.requests[2].path)
        assertEquals("""{"which":"a"}""", client.execute(client.request(Method.GET, "/a")).text(), "a's entry is still a's, not b's")
        assertEquals("\"x\"", hey.requests[3].header("If-None-Match"))
    }

    @Test
    fun a401FromAHopThatCarriedNoCredentialsRefreshesNothing() = runTest {
        var refreshes = 0
        val hey = mockHey(status(302, headers = mapOf("Location" to "https://files.example.com/export.json")), status(401), ok("[]"))
        val client = HeyClient {
            accessToken(object : TokenProvider {
                override suspend fun accessToken(): String = "token"
                override suspend fun refresh(): Boolean {
                    refreshes += 1
                    return true
                }
            })
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        assertFailsWith<HeyException.Auth> { client.boxes.list() }
        assertEquals(0, refreshes, "HEY's credentials were not the ones rejected")
        assertEquals(2, hey.requests.size, "and nothing is sent again")
    }
}
