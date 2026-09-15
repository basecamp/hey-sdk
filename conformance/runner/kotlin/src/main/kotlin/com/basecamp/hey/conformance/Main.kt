package com.basecamp.hey.conformance

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import java.io.File
import kotlin.system.exitProcess

/**
 * The conformance runner for the HEY Kotlin SDK. It reads the shared case definitions from
 * `conformance/tests/` and runs each one against the SDK, with a loopback mock server
 * standing in for HEY.
 */
fun main() {
    val directory = File("conformance/tests")
    val files = directory.listFiles { file -> file.extension == "json" }?.sortedBy { it.name }
    if (files == null) {
        System.err.println("Error finding test files: ${directory.absolutePath} is not a directory")
        exitProcess(1)
    }
    if (files.isEmpty()) {
        println("No test files found in ${directory.absolutePath}")
        return
    }

    val json = Json { ignoreUnknownKeys = true }
    var passed = 0
    var failed = 0
    for (file in files) {
        println("\n=== ${file.name} ===")
        // A file the runner cannot read is a failure, not a file with no cases in it.
        val cases = try {
            json.decodeFromString<List<TestCase>>(file.readText())
        } catch (error: Exception) {
            failed += 1
            println("  FAIL: ${file.path}\n        $error")
            continue
        }
        for (case in cases) {
            val failure = runCatching { runBlocking { runCase(case) } }.exceptionOrNull()
            if (failure == null) {
                passed += 1
                println("  PASS: ${case.name}")
            } else {
                failed += 1
                println("  FAIL: ${case.name}\n        ${failure.message ?: failure}")
            }
        }
    }

    println("\n=== Summary ===")
    println("Passed: $passed, Failed: $failed, Total: ${passed + failed}")
    if (failed > 0) exitProcess(1)
}

private suspend fun runCase(case: TestCase) {
    val baseUrl = case.configOverrides.baseUrl
    if (baseUrl != null) {
        runConfigOverrideCase(case, baseUrl)
    } else {
        runMockServerCase(case)
    }
}

/** A case that overrides the base URL never reaches a server: it asks what the client does with an endpoint it should refuse. */
private suspend fun runConfigOverrideCase(case: TestCase, baseUrl: String) {
    val client = runCatching { clientFor(case, baseUrl) }
    for (assertion in case.assertions) {
        when (assertion.type) {
            "requestCount" -> {
                val expected = (assertion.expected as? kotlinx.serialization.json.JsonPrimitive)?.content?.toLongOrNull()
                if (expected != 0L) throw AssertionFailure("Expected 0 requests for config override test, got expectation of $expected")
            }
            "errorCode" -> {
                val expected = (assertion.expected as? kotlinx.serialization.json.JsonPrimitive)?.content
                val error = client.exceptionOrNull() ?: throw AssertionFailure("Expected configuration error, but client was created successfully")
                val code = (error as? com.basecamp.hey.HeyException)?.code
                if (code != expected) throw AssertionFailure("Expected error code \"$expected\", got \"$code\"")
            }
            "noError" -> client.exceptionOrNull()?.let { throw AssertionFailure("Expected no error, got: $it") }
            else -> throw AssertionFailure("Unknown assertion type: ${assertion.type}")
        }
    }
}

private suspend fun runMockServerCase(case: TestCase) {
    val server = MockServer.start(case.mockResponses)
    val outcome = try {
        executeCase(case, server.baseUrl)
    } finally {
        // The recording is read after shutdown either way.
    }
    val recorded = server.shutdown()
    checkAll(Run(case, outcome, recorded, server.baseUrl))
}
