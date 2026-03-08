# ADR-003: Hermetic Build

## Status

Accepted

## Context

The Smithy build requires a custom OpenAPI mapper (`smithy-bare-arrays`) to transform wrapped Smithy output structures into bare array/object schemas matching the HEY API's actual response format. This JAR must be available at build time.

Options considered:

1. **Gradle build on every `smithy build`** — adds JDK + Gradle as build prerequisites. Slow. Non-deterministic (dependency resolution).
2. **Publish to Maven Central** — heavyweight for an internal tool. Version coordination overhead.
3. **Vendor the JAR** — commit the built artifact. Deterministic. Zero external dependencies at build time.

## Decision

Vendor the JAR. Commit `spec/lib/smithy-bare-arrays-1.0.0.jar` and reference via `file://` Maven repo in `smithy-build.json`:

```json
{
  "maven": {
    "repositories": [
      { "url": "https://repo1.maven.org/maven2/" },
      { "url": "file://lib" }
    ]
  }
}
```

The vendored artifact lives in a Maven-style directory layout under `spec/lib/com/basecamp/smithy-bare-arrays/1.0.0/`. The `file://lib` URL is relative to the `spec/` directory where `smithy-build.json` lives.

Keep Gradle source in `spec/smithy-bare-arrays/` for rebuilds. The Makefile's `smithy-mapper` target validates the JAR exists but does not rebuild it.

## Consequences

- `smithy build` works with only the Smithy CLI installed — no JDK/Gradle needed for normal development.
- Changing the mapper requires: edit source → `cd spec/smithy-bare-arrays && ./gradlew publishToMavenLocal` → copy JAR to `spec/lib/` → commit.
- The vendored JAR is ~15 KB. Acceptable for a committed artifact.
