rootProject.name = "hey-sdk-kotlin"

dependencyResolutionManagement {
    repositories {
        mavenCentral()
    }
}

include(":hey-sdk")
project(":hey-sdk").projectDir = file("sdk")
include(":generator")
// The conformance runner lives with the other runners under conformance/runner, as the
// Go and Rust ones do, and is built as a subproject of this build so it shares the
// wrapper, the version catalog and the SDK it drives.
include(":conformance")
project(":conformance").projectDir = file("../conformance/runner/kotlin")
