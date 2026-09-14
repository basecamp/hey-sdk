plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.kotlin.serialization)
    `maven-publish`
}

group = "com.basecamp"
version = "0.31.0"

kotlin {
    jvm {
        // The generated common+jvm main sources compile with every warning promoted to an
        // error, so a deprecated call or an unused parameter fails the build rather than
        // scrolling past.
        compilations.named("main") {
            compileTaskProvider.configure {
                compilerOptions {
                    allWarningsAsErrors.set(true)
                }
            }
        }
    }

    sourceSets {
        commonMain.dependencies {
            api(libs.ktor.client.core)
            api(libs.kotlinx.serialization.json)
            implementation(libs.kotlinx.coroutines.core)
        }
        jvmMain.dependencies {
            implementation(libs.ktor.client.cio)
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
            implementation(libs.ktor.client.mock)
            implementation(libs.kotlinx.coroutines.test)
        }
        jvmTest.dependencies {
            implementation(libs.junit.jupiter)
        }
    }
}

tasks.withType<Test> {
    useJUnitPlatform()
}

// The same commit and toolchain build the same bytes, so the release workflow can tell a
// re-run that finds the version already on GitHub Packages from one that finds something
// else there: zip entry timestamps and directory order are the only things that would differ.
tasks.withType<AbstractArchiveTask>().configureEach {
    isPreserveFileTimestamps = false
    isReproducibleFileOrder = true
}

publishing {
    repositories {
        maven {
            name = "GitHubPackages"
            url = uri("https://maven.pkg.github.com/basecamp/hey-sdk")
            credentials {
                username = System.getenv("GITHUB_USER") ?: "x-access-token"
                password = System.getenv("GITHUB_ACCESS_TOKEN") ?: ""
            }
        }
        // Every file the publication is made of, laid out as GitHub Packages will hold it. The
        // release workflow publishes here first and checks each one against the remote before
        // and after publishing, since a Maven version there is several files and a partial
        // upload is one it can neither finish nor overwrite.
        maven {
            name = "staging"
            url = uri(layout.buildDirectory.dir("staging-repo"))
        }
    }
}
