package com.basecamp.hey

import com.basecamp.hey.generated.habits
import com.basecamp.hey.services.HabitParams
import io.ktor.http.headersOf
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

/** A 422 keeps the reasons HEY gave: [HeyException.Validation.fieldErrors], and the hint that joins them. */
class ValidationErrorTest {
    /** The body HEY answers a refused habit with: the model's full messages (`habit.errors.full_messages`). */
    private val habitRefusal = """{"errors":["Title can't be blank","Days must include at least one day"]}"""

    private val habitReasons = listOf(
        FieldError(null, "Title can't be blank"),
        FieldError(null, "Days must include at least one day"),
    )

    private fun refused(body: String): HeyException =
        HeyException.fromResponse(422, Method.POST, headersOf(), body.encodeToByteArray())

    @Test
    fun aRefusedHabitKeepsEachReasonHeyGave() = runTest {
        val hey = mockHey(status(422, habitRefusal, headers = mapOf("X-Request-Id" to "req-422")))

        val error = assertFailsWith<HeyException.Validation> { hey.client().habits.createHabit(HabitParams()) }

        assertEquals(422, error.httpStatus)
        assertEquals(false, error.retryable)
        assertEquals("req-422", error.requestId)
        assertEquals(habitReasons, error.fieldErrors)
        assertEquals("Title can't be blank; Days must include at least one day", error.hint)
        assertEquals(1, hey.requests.size)
    }

    @Test
    fun aRawWriteRefusedKeepsEachReasonHeyGave() = runTest {
        val hey = mockHey(status(422, habitRefusal))
        val client = hey.client()

        val error = assertFailsWith<HeyException.Validation> {
            client.execute(client.request(Method.POST, "/calendar/habits.json"))
        }

        assertEquals(habitReasons, error.fieldErrors)
    }

    @Test
    fun everyShapeOfReasonReads() {
        val cases = listOf(
            """{"errors":[{"field":"subject","message":"is too long"},{"attribute":"content","message":"can't be blank"}]}""" to
                listOf(FieldError("subject", "is too long"), FieldError("content", "can't be blank")),
            """{"errors":["Title can't be blank", 7, {"field":"days"}, "", null]}""" to
                listOf(FieldError(null, "Title can't be blank")),
            """{"error":"Subject can't be blank"}""" to listOf(FieldError(null, "Subject can't be blank")),
            """{"errors":{"subject":["is too long"]},"message":"Invalid message"}""" to listOf(FieldError(null, "Invalid message")),
            "" to emptyList(),
            "<html>Unprocessable</html>" to emptyList(),
            """{"errors":[]}""" to emptyList(),
        )
        for ((body, expected) in cases) {
            val error = refused(body)
            check(error is HeyException.Validation) { "$body mapped to $error" }
            assertEquals(expected, error.fieldErrors, body)
        }
    }

    @Test
    fun aReasonNamingItsFieldReadsWithIt() {
        val error = refused("""{"errors":[{"field":"subject","message":"is too long"}]}""")

        assertEquals("subject: is too long", error.hint)
        assertEquals("subject: is too long", (error as HeyException.Validation).fieldErrors.single().toString())
    }

    @Test
    fun aHintIsItsFieldErrorsJoined() {
        val cases = listOf(
            """{"error":"Invalid habit","errors":["Title can't be blank"]}""" to "Title can't be blank",
            """{"error":"Invalid habit","message":"Try again"}""" to "Invalid habit",
            """{"errors":[{"field":7,"message":"is invalid"}]}""" to "is invalid",
            """{"errors":"wrong","message":"Invalid"}""" to "Invalid",
        )
        for ((body, hint) in cases) {
            val error = refused(body) as HeyException.Validation
            assertEquals(hint, error.hint, body)
            assertEquals(hint, error.fieldErrors.joinToString("; "), body)
        }
    }

    @Test
    fun aRefusalTheSdkRaisesItselfHasNoFieldErrors() {
        assertTrue(HeyException.Validation("clearance status must be \"approved\" or \"denied\"").fieldErrors.isEmpty())
    }
}
