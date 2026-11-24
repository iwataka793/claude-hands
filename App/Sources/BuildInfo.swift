// GitHub Actions がビルドのときにコミット番号で上書きする。
// 手元でビルドしたときは "dev" のまま。
enum BuildInfo {
    static let commit = "dev"
}
