// swift-tools-version:5.9
// iPhone に依存しないロジックだけを置くパッケージ。
// Windows / Linux / Mac のどこでも `swift test` で確かめられるようにする。
import PackageDescription

let package = Package(
    name: "DevKitCore",
    platforms: [.iOS(.v17), .macOS(.v13)],
    products: [
        .library(name: "DevKitCore", targets: ["DevKitCore"])
    ],
    targets: [
        .target(name: "DevKitCore"),
        .testTarget(name: "DevKitCoreTests", dependencies: ["DevKitCore"])
    ]
)
