package com.basecamp.hey

import com.basecamp.hey.generated.*
import kotlinx.coroutines.test.runTest
import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.header
import io.ktor.http.HttpHeaders
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
    fun aNoStoreAnswerIsNotHeld() = runTest {
        val hey = mockHey(
            ok("[]", mapOf("ETag" to "\"v1\"", "Cache-Control" to "private, No-Store")),
            ok("[]", mapOf("ETag" to "\"v2\"")),
        )
        val store = InMemoryCache()
        val client = hey.client {
            enableCache = true
            cache = store
        }
        client.boxes.list()
        assertEquals(0, store.size)
        client.boxes.list()
        assertNull(hey.requests[1].header("If-None-Match"))
    }

    @Test
    fun aSuccessWithoutAValidatorEndsWhatWasHeld() = runTest {
        val hey = mockHey(ok("""{"n":"a"}""", mapOf("ETag" to "\"v1\"")), ok("""{"n":"b"}"""), ok("""{"n":"c"}"""))
        val store = InMemoryCache()
        val client = hey.client {
            enableCache = true
            cache = store
        }
        assertEquals("""{"n":"a"}""", client.execute(client.request(Method.GET, "/thing")).text())
        assertEquals("""{"n":"b"}""", client.execute(client.request(Method.GET, "/thing")).text())
        assertEquals("\"v1\"", hey.requests[1].header("If-None-Match"))
        assertEquals(0, store.size, "b replaced a, and cannot be revalidated, so nothing is held")
        client.execute(client.request(Method.GET, "/thing"))
        assertNull(hey.requests[2].header("If-None-Match"), "a's validator is not sent for a body HEY has moved on from")
    }

    @Test
    fun aNoStoreAnswerEvictsWhatWasHeldForTheKey() = runTest {
        val hey = mockHey(
            ok("[]", mapOf("ETag" to "\"v1\"")),
            ok("[]", mapOf("ETag" to "\"v2\"", "Cache-Control" to "no-store")),
            ok("[]"),
        )
        val store = InMemoryCache()
        val client = hey.client {
            enableCache = true
            cache = store
        }
        client.boxes.list()
        assertEquals(1, store.size)
        client.boxes.list()
        assertEquals("\"v1\"", hey.requests[1].header("If-None-Match"))
        assertEquals(0, store.size)
        client.boxes.list()
        assertNull(hey.requests[2].header("If-None-Match"))
    }

    @Test
    fun a304AnswersThePageUnderTheHeadersItWasCachedWith() = runTest {
        val hey = mockHey(
            ok(
                """[{"id":1,"kind":"imbox","name":"a"}]""",
                mapOf("ETag" to "\"v1\"", "Link" to "</boxes.json?page=2>; rel=\"next\"", "X-Total-Count" to "2", "Set-Cookie" to "session=abc"),
            ),
            status(304, headers = mapOf("ETag" to "\"v1\"", "X-Total-Count" to "3")),
            ok("""[{"id":2,"kind":"imbox","name":"b"}]"""),
        )
        val store = InMemoryCache()
        val client = hey.client {
            enableCache = true
            cache = store
        }
        client.boxes.list()
        val held = store.get(cacheKey("https://app.hey.com/boxes.json", "13:authorization1:17:Bearer test-token"))!!
        assertEquals(setOf("ETag", "Link", "X-Total-Count", "Content-Type"), held.headers.keys, "the entry keeps what came with the body, less any credential")

        val revalidated = client.boxes.list()
        assertEquals(listOf(1L), revalidated.value.map { it.id })
        assertEquals("2", revalidated.nextPage, "the Link the 304 left out is the cached one")
        assertEquals(3L, revalidated.totalCount, "the header the 304 did carry replaces the cached one")
        val second = client.nextPage(revalidated)!!
        assertEquals(listOf(2L), second.value.map { it.id })
        assertEquals("2", hey.requests[2].query("page"))
    }

    @Test
    fun whatA304MovesIsWhatTheNextReadGoesOutWith() = runTest {
        val hey = mockHey(
            ok("[]", mapOf("ETag" to "\"v1\"", "Link" to "</boxes.json?page=2>; rel=\"next\"")),
            status(304, headers = mapOf("ETag" to "\"v2\"", "Link" to "</boxes.json?page=3>; rel=\"next\"")),
            status(304, headers = mapOf("ETag" to "\"v2\"")),
        )
        val client = hey.client { enableCache = true }
        client.boxes.list()
        assertEquals("3", client.boxes.list().nextPage)
        assertEquals("3", client.boxes.list().nextPage, "the cursor the 304 moved is what the entry keeps")
        assertEquals("\"v1\"", hey.requests[1].header("If-None-Match"))
        assertEquals("\"v2\"", hey.requests[2].header("If-None-Match"), "the validator the 304 moved is what the next read sends")
    }

    @Test
    fun a304SayingNoStoreEndsTheEntry() = runTest {
        val hey = mockHey(
            ok("[]", mapOf("ETag" to "\"v1\"")),
            status(304, headers = mapOf("ETag" to "\"v1\"", "Cache-Control" to "no-store")),
            ok("[]"),
        )
        val store = InMemoryCache()
        val client = hey.client {
            enableCache = true
            cache = store
        }
        client.boxes.list()
        client.boxes.list()
        assertEquals(0, store.size)
        client.boxes.list()
        assertNull(hey.requests[2].header("If-None-Match"))
    }

    @Test
    fun theKeyNeverHoldsTheCredential() {
        val key = cacheKey("https://app.hey.com/boxes.json", "Bearer secret")
        assertEquals(64, key.length)
        assertEquals(false, key.contains("secret"))
        assertEquals(key, cacheKey("https://app.hey.com/boxes.json", "Bearer secret"))
        assertEquals(false, key == cacheKey("https://app.hey.com/boxes.json", "Bearer other"))
    }

    @Test
    fun aCallerCannotWriteIntoTheCacheThroughTheBodyItWasHanded() = runTest {
        val hey = mockHey(ok("""{"n":1}""", mapOf("ETag" to "\"v1\"")), status(304, headers = mapOf("ETag" to "\"v1\"")), status(304, headers = mapOf("ETag" to "\"v1\"")))
        val client = hey.client { enableCache = true }
        val first = client.execute(client.request(Method.GET, "/thing"))
        first.body.fill(0)
        val second = client.execute(client.request(Method.GET, "/thing"))
        assertEquals("""{"n":1}""", second.text(), "the entry holds its own bytes")
        second.body.fill(0)
        assertEquals("""{"n":1}""", client.execute(client.request(Method.GET, "/thing")).text(), "and hands out its own copy each time")
    }

    /** A strategy that signs with a header of its own: an API key, a cookie, whatever the caller's HEY takes. */
    private class HeaderAuth(private val name: String, private val value: String, private val bearer: String? = null) : AuthStrategy {
        override suspend fun authenticate(request: HttpRequestBuilder) {
            request.header(name, value)
            bearer?.let { request.header(HttpHeaders.Authorization, "Bearer $it") }
        }
    }

    @Test
    fun aStrategyWithoutABearerStillGetsRevalidation() = runTest {
        val hey = mockHey(ok("[]", mapOf("ETag" to "\"v1\"")), status(304, headers = mapOf("ETag" to "\"v1\"")))
        val client = HeyClient {
            auth(HeaderAuth("X-Api-Key", "key-123"))
            engine = hey.engine
            enableCache = true
        }
        client.boxes.list()
        client.boxes.list()
        assertEquals("\"v1\"", hey.requests[1].header("If-None-Match"), "the key partitions the cache as a bearer would")
    }

    @Test
    fun twoIdentitiesThatShareABearerButNotACookieNeverShareAnEntry() = runTest {
        val store = InMemoryCache()
        val hey = mockHey(ok("""{"who":"a"}""", mapOf("ETag" to "\"same\"")), ok("""{"who":"b"}""", mapOf("ETag" to "\"same\"")), status(304))
        val a = HeyClient {
            auth(HeaderAuth(HttpHeaders.Cookie, "session=a", bearer = "shared"))
            engine = hey.engine
            enableCache = true
            cache = store
        }
        val b = HeyClient {
            auth(HeaderAuth(HttpHeaders.Cookie, "session=b", bearer = "shared"))
            engine = hey.engine
            enableCache = true
            cache = store
        }
        a.execute(a.request(Method.GET, "/me"))
        assertEquals("""{"who":"b"}""", b.execute(b.request(Method.GET, "/me")).text())
        assertNull(hey.requests[1].header("If-None-Match"), "b is not asked to validate a's entry")
        assertEquals(2, store.size, "one entry each")
        assertEquals("""{"who":"a"}""", a.execute(a.request(Method.GET, "/me")).text(), "and a's 304 answers a's body")
    }

    /** A strategy that sets one header several times, as a list, or once with the list's spelling. */
    private class ListAuth(private val values: List<String>) : AuthStrategy {
        override suspend fun authenticate(request: HttpRequestBuilder) {
            for (value in values) request.headers.append("X-Key", value)
        }
    }

    @Test
    fun twoSpellingsOfAHeaderThatReadTheSameNeverShareAnEntry() = runTest {
        val store = InMemoryCache()
        val hey = mockHey(ok("""{"who":"A SECRET"}""", mapOf("ETag" to "\"same\"")), ok("""{"who":"b"}""", mapOf("ETag" to "\"same\"")))
        val two = HeyClient { auth(ListAuth(listOf("alpha", "beta"))); engine = hey.engine; enableCache = true; cache = store }
        val one = HeyClient { auth(ListAuth(listOf("alpha, beta"))); engine = hey.engine; enableCache = true; cache = store }
        two.execute(two.request(Method.GET, "/me"))
        assertEquals("""{"who":"b"}""", one.execute(one.request(Method.GET, "/me")).text())
        assertNull(hey.requests[1].header("If-None-Match"), "the one-value identity is not asked to validate the two-value identity's entry")
        assertEquals(2, store.size)
    }

    @Test
    fun aCredentialHeyEchoesIsNotKeptInTheCache() = runTest {
        val hey = mockHey(
            ok("[]", mapOf("ETag" to "\"v1\"", "X-Api-Key" to "key-123", "X-Echo" to "fine")),
            status(304, headers = mapOf("ETag" to "\"v1\"", "X-Api-Key" to "key-123")),
        )
        val store = InMemoryCache()
        val client = HeyClient {
            auth(HeaderAuth("X-Api-Key", "key-123"))
            engine = hey.engine
            enableCache = true
            cache = store
        }
        client.boxes.list()
        val entry = store.get(cacheKey("https://app.hey.com/boxes.json", "9:x-api-key1:7:key-123"))!!
        assertEquals(null, entry.headers.keys.firstOrNull { it.equals("X-Api-Key", ignoreCase = true) }, "the key the strategy signs with is not kept, echoed or not")
        assertEquals(listOf("fine"), entry.headers["X-Echo"])
        client.boxes.list()
        val again = store.get(cacheKey("https://app.hey.com/boxes.json", "9:x-api-key1:7:key-123"))!!
        assertEquals(null, again.headers.keys.firstOrNull { it.equals("X-Api-Key", ignoreCase = true) }, "nor after a 304 that echoes it")
    }
}
