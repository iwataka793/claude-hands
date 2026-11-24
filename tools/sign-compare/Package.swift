// swift-tools-version:5.9
// SideStore の署名部品（CodeSignKit）で WebDriverAgent に署名して、Apple の codesign の結果と比べるための小さな道具。
import PackageDescription
let package = Package(
    name: "SignCompare",
    platforms: [.macOS(.v13)],
    dependencies: [.package(url: "https://github.com/mahee96/CodeSignKit.git", revision: "d0c6771")],
    targets: [.executableTarget(name: "SignCompare", dependencies: [.product(name: "CodeSignKit", package: "CodeSignKit")])]
)
