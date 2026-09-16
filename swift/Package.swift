// swift-tools-version: 6.0
// Development manifest: the library, its generator and their tests. Consumers resolve the
// library through the distribution manifest at the repository root, which builds the same
// Sources/Hey.
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
            path: "Sources/Hey",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .executableTarget(
            name: "HeyGenerator",
            path: "Sources/HeyGenerator",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .testTarget(
            name: "HeyTests",
            dependencies: ["Hey"],
            path: "Tests/HeyTests",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .testTarget(
            name: "HeyGeneratorTests",
            dependencies: ["HeyGenerator"],
            path: "Tests/HeyGeneratorTests",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
