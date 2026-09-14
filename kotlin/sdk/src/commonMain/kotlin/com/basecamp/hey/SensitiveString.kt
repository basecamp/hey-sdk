package com.basecamp.hey

import kotlinx.serialization.Serializable

/**
 * A string the model marks as sensitive — an email address, a token — which prints as
 * `[REDACTED]` so that a `toString()` of anything holding one cannot put it in a log. The
 * value itself is a call to [expose] away, and goes over the wire as the plain string it is.
 */
@Serializable
@JvmInline
value class SensitiveString(private val value: String) {
    /** The string itself. */
    fun expose(): String = value

    /** Whether there is nothing behind the redaction. */
    val isEmpty: Boolean get() = value.isEmpty()

    override fun toString(): String = "[REDACTED]"
}
