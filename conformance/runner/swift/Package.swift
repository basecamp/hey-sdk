// swift-tools-version: 6.0
// The conformance runner for the Swift SDK. It depends on the library through the distribution
// manifest at the repository root rather than swift/Package.swift: Swift Package Manager names a
// package by its directory, and this one's directory is also called swift.
import PackageDescription

let package = Package(
    name: "ConformanceRunner",
    platforms: [
        .macOS(.v13),
    ],
    dependencies: [
        .package(name: "Hey", path: "../../.."),
    ],
    targets: [
        // Reading fixtures, checking assertions and serving the loopback mock: kept apart from
        // the executable so its branches can be unit-tested.
        .target(
            name: "ConformanceSupport",
            dependencies: [.product(name: "Hey", package: "Hey")],
            path: "Sources/ConformanceSupport",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .executableTarget(
            name: "ConformanceRunner",
            dependencies: ["ConformanceSupport", .product(name: "Hey", package: "Hey")],
            path: "Sources/ConformanceRunner",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        // The assertion helpers, and the library's own transport driven over a real loopback
        // connection: bounded bodies, refused redirects, cancellation.
        .testTarget(
            name: "ConformanceSupportTests",
            dependencies: ["ConformanceSupport", .product(name: "Hey", package: "Hey")],
            path: "Tests/ConformanceSupportTests",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
