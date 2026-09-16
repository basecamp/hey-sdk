import Foundation

// Generates the Swift HEY SDK's models, routes and services.
//
// Reads `openapi.json`, `behavior-model.json` and `swift/names.toml` from the repository root
// and writes `swift/Sources/Hey/Generated`. With `--check` it writes nothing and exits non-zero
// when the checked-in files differ from what it would generate.

var check = false
var root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
var arguments = CommandLine.arguments.dropFirst().makeIterator()
while let argument = arguments.next() {
    switch argument {
    case "--check":
        check = true
    case "--root":
        guard let path = arguments.next() else {
            FileHandle.standardError.write(Data("error: --root needs a path\n".utf8))
            exit(1)
        }
        root = URL(fileURLWithPath: path, relativeTo: root).standardizedFileURL
    default:
        FileHandle.standardError.write(Data("error: unknown argument \(argument)\n".utf8))
        exit(1)
    }
}

do {
    let files = try generate(root: root)
    let target = root.appendingPathComponent(generatedDirectory)
    if check {
        let problems = stale(files, at: target)
        if !problems.isEmpty {
            var message = "error: swift/Sources/Hey/Generated is out of date; run `make swift-generate`\n"
            for problem in problems { message += "  \(problem)\n" }
            FileHandle.standardError.write(Data(message.utf8))
            exit(1)
        }
        print("Generated Swift code is up to date (\(files.count) files)")
    } else {
        try write(files, to: target)
        print("Generated \(files.count) files into \(target.path)")
    }
} catch {
    FileHandle.standardError.write(Data("error: \(error)\n".utf8))
    exit(1)
}
