package com.basecamp.hey

import com.basecamp.hey.generated.*
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
import io.ktor.http.encodedPath
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

    /** A strategy that signs the method and path, as an HMAC scheme does: a hop to another URL needs a signature of its own. */
    private class SigningAuth : AuthStrategy {
        override suspend fun authenticate(request: HttpRequestBuilder) {
            request.header("X-Signature", "${request.method.value} ${request.url.encodedPath}")
        }
    }

    @Test
    fun aHopOnTheSameOriginIsSignedForWhereItGoes() = runTest {
        val hey = mockHey(
            status(302, headers = mapOf("Location" to "/boxes/all.json")),
            ok("[]"),
            status(303, headers = mapOf("Location" to "/postings/seen.json")),
            ok(""),
            status(302, headers = mapOf("Location" to "https://files.example.com/export.json")),
            ok("[]"),
        )
        val client = HeyClient {
            auth(SigningAuth())
            engine = hey.engine
            timeout = Duration.INFINITE
        }
        client.boxes.list()
        assertEquals("GET /boxes.json", hey.requests[0].header("X-Signature"))
        assertEquals("GET /boxes/all.json", hey.requests[1].header("X-Signature"), "signed again for the URL the hop goes to")

        client.execute(client.request(Method.POST, "/postings/mark").jsonBody("{}"))
        assertEquals("POST /postings/mark.json", hey.requests[2].header("X-Signature"))
        assertEquals("GET", hey.requests[3].method, "a 303 turns the POST into a GET")
        assertEquals("GET /postings/seen.json", hey.requests[3].header("X-Signature"), "and the signature says so")

        client.boxes.list()
        assertNull(hey.requests[5].header("X-Signature"), "a hop off the origin is never signed")
    }

    @Test
    fun aStrategyThatCannotSignAHopFailsAsItselfAndIsNotResent() = runTest {
        var signings = 0
        val failing = object : AuthStrategy {
            override suspend fun authenticate(request: HttpRequestBuilder) {
                signings += 1
                if (signings > 1) throw IllegalStateException("signer offline")
                request.header("X-Signature", "ok")
            }
        }
        val hey = mockHey(status(302, headers = mapOf("Location" to "/boxes/all.json")), ok("[]"))
        val log = mutableListOf<String>()
        val client = HeyClient {
            auth(failing)
            engine = hey.engine
            timeout = Duration.INFINITE
            hooks = object : HeyHooks {
                override fun onRetry(info: RequestInfo, attempt: Int, error: Throwable, delayMs: Long) { log += "retry" }
                override fun onRequestEnd(info: RequestInfo, result: RequestResult) { log += "request:${result.error?.message}" }
                override fun onOperationEnd(info: OperationInfo, result: OperationResult) { log += "operation:${result.error?.message}" }
            }
        }
        val error = assertFailsWith<IllegalStateException> { client.boxes.list() }
        assertEquals("signer offline", error.message)
        assertEquals(1, hey.requests.size, "the hop is never sent and nothing is resent")
        assertEquals(listOf("request:signer offline", "operation:signer offline"), log)
    }
}
