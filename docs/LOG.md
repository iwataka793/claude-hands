# 経過の記録

新しいものが上。

## 2026-11-24（根本原因：SideStore が「デバッグ用の掃除」を本番でも実行して xctest を消している）

- SideStore の詳しい記録（Verbose Ops Logging）をオンにして入れ直し、記録を PC に取り出した（16-log）
  - 18:15:22 IPA を開いた時点では PlugIns/WebDriverAgentRunner.xctest（中身・WebDriverAgentLib 1,364,480 バイト含む）はちゃんと展開されている
  - 18:15:25 署名する時点の App.app では PlugIns が空。署名（ldid）が問い合わせた場所も本体だけ
  - → 消えたのは「署名の準備（ResignAppOperation の prepare）」の中
- この SideStore は LiveContainer 同梱版：0.6.4-20260714.406+ddf08f58（LiveContainer/SideStore の LiveContainerSupport 系、commit ddf08f58）
  - AltStore/Operations/ResignAppOperation.swift：PlugIns の中で名前に「.xctest」を含むものを削除する処理が `#if DEBUG` の中にある（元は AltStore が自分のデバッグ版のテストを消すためのもの）
  - AltStore.xcodeproj のプロジェクト全体の Release 設定に SWIFT_ACTIVE_COMPILATION_CONDITIONS = "DEBUG BETA" が入っていて、SideStore 本体はそれを引き継ぐ → 本番ビルドでも DEBUG が有効になり、入れるアプリ全部から .xctest が消される
- 回避の案（未検証）：テスト本体のフォルダ名を「.xctest」を含まない名前にした WDA を作る（go-ios は --xctestconfig=名前 で指定できる）
- 直す先：LiveContainer/SideStore（DEBUG の付け方か、削除を SideStore 自身のビルドに限る）

## 2026-11-24（実物で確認：SideStore が署名し直した WDA は PlugIns が空）

- SideStore の「Export Resigned Apps」をオンにして WDA（wda-4）を入れ直し、PC から LiveContainer の書類フォルダ（/Documents/SideStore/Documents/ResignedApps）を取り出した（15-pull）
  - 署名し直した WebDriverAgentRunner-Runner.app の PlugIns/ は空。WebDriverAgentRunner.xctest が丸ごと無い
  - SideStore の記録（ConsoleLogs）でも、iPhone に送った IPA は 174,736 バイト。元の IPA（716,481 バイト）の中で一番大きい WebDriverAgentLib（約 1.4MB）が入っていない大きさ
  - 起動するとエラー 104 が再現（App.app/PlugIns/WebDriverAgentRunner.xctest が無い）
- → 原因は「SideStore が iPhone に送る前に、自分の手元で xctest を落としている」。署名の作り方の違いではない
- 訂正：この SideStore の署名は AltSign + ldid（記録に [AltSign] ALTSigner.performSigning）。CodeSignKit ではなかった。diag-1 の比較は対象違い
- 落ちる段階は「IPA を開いて Cached.app にした後〜署名の前」のどこか。次は SideStore の詳しい記録（Verbose Logging）をオンにして、もう一度入れて段階を特定する

## 2026-11-24（エラー 104 の深掘り：xctest が「置かれていない」とほぼ確定）

- 13-dig（PC）：今の iloader 版 WDA で、わざと無いテスト本体（NoSuchBundle.xctest）を指定して起動した
  - 出たエラーは SideStore 版と同じ：Code 104「Failed to create a bundle instance … Check that the bundle exists on disk.」。下に付く理由（UserInfo）は説明文だけで、ほかには何も無い
  - iPhone のログにも Sandbox（読ませなかった）の行は無し。クラッシュレポートも無し
  - → SideStore 版のエラー 104 は「その場所に xctest のフォルダが無かった」とみてよい
- iloader 版も、入った場所は …/App.app、ID は com.facebook.WebDriverAgentRunner.xctrunner.<チームID> で SideStore 版と同じ。「App.app への改名」は原因ではない（前の見立てを取り消す）
- 受け渡し場所（PublicStaging）には SideStore が作ったフォルダ com.facebook.WebDriverAgentRunner.xctrunner/ が残っていたが、中は空。送った中身はもう無い
- ソースを読んだ範囲（実物はまだ見ていない）：minimuxer はアプリのフォルダにあるファイルを全部そのまま送っていて、選り分けはしていない。なので、SideStore が送る前の「署名し直したアプリ」の段階で xctest が無くなっている疑いが一番強い
- 次：SideStore の設定「Export Resigned App」をオンにして WDA を一度入れると、送ったのと同じ IPA が SideStore の書類フォルダ（ResignedApps）に残る。これを PC で開いて確かめる。iloader 版は上書きされるので、終わったら wda-refresh で戻す
- ついで：16:00 の自動更新は「iPhone が見つからない」で失敗（USB につながっていなかった）。待ち直しが効かずに 3 秒で終わっている。あとで直す

