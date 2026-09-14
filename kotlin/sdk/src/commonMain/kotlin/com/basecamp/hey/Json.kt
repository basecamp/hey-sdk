package com.basecamp.hey

import kotlinx.serialization.json.Json

/**
 * The one JSON configuration every model goes through. Unknown keys are ignored, so a field
 * HEY adds is not a failure; nulls are left off the wire on the way out, so an optional
 * member a caller leaves unset is not sent as `null`; and defaults are written, so a member
 * that happens to hold its default still goes out. There is no `isLenient`, deliberately: a
 * number read into a declared String is a wrong answer, not a lenient one.
 */
@PublishedApi
internal val heyJson: Json = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
    explicitNulls = false
    encodeDefaults = true
}
