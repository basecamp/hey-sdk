plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    application
}


application {
    mainClass.set("com.basecamp.hey.conformance.MainKt")
}

// The runner reads conformance/tests relative to the repository root, the parent of the
// kotlin/ Gradle build it belongs to.
tasks.named<JavaExec>("run") {
    workingDir = rootProject.projectDir.parentFile
}

tasks.withType<Test>().configureEach {
    useJUnitPlatform()
    workingDir = rootProject.projectDir.parentFile
}

dependencies {
    implementation(project(":hey-sdk"))
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.core)

    testImplementation(kotlin("test"))
}
