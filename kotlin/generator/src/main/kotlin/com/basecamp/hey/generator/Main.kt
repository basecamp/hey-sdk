package com.basecamp.hey.generator

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import java.io.File
import kotlin.system.exitProcess

/**
 * Generates the Kotlin HEY SDK's models, routes and services.
 *
 * Reads `openapi.json`, `behavior-model.json` and `kotlin/generator/names.toml` from the
 * repository root and writes `kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated/`. With
 * `--check` it writes nothing and exits non-zero when the checked-in files differ from what
 * it would generate.
 */
fun main(args: Array<String>) {
    var check = false
    var root = File(".").canonicalFile
    var index = 0
    while (index < args.size) {
        when (val argument = args[index]) {
            "--check" -> check = true
            "--root" -> {
                index++
                root = File(args.getOrNull(index) ?: fail("--root needs a path")).canonicalFile
            }
            else -> fail("unknown argument $argument")
        }
        index++
    }

    try {
        val files = generate(root)
        val target = File(root, GENERATED_DIRECTORY)
        if (check) verify(target, files) else write(target, files)
    } catch (error: GeneratorException) {
        fail(error.message ?: "generation failed")
    }
}

const val GENERATED_DIRECTORY = "kotlin/sdk/src/commonMain/kotlin/com/basecamp/hey/generated"

private val json = Json { ignoreUnknownKeys = true }

fun generate(root: File): Map<String, String> {
    val openapi = readJson(File(root, "openapi.json"))
    val behavior = readJson(File(root, "behavior-model.json"))
    val naming = Naming.parse(File(root, "kotlin/generator/names.toml").readText())
    val model = Model.build(openapi, behavior, naming)
    return render(model)
}

private fun readJson(file: File): JsonObject {
    if (!file.exists()) throw GeneratorException("${file.path} does not exist")
    return json.parseToJsonElement(file.readText()) as? JsonObject
        ?: throw GeneratorException("${file.path} is not a JSON object")
}

private fun write(target: File, files: Map<String, String>) {
    target.walkBottomUp().filter { it.isFile && it.extension == "kt" }.forEach { it.delete() }
    for ((path, content) in files) {
        val file = File(target, path)
        file.parentFile.mkdirs()
        file.writeText(content)
    }
    println("Generated ${files.size} files into ${target.path}")
}

private fun verify(target: File, files: Map<String, String>) {
    val stale = mutableListOf<String>()
    for ((path, content) in files) {
        val file = File(target, path)
        if (!file.exists()) {
            stale += "$path is missing"
        } else if (file.readText() != content) {
            stale += "$path differs"
        }
    }
    val expected = files.keys.toSet()
    target.walkTopDown().filter { it.isFile && it.extension == "kt" }.forEach { file ->
        val relative = file.relativeTo(target).path.replace(File.separatorChar, '/')
        if (relative !in expected) stale += "$relative is not generated any more"
    }
    if (stale.isNotEmpty()) {
        System.err.println("error: kotlin/sdk generated code is out of date; run `make kt-generate`")
        stale.forEach { System.err.println("  $it") }
        exitProcess(1)
    }
    println("Generated Kotlin code is up to date (${files.size} files)")
}

private fun fail(message: String): Nothing {
    System.err.println("error: $message")
    exitProcess(1)
}
