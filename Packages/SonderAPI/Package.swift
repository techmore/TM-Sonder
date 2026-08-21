// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "SonderAPI",
    platforms: [
        .macOS(.v14),
        .iOS(.v17)
    ],
    products: [
        .library(name: "SonderAPI", targets: ["SonderAPI"])
    ],
    targets: [
        .target(
            name: "SonderAPI",
            path: "Sources/SonderAPI"
        ),
        .testTarget(
            name: "SonderAPITests",
            dependencies: ["SonderAPI"],
            path: "Tests/SonderAPITests"
        )
    ]
)
