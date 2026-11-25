# iphone-dev-kit

Windows と Claude で iPhone アプリを作るための道具箱です。Mac は使いません。
コードは GitHub に置き、ビルドは GitHub Actions のクラウドの Mac で行い、iPhone には SideStore / LiveContainer で入れます。

## 中身

| フォルダ | 役割 | 確かめ方 |
| --- | --- | --- |
| `Core/` | iPhone に依存しないロジック | `swift test`（Linux / Windows / Mac） |
| `App/` | iPhone の画面と機能（SwiftUI） | クラウドのシミュレーター、実機 |
| `App/UITests/` | シミュレーターで実際に操作するテスト | Actions で自動実行、スクリーンショットを保存 |
| `scripts/` | Actions で使う補助スクリプト。`scripts/pc/` は PC で実機を見る・動かす台本（go.bat + plan.json） | |
| `docs/` | 手順書（OPERATIONS.md）と記録（LOG.md） | |
| `tools/iphone-mcp/` | Claude が iPhone を見て・動かす道具（Claude デスクトップの拡張機能。手順は docs/MCP.md） | Actions で試験 → Releases `mcp-N` |
| `tools/pc-mcp/` | Claude が PC を見て・動かす道具（拡張機能＋常駐役。手順は docs/PC.md） | Actions で Windows の通し試験 → Releases `pc-N` |
| `tools/switch-app/` | カオルが Claude に任せる範囲（オフ・見るだけ・操作も可・10 分許可）を切り替える窓。「許可しますか？」もここから出る | PC で確かめた |
| `tools/lc-install/`・`tools/wda-sidestore/` | LiveContainer への入れ替え・WebDriverAgent を SideStore 向けにする道具 | |

## 試作1：スワイプ

画面をスワイプすると、向き（下スワイプ・上スワイプなど）が1行ずつ出ます。
同じ内容が iPhone のシステムログに `DEVKIT swipe=...` として出るので、PC の go-ios で確かめられます。
画面下の端からのスワイプは、1回目をアプリが受け取ります（2回目でホームに戻る）。

## iPhone に入れる

1. iPhone の Safari で最新の IPA をダウンロード：
   `https://github.com/iwataka793/iphone-dev-kit/releases/latest/download/DevKit.ipa`
2. LiveContainer（または SideStore）でその IPA を開いて入れる
3. 画面下の `build:` の文字が、GitHub の最新コミットの番号と同じなら最新版

## 自動ビルド

- `Core/` を変えると：Linux でロジックのテスト（`core-tests.yml`）
- `App/` か `Core/` を変えて main に push すると：Mac でシミュレーターの UI テスト → 署名なし IPA → Releases（`ios.yml`）

## 決まりごと

- この リポジトリは公開。パスワード、トークン、API キー、会社や仕事の情報、個人情報は入れない（秘密は GitHub の Secrets に置く）
- 頼まれていない機能は足さない
- 分かったこと・変えたことは `docs/LOG.md` に書く
