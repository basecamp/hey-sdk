// swift-tools-version: 6.0
// Distribution manifest: what Swift Package Manager resolves when an app depends on
// https://github.com/basecamp/hey-sdk at a version tag. It builds the library alone; the
// generator and the tests are in swift/Package.swift.
import PackageDescription

let package = Package(
    name: "Hey",
    platforms: [
        .iOS(.v16),
        .macOS(.v13),
    ],
    products: [
        .library(name: "Hey", targets: ["Hey"]),
    ],
    targets: [
        .target(
            name: "Hey",
            path: "swift/Sources/Hey",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
