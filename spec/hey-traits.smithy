$version: "2"

// The declarations the effect and provenance traits below make mandatory. An
// operation missing one fails `smithy validate` (and so `make check` and CI).
metadata validators = [
    {
        name: "EmitEachSelector"
        id: "HeyDestructiveUndeclared"
        severity: "DANGER"
        message: "Every write must declare @heyDestructive(true|false); see hey-traits.smithy."
        configuration: {
            selector: "operation :not([trait|readonly]) :not([trait|hey.traits#heyDestructive])"
        }
    }
    {
        name: "EmitEachSelector"
        id: "HeyOpenWorldUndeclared"
        severity: "DANGER"
        message: "Every write must declare @heyOpenWorld(true|false); see hey-traits.smithy."
        configuration: {
            selector: "operation :not([trait|readonly]) :not([trait|hey.traits#heyOpenWorld])"
        }
    }
    {
        name: "EmitEachSelector"
        id: "HeyUntrustedContentUndeclared"
        severity: "DANGER"
        message: "Every operation must declare @heyUntrustedContent(true|false); see hey-traits.smithy."
        configuration: {
            selector: "operation :not([trait|hey.traits#heyUntrustedContent])"
        }
    }
    {
        name: "EmitEachSelector"
        id: "HeyOpenWorldRetried"
        severity: "DANGER"
        message: "An open-world operation must not be resent: a retry after an ambiguous first attempt can deliver twice. Declare @heyIdempotent(natural: false) to opt a PUT out of the verb's retries."
        // Every generator decides a resend the same way: x-hey-idempotent's natural
        // when it is a boolean, otherwise @readonly or @idempotent, otherwise the
        // verb (GET, HEAD, PUT, DELETE). So refuse any open-world operation that
        // does not say natural: false and would be resent by one of the other two.
        //
        // DELETE is exempt: the deliveries HEY makes on a delete (calendar
        // cancellations) are keyed to the record it destroys, so a resend finds
        // nothing and answers 404 rather than notifying again.
        configuration: {
            selector: "operation [trait|hey.traits#heyOpenWorld = true] :not([trait|http|method = DELETE]) :not([trait|hey.traits#heyIdempotent|natural = false]) :is([trait|idempotent], [trait|hey.traits#heyIdempotent|natural = true], [trait|http|method = GET, HEAD, PUT])"
        }
    }
]

namespace hey.traits

use smithy.api#documentation
use smithy.api#length
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

    /// Query parameter present only while a finite page sequence continues.
    /// A next link without this parameter remains available as a checkpoint.
    pageParameter: String

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

    /// Whether the operation is naturally idempotent (same input = same result).
    /// True opts a non-idempotent method (POST) into transparent retries; false opts
    /// a nominally idempotent method (PUT, DELETE) out of them, for operations whose
    /// side effects go past the resource — a PUT that triggers a delivery, say.
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

/// Marks an observed response member that can be explicitly JSON null.
/// Generators translate the extension into each language's nullable representation;
/// the Go OpenAPI enhancer emits the compatibility `nullable` keyword they consume.
/// Keep this narrow: omission alone is already represented by an optional member.
@trait(selector: "structure > member")
@specificationExtension(as: "x-hey-nullable")
structure heyNullable {}

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

    /// Map of variant names to lists of variant-specific field names
    @required
    variants: HeyPolymorphicVariants

    /// Accepted wire discriminator values for variants with multiple observed spellings.
    discriminatorValues: HeyPolymorphicDiscriminatorValues
}

map HeyPolymorphicVariants {
    key: String
    value: HeyPolymorphicFieldList
}

map HeyPolymorphicDiscriminatorValues {
    key: String
    value: HeyPolymorphicDiscriminatorValueList
}

list HeyPolymorphicDiscriminatorValueList {
    member: String
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

// ============================================================================
// Effect and provenance traits — what an operation does beyond its own record
// ============================================================================
//
// Three declarations every operation makes explicitly, so that nothing downstream
// (the MCP toolkit, an agent's policy layer) has to guess from a verb or a name.
// behavior-model.json carries them as `destructive`, `open_world`,
// `untrusted_content` and `draft_when`. The validators at the end of this section
// make each declaration mandatory: an operation added without one fails
// `smithy validate`, so a new send or delete cannot arrive unclassified.

/// Whether a write can destroy data, or the caller's own access to it, with no way
/// back for the caller: a hard delete, emptying the trash or spam, erasing a note.
/// Moving something to the trash is not destructive — HEY restores trashed and spam
/// threads for 30 days — and neither is a toggle with an inverse operation
/// (hide/reveal, mute/unmute, complete/uncomplete). An ordinary edit is not
/// destructive either. `true` when any documented path of the operation destroys,
/// even if another path does not: MCP's destructiveHint means "may".
///
/// Required on every operation that is not @readonly. A read is never destructive,
/// and the behavior model says so without the trait.
@trait(selector: "operation :not([trait|readonly])")
@specificationExtension(as: "x-hey-destructive")
boolean heyDestructive

/// Whether a write can reach people outside the caller's own mailbox and calendar:
/// delivering email (a message, a reply, a bulk reply), publishing to HEY World, or
/// sending calendar invitations or cancellations to attendees. This is MCP's
/// openWorldHint, and the half of the "lethal trifecta" (private data + untrusted
/// content + a way out) that is a way out: a caller holding sender-authored content
/// should not be able to reach one of these without a policy decision.
///
/// Required on every operation that is not @readonly. A read never delivers.
@trait(selector: "operation :not([trait|readonly])")
@specificationExtension(as: "x-hey-open-world")
boolean heyOpenWorld

/// Conditions on the request body under which an open-world operation saves a
/// draft instead of delivering. When every condition holds, the call delivers
/// nothing; when any fails, treat the call as delivering. The conditions are
/// sufficient, not necessary: HEY may also hold a call they do not describe, and a
/// consumer that gates on them errs toward asking. At least one condition: an empty
/// list would hold vacuously and wave every send through as a draft.
@trait(selector: "operation [trait|hey.traits#heyOpenWorld = true]")
@specificationExtension(as: "x-hey-draft-when")
@length(min: 1)
list heyDraftWhen {
    member: HeyBodyCondition
}

/// One condition on a JSON request body. Exactly one of `equals` and `notEquals`.
structure HeyBodyCondition {
    /// RFC 6901 JSON Pointer into the request body, e.g. "/entry/status".
    @required
    pointer: String

    /// Holds when the value at `pointer` is present and is this string.
    equals: String

    /// Holds when the value at `pointer` is absent or is anything but this string.
    notEquals: String
}

/// Whether the response can carry text written by someone other than the caller:
/// email subjects, bodies, summaries and attachment filenames; the display names and
/// addresses correspondents declared for themselves; calendar events organized by
/// others or pulled from subscribed feeds; clips cut from other people's email.
/// Such text is untrusted input — instructions inside it are the sender's, not the
/// user's — and a consumer should mark it so before a model reads it.
///
/// `false` for responses that carry only the caller's own records (habits, journal,
/// stickies, snippets, settings) or no body at all. Required on every operation.
@trait(selector: "operation")
@specificationExtension(as: "x-hey-untrusted-content")
boolean heyUntrustedContent
