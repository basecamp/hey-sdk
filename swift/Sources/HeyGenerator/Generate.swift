import Foundation

/// Where the generated sources live, relative to the repository root.
let generatedDirectory = "swift/Sources/Hey/Generated"

/// Reads the model and the naming overrides from the repository at `root` and renders every
/// generated file.
func generate(root: URL) throws -> [(String, String)] {
    let openapi = try readJSON(root.appendingPathComponent("openapi.json"))
    let behavior = try readJSON(root.appendingPathComponent("behavior-model.json"))
    let namesURL = root.appendingPathComponent("swift/names.toml")
    guard let names = try? String(contentsOf: namesURL, encoding: .utf8) else {
        throw GeneratorError("\(namesURL.path) does not exist")
    }
    let model = try Model.build(openapi: openapi, behavior: behavior, naming: try Naming.parse(names))
    return render(model)
}

private func readJSON(_ url: URL) throws -> JSON {
    guard let text = try? String(contentsOf: url, encoding: .utf8) else {
        throw GeneratorError("\(url.path) does not exist")
    }
    return try JSON.parse(text)
}

/// Replaces every generated Swift file under `target` with `files`.
func write(_ files: [(String, String)], to target: URL) throws {
    let manager = FileManager.default
    for existing in swiftFiles(under: target) {
        try manager.removeItem(at: target.appendingPathComponent(existing))
    }
    for (path, content) in files {
        let url = target.appendingPathComponent(path)
        try manager.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try content.write(to: url, atomically: true, encoding: .utf8)
    }
}

/// What is out of date under `target`: a file that is missing, differs, or is no longer
/// generated. Empty when the checked-in tree is exactly what the generator writes.
func stale(_ files: [(String, String)], at target: URL) -> [String] {
    var problems: [String] = []
    let expected = Set(files.map(\.0))
    for (path, content) in files {
        let url = target.appendingPathComponent(path)
        guard let existing = try? String(contentsOf: url, encoding: .utf8) else {
            problems.append("\(path) is missing")
            continue
        }
        if existing != content { problems.append("\(path) differs") }
    }
    for path in swiftFiles(under: target) where !expected.contains(path) {
        problems.append("\(path) is not generated any more")
    }
    return problems
}

/// The Swift files under a directory, as paths relative to it, in order.
func swiftFiles(under directory: URL) -> [String] {
    guard let enumerator = FileManager.default.enumerator(atPath: directory.path) else { return [] }
    var paths: [String] = []
    while let path = enumerator.nextObject() as? String {
        if path.hasSuffix(".swift") { paths.append(path) }
    }
    return paths.sorted()
}
