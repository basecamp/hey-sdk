import ConformanceSupport
import Foundation
import Hey

// The conformance runner for the HEY Swift SDK. It reads the shared case definitions from
// conformance/tests and runs each one against the SDK, with a loopback mock server standing in
// for HEY.

let directory = URL(fileURLWithPath: CommandLine.arguments.dropFirst().first ?? "../../tests")
let files: [URL]
do {
    files = try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)
        .filter { $0.pathExtension == "json" }
        .sorted { $0.lastPathComponent < $1.lastPathComponent }
} catch {
    FileHandle.standardError.write(Data("Error finding test files: \(directory.path) is not a directory\n".utf8))
    exit(1)
}
if files.isEmpty {
    print("No test files found in \(directory.path)")
    exit(0)
}

var passed = 0
var failed = 0
for file in files {
    print("\n=== \(file.lastPathComponent) ===")
    // A file the runner cannot read is a failure, not a file with no cases in it.
    let cases: [TestCase]
    do {
        cases = try TestCase.load(String(contentsOf: file, encoding: .utf8))
    } catch {
        failed += 1
        print("  FAIL: \(file.path)\n        \(error)")
        continue
    }
    for testCase in cases {
        do {
            try await runCase(testCase)
            passed += 1
            print("  PASS: \(testCase.name)")
        } catch {
            failed += 1
            print("  FAIL: \(testCase.name)\n        \(error)")
        }
    }
}

print("\n=== Summary ===")
print("Passed: \(passed), Failed: \(failed), Total: \(passed + failed)")
exit(failed > 0 ? 1 : 0)

func runCase(_ testCase: TestCase) async throws {
    if let baseURL = testCase.configOverrides.baseURL {
        try await runConfigOverrideCase(testCase, baseURL)
    } else {
        let server = try MockHEY(testCase.mockResponses)
        let outcome = await executeCase(testCase, server.baseURL)
        let recorded = server.shutdown()
        try checkAll(Run(testCase: testCase, outcome: outcome, recorded: recorded, baseURL: server.baseURL))
    }
}

/// A case that overrides the base URL never reaches a server: it asks what the client does with an
/// endpoint it should refuse.
func runConfigOverrideCase(_ testCase: TestCase, _ baseURL: String) async throws {
    let client = await capture { try await clientFor(testCase, baseURL) }
    for assertion in testCase.assertions {
        switch assertion.type {
        case "requestCount":
            let expected = assertion.expected.int
            if expected != 0 {
                throw AssertionFailure("Expected 0 requests for config override test, got expectation of \(expected.map(String.init) ?? "nil")")
            }
        case "errorCode":
            guard case let .failure(error) = client else {
                throw AssertionFailure("Expected configuration error, but client was created successfully")
            }
            let code = (error as? HeyError)?.code
            if code != assertion.expected.string {
                throw AssertionFailure("Expected error code \"\(assertion.expected.string ?? "")\", got \"\(code ?? "\(error)")\"")
            }
        case "noError":
            if case let .failure(error) = client { throw AssertionFailure("Expected no error, got: \(error)") }
        default:
            throw AssertionFailure("Unknown assertion type: \(assertion.type)")
        }
    }
}

/// What `body` answered or threw.
func capture<T>(_ body: () async throws -> T) async -> Result<T, any Error> {
    do {
        return .success(try await body())
    } catch {
        return .failure(error)
    }
}