## 2026-11-24（SideStore で WebDriverAgent が動かない原因調べ：署名は白）

- `tools/sign-compare`（Actions の Mac）で、同じ WDA を 3 通りに署名して比べた（Releases の diag-1）
  - A：CodeSignKit（SideStore と同じ署名部品・同じ振り分け）／B：rcodesign／C：Apple の codesign
  - 3 つとも `codesign --verify --deep --strict` で合格。ファイルの並び（_CodeSignature の場所）も同じ。テスト本体（.xctest）は 3 つとも署名済みで、本体の CodeResources にも封じられている
  - A だけの違い：本体を universal 形式で包む／ページ 16KB でハッシュ／テスト本体にも本体の entitlements を付ける。どれも「バンドルを作れない」とは別の種類の失敗になるはずのもの
- 見立て：Error 104（Failed to create a bundle instance）は、署名を調べる前の「その場所にフォルダが無い・読めない」で出る失敗。署名の中身ではなく、SideStore が実際に iPhone へ送る中身（フォルダ名 App.app への改名、ファイルの権限、並び）を疑う
- 次：SideStore が保存している署名済みの WDA を PC 経由で取り出して（読むだけ）、Mac で中身と権限を調べる。残っていなければ SideStore で入れ直す必要があり、その時は iloader の WDA が上書きされる（同じ ID）ので先に確認する

## 2026-11-24（WebDriverAgent の 7 日更新を PC で自動にした）

- SideStore で入れた WebDriverAgent は、何をしても `runwda` が「WebDriverAgentRunner.xctest を作れない（Error 104）」で失敗
  - SideStore のソース（SideSign / CodeSignKit、2026-09 に書き直された署名部品）を読んだ：`.xctest` にも署名し、IPA にも入れている。拡張（`.appex`）以外の ID は書き換えない
  - テスト本体の ID を SideStore の書き換え後に合わせても（…xctrunner.<チームID>.WebDriverAgentRunner）同じ失敗
  - インストール中の syslog（約 2,900 行）にも、テスト本体を捨てた記録は無い。原因は特定できず。SideStore 側の不具合と見ている
  - iloader で上書きすると、SideStore の一覧から消える（SideStore の管理表に載せたまま中身だけ替える作戦は不可）
- 対処：iloader と同じ部品（nab138/isideload 0.5.2）で `tools/wda-refresh`（Windows 用 exe、Actions でビルド、Releases の tools-N）を作った
  - iloader が保存したパスワード（資格情報マネージャー "iloader"）と、iloader の証明書・Anisette の状態（KeyringStorage "iloader"）を使い回す。Anisette は mac-ani.jkcoxson.com
  - 証明書が上限なら取り消さずにエラーで止める（LiveContainer / SideStore の証明書を守る）。2 段階認証を求められたら止める
  - 結果：サインインで 2 段階認証は聞かれず、iPhone（USB）に入った。PC から 4 方向のスワイプも DevKit のログで確認
  - iPhone がスリープ中は usbmuxd に出てこないことがある → 最大 60 秒待つようにした
- PC のタスクスケジューラに「iPhone WebDriverAgent 自動更新」を登録（3 時間ごと。最後の成功から 4 日未満なら何もしない）。記録は `Downloads\devkit-setup\wda-refresh-log.txt`
- メールアドレスとチーム ID はリポジトリに置かない（PC のスクリプトにだけ書く）

## 2026-11-24（PC から実機を操作できた：WebDriverAgent）

- `.github/workflows/wda.yml`：appium/WebDriverAgent（30aac06）を署名なしでビルドし、Releases に `wda-N` として置く（`--latest=false`。DevKit の最新版リンクは変えない）
- 入れ方で3回つまずいた
  1. LiveContainer の「＋」で入れて開く → testmanagerd がなく落ちる（当然。WDA は本体に入れて PC から起動する）
  2. SideStore で入れる → go-ios `runwda` が「WebDriverAgentRunner.xctest を作れない（Error 104）」。同梱の XCTest 一式を外しても同じ
  3. iloader の「Import IPA」→「Extension WebDriverAgentRunner is not part of the main app bundle identifier」で拒否。**テスト本体の ID（com.facebook.WebDriverAgentRunner）が本体の ID（…xctrunner）の下にない**のが原因。SideStore の失敗も同じ理由と見ている
