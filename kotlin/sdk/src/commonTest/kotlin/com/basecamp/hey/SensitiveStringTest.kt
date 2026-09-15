package com.basecamp.hey

import com.basecamp.hey.generated.models.Sender
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse

class SensitiveStringTest {
    @Test
    fun itPrintsRedactedAndTravelsAsAString() {
        val sender = heyJson.decodeFromString(Sender.serializer(), """{"id":1,"email_address":"jane@example.com"}""")
        assertEquals("jane@example.com", sender.emailAddress!!.expose())
        assertEquals("[REDACTED]", sender.emailAddress.toString())
        assertFalse(sender.toString().contains("jane@example.com"))
        assertEquals("""{"id":1,"email_address":"jane@example.com"}""", heyJson.encodeToString(Sender.serializer(), sender))
        assertEquals("StaticTokenProvider([REDACTED])", StaticTokenProvider("secret").toString())
    }
}
