# ADR-001: Polymorphic Shape Modeling

## Status

Accepted

## Context

HEY's API has two polymorphic types:

- **Posting** — discriminated by `kind` (topic, bundle, entry). Variant fields merged at the top level.
- **Recording** — discriminated by `type` (CalendarEvent, CalendarHabit, CalendarTodo, CalendarTimeTrack, CalendarJournalEntry). Variant fields merged at the top level.

Every SDK generator must handle these types. Unlike discriminated unions with envelope wrappers, HEY merges all variant fields into a single flat object. This means a `Posting` response includes fields for topics, bundles, *and* entries — only a subset populated depending on `kind`.

## Decision

Use `@mixin` composition in Smithy to document which fields belong to which variant. The OpenAPI output is a flat schema with all optional fields plus an `x-hey-polymorphic` extension that carries the discriminator and variant mapping.

Each generator reads this extension:

| Language | Strategy |
|----------|----------|
| **Go** | Flat struct. Discriminator-aware accessor helpers generated. |
| **Ruby** | Flat struct. Accessor helpers. |
| **TypeScript** | `openapi-typescript` produces the flat type. `generate-services.ts` emits discriminated union type guards from `x-hey-polymorphic`. |
| **Swift** | `HEYGenerator` reads the extension and emits enum with associated values. |
| **Kotlin** | Kotlin generator reads the extension and emits sealed class hierarchy. |

## Consequences

- Smithy model uses `@mixin` and `@heyPolymorphic` trait — not Smithy unions (which would produce `oneOf` and break flat-object semantics).
- All variant-specific fields are optional in the generated types. Type narrowing happens at the accessor/guard layer.
- The `x-hey-polymorphic` extension is the contract between the Smithy model and per-language generators.