- 対処（wda-4）：IPA 作成時に、dSYM と同梱の XCTest 一式（XC*.framework、Testing.framework、libXCTestSwiftSupport.dylib）を外し、テスト本体の ID を `com.facebook.WebDriverAgentRunner.xctrunner.WebDriverAgentRunner` に変える → **iloader の Import IPA で入った**
- 起動：`ios tunnel start --userspace`（管理者なし）→ `ios runwda --bundleid=<id> --testrunnerbundleid=<id> --xctestconfig=WebDriverAgentRunner.xctest` → `ios forward 8100 8100`。go-ios のログに「ServerURLHere」は出ないが、5秒で `/status` が返る（WDA 16.15.3、iOS 27.2.1）
- 操作：`POST /session` → `/wda/apps/activate`（LiveContainer）→ `/wda/dragfromtoforduration` を下・上・右・左。画面は 390×844（ポイント）
- 結果：DevKit のログに `DEVKIT swipe=down/up/right/left` の4行、画面にも4行。**PC から操作 → 実機のログと画面で確認、の一周がつながった**
- 注意：syslog はトンネルより先に始める（トンネルがあると RSD 経由になり失敗した）。WDA はアプリの枠を1つ使う（無料は3個まで：LiveContainer、WDA）
- PC 側のスクリプト：`Downloads\devkit-setup\9-wda.*`

## 2026-11-24（SideStore の更新が通るようになった）

- 症状：「Refresh All」で「You do not appear to be connected to VPN」。LocalDevVPN（1.3.0）のログでは接続済み（状態3）でエラーなし
- SideStore の Settings → Health Check で原因が分かった：VPN トンネル○、**Ping ×**。SideStore はトンネルの住所（Tunnel Iface IP 10.7.1.1）の次の番号 **10.7.1.2** を iPhone 本体とみなして話しかけていたが、LocalDevVPN の Device IP は **10.7.0.1** だった
- 対処：LocalDevVPN の Tunnel IP を `10.7.1.1` → **`10.7.0.0`** に変更（Device IP は `10.7.0.1` のまま）。SideStore が期待する住所が 10.7.0.1 になり一致
- 結果：Health Check がすべて○（System Healthy。Ping Reachable、Pairing file Verified、DDI Mounted）。LiveContainer の更新が VPN のエラーなしで始まった
- 元に戻すとき：Tunnel IP を 10.7.1.1 に戻す

## 2026-11-24（PC から実機のログと画面を取れた）

- `ios syslog`（go-ios v1.3.2）はトンネルなしで動く。40秒で約4MB。DevKit のスワイプ11回がすべて `LiveContainer(DevKit)[pid] <Notice>: DEVKIT swipe=… commit=3dd056a` として取れた（`privacy: .public` が効いている）
- `ios screenshot` はトンネルなしだと失敗（iOS 27.2.1 はトンネルが必要）
- `ios tunnel start --userspace` は**管理者なし**で立つ（wintun 不要の userspace）。立てたまま `ios screenshot --output=…` で実機の画面（1170×2532、約100KB）が取れた。開発者イメージ（DDI）は `image list` で none のままでも撮れた
- PC 側のスクリプト：`Downloads\devkit-setup\6-see.*`（syslog）、`7-tunnel.*`（トンネル＋画面）。終わったらトンネルは止める

## 2026-11-24（実機で初めて動いた）

- iloader v2.4.2 で LiveContainer + SideStore（Stable）を署名用 ID で入れた。iOS 27.2.1 でペアリングのエラーは出なかった
- LiveContainer の「SideStore から証明書をインポート」は、内蔵の SideStore にサインインする前だと「証明書が見つかりません」になる → 先に SideStore にサインインする
- SideStore のサインインで「Apple didn't give valid end provisioning data」→ Anisette サーバーを「SideStore (.zip)」から「SideStore (New)」（new.ani.sidestore.io）に替えたら通った
- 証明書のインポートは成功（表示が「証明書を削除」「SideStore から証明書をリフレッシュ」に変わる）
- SideStore の「Refresh All」は「You do not appear to be connected to VPN」で失敗。LocalDevVPN は Connected の表示。期限（7日、12月1日ごろ）までに要対応
- DevKit.ipa（build: 3dd056a）を Safari でダウンロードし、LiveContainer に入れて起動できた。上下左右のスワイプがすべて記録された（実機のスクリーンショットで確認）

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
