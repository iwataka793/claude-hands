# 経過の記録

新しいものが上。

## 2026-11-24（PC の準備）

- Windows の「スマート アプリ コントロール」が署名のない iloader を止めた → オフにした（2026年4月の更新以降はオンに戻せる）
- iloader 2.4.2 をインストール（公式ハッシュと一致を確認）。iPhone を認識
- go-ios 1.3.2 を `%USERPROFILE%\devkit\go-ios` に配置（公式ハッシュと一致）。wintun 0.14.1 は WireGuard の署名を確認して ios.exe の隣に置いた
- go-ios で iOS 27.2.1 の端末情報・デベロッパモードの状態・言語が読めた（トンネル不要の範囲）。デベロッパモードはまだオフ
- Windows 版 Swift 6.4.3 と VS 2022 Build Tools（MSVC 14.44、Windows SDK 10.0.26100）を winget で導入
- `swift test` は VS の環境（vcvars64.bat）を読み込んでから実行する必要がある。Core のテスト7件が通過：1回目42秒（ビルド込み）、2回目4秒
- PC のターミナルには Claude から入力できないため、スクリプトを `Downloads\devkit-setup` に置いてエクスプローラーから実行する形にした
- 署名用 Apple ID の作成で「現在アカウントを作成できません」（電話番号の段階）→ 時間を置いて再挑戦

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
