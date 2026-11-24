// SideSign（SideStore の署名）と同じ手順で CodeSigner.sign を呼ぶ。
// entitlementProvider は SideSign の AppBundleSigner と同じ振り分け：
//   本体・本体内の実行ファイル・.appex 以外の束（.xctest を含む）→ 本体の entitlements
//   .framework / .dylib → なし
// 使い方：SignCompare <App.app> <p12> <entitlements.plist>
import Foundation
import CodeSignKit

let a = CommandLine.arguments
guard a.count >= 4 else { print("usage: SignCompare <app> <p12> <ent>"); exit(2) }
let appURL = URL(fileURLWithPath: a[1]).resolvingSymlinksInPath()
let p12 = try Data(contentsOf: URL(fileURLWithPath: a[2]))
let ent = try String(contentsOfFile: a[3], encoding: .utf8)

try CodeSigner.sign(
    appPath: appURL.path,
    keyData: p12,
    password: "test",
    entitlementProvider: { path in
        let p = path.trimmingCharacters(in: .whitespacesAndNewlines)
        let kind: String
        let xml: String
        if p.isEmpty {
            kind = "main"; xml = ent
        } else if p.contains(".appex") {
            kind = "appex"; xml = ent
        } else if p.hasSuffix(".framework") || p.contains(".framework") {
            kind = "framework"; xml = ""
        } else if p.hasSuffix(".dylib") {
            kind = "dylib"; xml = ""
        } else {
            kind = "mainAppExecutable"; xml = ent
        }
        print("provider: '\(p)' -> \(kind) ent=\(xml.isEmpty ? "none" : "main")")
        return xml
    },
    progress: {}
)
print("signed \(appURL.path)")
