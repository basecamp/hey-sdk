package com.basecamp.hey

import com.basecamp.hey.generated.*
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.respond
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class ClientBuilderTest {
    private val engine = MockEngine { respond("[]") }

    @Test
    fun plainHttpIsRefusedOffThisMachine() {
        val error = assertFailsWith<HeyException.Usage> {
            HeyClient {
                accessToken("token")
                baseUrl = "http://evil.example.com"
                this.engine = this@ClientBuilderTest.engine
            }
        }
        assertEquals(HeyException.CODE_USAGE, error.code)
        assertContains(error.message!!, "must use HTTPS")
    }

    @Test
    fun plainHttpIsAllowedOnLocalhost() {
        for (base in listOf("http://localhost:3000", "http://127.0.0.1:8080", "http://app.localhost", "https://app.hey.com")) {
            val client = HeyClient {
                accessToken("token")
                baseUrl = base
                this.engine = this@ClientBuilderTest.engine
            }
            assertEquals(base, client.baseUrl.toString().trimEnd('/'))
        }
    }

    @Test
    fun credentialsAreRequired() {
        assertFailsWith<HeyException.Usage> { HeyClient { engine = this@ClientBuilderTest.engine } }
        assertFailsWith<HeyException.Usage> {
            HeyClient {
                accessToken("token")
                auth(BearerAuth(StaticTokenProvider("other")))
            }
        }
    }

    @Test
    fun theSettingsAreChecked() {
        assertFailsWith<HeyException.Usage> { HeyClient { accessToken("t"); maxPages = 0; engine = this@ClientBuilderTest.engine } }
        assertFailsWith<HeyException.Usage> { HeyClient { accessToken("t"); maxRetries = -1; engine = this@ClientBuilderTest.engine } }
        assertFailsWith<HeyException.Usage> { HeyClient { accessToken("t"); timeout = kotlin.time.Duration.ZERO; engine = this@ClientBuilderTest.engine } }
        assertFailsWith<HeyException.Usage> { HeyClient { accessToken("t"); baseUrl = "not a url" } }
    }

    @Test
    fun theUserAgentNamesTheSdkAndTheApiContract() {
        assertEquals("hey-sdk-kotlin/${HeyConfig.VERSION} (api:${HeyConfig.API_VERSION})", HeyConfig.DEFAULT_USER_AGENT)
        assertTrue(Regex("\\d{4}-\\d{2}-\\d{2}").matches(HeyConfig.API_VERSION))
    }

    @Test
    fun closingADerivedClientLeavesTheTransportToTheRoot() = runTest {
        val hey = mockHey(ok(IDENTITY), ok(IDENTITY), ok("[]"), ok("[]"))
        val root = hey.client()
        val work = root.forAccount(42)
        val other = root.forAccount(42)
        work.close()
        other.boxes.list()
        root.boxes.list()
        assertEquals(4, hey.requests.size, "the root and a sibling still send after a derived client is closed")
        root.close()
        assertFailsWith<Exception> { root.boxes.list() }
    }
}
