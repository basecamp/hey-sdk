# HEY SDK -- Agent Instructions

## Hard Rules

1. **Never hand-write API methods.** All operations are generated from the Smithy spec.
2. **Never construct URL paths manually.** Use the generated route table.
3. **Every new operation needs tests.** Unit tests per language + conformance tests.
4. **Run `make check` before committing.** All checks must pass.

## Pipeline

```
Smithy spec -> OpenAPI -> Behavior Model -> Per-language generators -> SDK code
```

## Anti-Patterns

- Editing `openapi.json` directly (it is generated from Smithy)
- Adding API methods without updating the Smithy spec
- Skipping conformance tests for behavioral changes
- Using `fmt.Sprintf` or template literals for API paths

## Development Workflow

1. Edit the Smithy spec in `spec/`
2. Run `make smithy-build` to regenerate OpenAPI
3. Run per-language generators: `make {lang}-generate-services`
4. Add/update tests
5. Run `make check`
6. Commit
