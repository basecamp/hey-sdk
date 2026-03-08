$version: "2"

namespace hey.traits

use smithy.api#documentation
use smithy.api#trait
use smithy.openapi#specificationExtension

// ============================================================================
// Bridge Traits — emit x-hey-* extensions to OpenAPI
// ============================================================================

/// Retry semantics for HEY API operations.
/// Emits x-hey-retry extension to OpenAPI for SDK code generators.
@trait(selector: "operation")
@specificationExtension(as: "x-hey-retry")
structure heyRetry {
    /// Maximum number of retry attempts (default: 3)
    maxAttempts: Integer

    /// Base delay in milliseconds between retries (default: 1000)
    baseDelayMs: Integer

    /// Backoff strategy: "exponential" | "linear" | "constant"
    backoff: String

    /// HTTP status codes that trigger a retry (e.g., [429, 503])
    retryOn: HeyRetryStatusCodes
}

list HeyRetryStatusCodes {
    member: Integer
}

/// Pagination semantics for HEY list operations.
/// Emits x-hey-pagination extension to OpenAPI for SDK code generators.
@trait(selector: "operation")
@specificationExtension(as: "x-hey-pagination")
structure heyPagination {
    /// Pagination style: "link" (Link header RFC5988) or "window" (date-windowed)
    style: String

    /// Name of the response header containing total count
    totalCountHeader: String

    /// Maximum items per page (server default)
    maxPageSize: Integer
}

/// Idempotency semantics for HEY write operations.
/// Emits x-hey-idempotent extension to OpenAPI for SDK code generators.
@trait(selector: "operation")
@specificationExtension(as: "x-hey-idempotent")
structure heyIdempotent {
    /// Whether the operation supports client-provided idempotency keys
    keySupported: Boolean

    /// Header name for idempotency key (if supported)
    keyHeader: String

    /// Whether the operation is naturally idempotent (same input = same result)
    natural: Boolean
}

/// Marks members containing sensitive data that should not be logged.
/// Emits x-hey-sensitive extension to OpenAPI for SDK code generators.
@trait(selector: "structure > member")
@specificationExtension(as: "x-hey-sensitive")
structure heySensitive {
    /// Category of sensitive data: "pii", "credential", "financial"
    category: String

    /// Whether the value should be redacted in logs (default: true)
    redact: Boolean
}

/// Polymorphic shape metadata for types discriminated by a field.
/// Emits x-hey-polymorphic extension to OpenAPI for SDK code generators.
///
/// Used for Posting (discriminated by `kind`) and Recording (discriminated by `type`).
/// See ADR-001 for the per-language generation strategy.
@trait(selector: "structure")
@specificationExtension(as: "x-hey-polymorphic")
structure heyPolymorphic {
    /// The field name used as the discriminator (e.g., "kind", "type")
    @required
    discriminator: String

    /// Map of discriminator values to lists of variant-specific field names
    @required
    variants: HeyPolymorphicVariants
}

map HeyPolymorphicVariants {
    key: String
    value: HeyPolymorphicFieldList
}

list HeyPolymorphicFieldList {
    member: String
}

/// Marks an operation where specific HTTP status codes indicate an empty/nil
/// result rather than an error. See ADR-004.
/// Emits x-hey-empty-on extension to OpenAPI for SDK code generators.
@trait(selector: "operation")
@specificationExtension(as: "x-hey-empty-on")
structure heyEmptyOn {
    /// HTTP status codes that should be treated as "no result" (e.g., [404])
    @required
    statusCodes: HeyEmptyOnStatusCodes
}

list HeyEmptyOnStatusCodes {
    member: Integer
}
