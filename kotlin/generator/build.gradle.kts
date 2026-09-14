plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
    application
}


application {
    mainClass.set("com.basecamp.hey.generator.MainKt")
}

// The generator reads openapi.json, behavior-model.json and its names.toml relative to the
// repository root, which is the parent of this Gradle build.
tasks.named<JavaExec>("run") {
    workingDir = rootProject.projectDir.parentFile
}

tasks.withType<Test>().configureEach {
    useJUnitPlatform()
}

dependencies {
    implementation(libs.kotlinx.serialization.json)
    testImplementation(kotlin("test"))
}
