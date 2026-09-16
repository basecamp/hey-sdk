plugins {
    alias(libs.plugins.kotlin.multiplatform) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.serialization) apply false
}

// javac defaults to the platform charset, so a Java source in any module would read UTF-8
// prose as US-ASCII under a C locale. No module has one today; pinned once here so the
// first that does inherits it.
subprojects {
    tasks.withType<JavaCompile>().configureEach {
        options.encoding = "UTF-8"
    }
}

// The wrapper's own download is retried, so a transient services.gradle.org fault does not
// fail a CI job before the build starts. This retries the distribution download, not the
// build.
tasks.wrapper {
    retries.set(3)
    retryBackOffMs.set(500)
}
