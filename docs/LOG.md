# 経過の記録

新しいものが上。

## 2026-11-24（自動ビルドの確認）

- Linux：Swift 6.4.2 でスワイプ判定のテスト7件が通過（約1分）
- Mac：Xcode 26.6、シミュレーター iPhone Air / iOS 26.5 で UI テスト（上下スワイプ）が通過。1回あたり約8分
- 署名なし IPA（約27KB）を Releases に配置。build-2 からスクリーンショットとテスト記録も同じ Release に置く
- Actions の「成果物（artifact）」は Claude の作業環境からダウンロードできない。Releases のファイルはダウンロードできる → 確認用のファイルは Releases に置く
- スクリーンショットで、スワイプのたびに「下スワイプ」「上スワイプ」の行が増えることを Claude が確認

## 2026-11-24

- リポジトリを作成。Core（スワイプの判定）、App（スワイプ試作）、UI テスト、Actions（Linux でロジックのテスト、Mac でシミュレーター確認と IPA 作成）を用意
- 対象の実機：iPhone 16e / iOS 27.2.1
- まだ確かめていないこと
  - Actions の Mac でのビルドとシミュレーターの UI テストが通るか
  - SideStore / LiveContainer で入れて実機で動くか（iOS 27 はペアリングのエラー報告あり）
  - 無料アカウントで拡張機能・App Groups が使えるか
  - go-ios（Windows）が iOS 27 で動くか
