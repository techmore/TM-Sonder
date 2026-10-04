// swift-tools-version: 6.2
import PackageDescription
import Foundation

// Exercise the actual client persistence code without a simulator or signing.
let sources = ["SonderClientModels.swift", "SonderOfflineDownloads.swift", "SonderAudiobookChapters.swift", "SonderProgressQueue.swift", "SonderEPUB.swift"]
let directory = URL(fileURLWithPath: #filePath).deletingLastPathComponent().appendingPathComponent("TM_Sonder_Client")
let excluded = (try? FileManager.default.contentsOfDirectory(atPath: directory.path))?.filter { !sources.contains($0) } ?? []
let package = Package(
    name: "SonderClientChecks",
    platforms: [.macOS(.v14)],
    dependencies: [.package(path: "../../Packages/SonderAPI"), .package(url: "https://github.com/weichsel/ZIPFoundation.git", exact: "0.9.20")],
    targets: [
        .target(name: "OfflineCore", dependencies: [.product(name: "SonderAPI", package: "SonderAPI"), .product(name: "ZIPFoundation", package: "ZIPFoundation")], path: "TM_Sonder_Client", exclude: excluded, sources: sources),
        .testTarget(name: "OfflineCoreTests", dependencies: ["OfflineCore", .product(name: "SonderAPI", package: "SonderAPI"), .product(name: "ZIPFoundation", package: "ZIPFoundation")], path: "ReleaseTests")
    ]
)
