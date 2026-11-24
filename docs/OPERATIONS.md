# 運用の手順書（Claude 向け）

この repo で iPhone アプリを作るときに、Claude が道具を使いこなすための手順書です。
新しいセッションでは、まずこれを全部読んでから作業します。経過・分かったことの記録は [LOG.md](LOG.md)（新しいものが上）。

- 「未検証」と書いてある所は、まだ実機で確かめていない。使う前に記録を取って確かめる
- 手順が変わったら、この文書も同じコミットで直す

---

## 1. 全体の形

```
[GitHub]  iwataka793/iphone-dev-kit（公開）
   ├─ Actions ios.yml   … main への push で DevKit をビルド → シミュレーターで UI テスト → Releases に build-N（最新版）
   ├─ Actions wda.yml   … WebDriverAgent をビルド → Releases に wda-N（最新版にはしない）
   └─ Actions core-tests.yml … Core/ のテスト（Linux）

[Windows PC]（カオルの PC。Claude はデスクトップアプリ経由でつなぐ）
   ├─ %USERPROFILE%\devkit\go-ios\ios.exe   … iPhone とやりとりする道具（go-ios v1.3.2）
   └─ Downloads\devkit-setup\               … 台本（go.bat / go.ps1 / plan.json）、ID などの設定 config.ps1（公開 repo に入れない）、記録（runs\）

[iPhone 16e / iOS 27.2.1]（USB で PC につなぐ）
   ├─ LiveContainer … 自作アプリ（DevKit など）はこの中で動かす。SideStore も中に同梱
   ├─ SideStore     … 無料 Apple ID で署名・7 日ごとの更新（LiveContainer と WebDriverAgent）
   ├─ LocalDevVPN   … SideStore が iPhone 自身にインストールするための VPN
   └─ WebDriverAgent（WebDriverAgentRunner-Runner）… PC から画面を操作するための部品
```

Claude の手元のシェル（Bash）はクラウドの Linux で、PC ではない。PC では次の 2 つしかできない。

- ファイルを置く・読む（デスクトップアプリの橋渡し）
- 画面を見てクリックする（computer use）

---

## 2. 開発の 1 周

1. **書く**：`App/`（画面）と `Core/`（iPhone に依存しないロジック）を直す。動きを確かめたい所には、iPhone のログに出る印を入れる（例：`logger.notice("DEVKIT ...")`、`privacy: .public` を付ける）
2. **ビルド**：main に push → ios.yml が動く
   - 結果は REST で見る（このセッションの gh は GraphQL を使うコマンドが通らないことがある）：
     `gh api 'repos/iwataka793/iphone-dev-kit/actions/runs?per_page=5' --jq '.workflow_runs[] | "\(.id) \(.name) \(.status) \(.conclusion) \(.head_sha[0:7])"'`
     失敗したら `gh api repos/iwataka793/iphone-dev-kit/actions/runs/<id>/jobs` で段階を見る
   - ファイルは `curl -sL https://github.com/iwataka793/iphone-dev-kit/releases/download/<タグ>/<ファイル>` で取れる。最新のタグは `gh api repos/iwataka793/iphone-dev-kit/releases/latest --jq .tag_name`。そのタグのコミットは `gh api repos/iwataka793/iphone-dev-kit/git/ref/tags/build-N --jq .object.sha`（リリースの説明文にも書いてある）
   - Releases の `build-N` に DevKit.ipa・simulator-check.zip（シミュレーターの画面）・simulator-test.log が置かれる
   - このセッションからは Releases を作れない（403）。作るのは Actions に任せる
3. **シミュレーターで確かめる**：`curl -sL https://github.com/iwataka793/iphone-dev-kit/releases/download/build-N/simulator-check.zip` などで取って見る
4. **iPhone に入れる**（人の手なし。2026-11-24 確認済み）：3-5 の手順で PC から LiveContainer に直接置く
   - 予備：カオルに Safari で `https://github.com/iwataka793/iphone-dev-kit/releases/latest/download/DevKit.ipa` を開いて LiveContainer で入れてもらう
   - 入ったかどうかは DEVKIT のログの `commit=` か、画面下の `build:` がコミット番号と同じかで確かめる
   - 比べる相手は「入れた build-N のタグのコミット」。ios.yml は App/・Core/・ios.yml・scripts/*.sh が変わったときだけ動くので、ほかのファイル（docs など）だけのコミットでは新しい build-N はできない
   - Actions が実行中なら、終わるまで待ってから比べる
5. **実機で動かして見る**：PC の go.bat（3 章）。ログ・画面・操作が一度にできる
6. **記録**：分かったことは LOG.md の一番上に書く

---

## 3. PC の使い方

**PC そのものを調べる・動かすとき**（容量・不具合・アプリの操作など）は、道具に `pc_status` があれば [PC.md](PC.md) の道具を使う。画面を撮って押す computer use より速い。

**iPhone のときは、まず MCP（[MCP.md](MCP.md)）が使えるか確かめる**：道具に `iphone_status` があれば、iPhone の確認・操作・DevKit の入れ替えはそれで行う（速い）。スイッチがオフなら、カオルに「見るだけ」か「操作も可」にしてもらう。MCP が無いときだけ、以下の go.bat のやり方を使う。

### 3-1. つながっているか

- `get_device_info` → `connectedFolders` に `%USERPROFILE%\Downloads`（ユーザーフォルダの Downloads） があれば OK
- つながらないとき：PC が寝ている／Claude デスクトップアプリが閉じている。カオルに頼む
- iPhone は USB でつなぎ、ロックを外してもらう（ロック中はつながらないことがある）

### 3-2. 台本を置く（初回・更新時）

最初に、PC の go.ps1 / go.bat が repo と同じかを確かめる（`device_stage_files` で取って `cmp`）。違えば置き直す。

repo の `scripts/pc/` を PC の `Downloads\devkit-setup\` に置く。

1. `SendUserFile` で go.ps1 / go.bat を送る（file_uuid が返る）
2. `device_commit_files` で `<ユーザーフォルダ>\Downloads\devkit-setup\go.ps1`（get_device_info の connectedFolders の下） などに書く（既にあるなら `force: true`）
3. 初回だけ：`config.example.ps1` を元に `devkit-setup\config.ps1` を作って置く（Claude が作れる。LiveContainer の bundle id は `ios apps --list` の LiveContainer の行だけを絞って調べる）
   - 値は PC の中だけに置く。公開 repo に書かない

.ps1 は「UTF-8（BOM 付き）＋ CRLF」でないと、Windows PowerShell 5.1 で日本語が壊れる。repo では `.gitattributes` で CRLF にし、BOM はファイルに入れてある。新しく作るときも同じにする。

### 3-3. 実行する（go.bat）

1. `plan.json`（3-4）を作り、`SendUserFile` → `device_commit_files` で `devkit-setup\plan.json` に置く
2. computer use でエクスプローラーを開き、`devkit-setup` の `go.bat` をダブルクリックする
   - 開き方：`computer_open_application("エクスプローラー")`（またはタスクバーのフォルダのアイコン）→ ホームが開くので、左の一覧の「devkit-setup」を押す
   - 許可していないアプリ（通知の窓など）が一番前にあるとクリックが断られる。エクスプローラーの窓を一度押して前に出してからやり直す
   - 初回は `computer_resolve_access(["File Explorer"])` → `computer_request_access`
   - エクスプローラーは click 権限だけ（打鍵・右クリック不可）。ダブルクリックで .bat を動かすのは可
   - 一覧の並びはファイルを置くたびに変わる（新しい順）。**ファイルを置いたら 2 秒待ち、押す直前に `computer_zoom` で名前を確かめてから、別の呼び出しで押す**（zoom と押すのを同じ batch に入れると確かめた意味がない。2026-11-24 に config.ps1 を開いてしまった）
   - 待ちは 1 回 50 秒まで。長いときは分けて待つ
3. 結果を読む：`device_stage_files` で `devkit-setup\last-result.txt` を取る。画面は `runs\<日時>-<name>\*.png`
   - かかる時間：WDA ありで 40〜90 秒（トンネル 15 秒＋WDA の起動 5 秒＋操作）。最後の行が `== 完了` なら終わっている。1 行目の時刻が今回のものかも確かめる（前の結果を読まない）
   - iPhone のログの時刻は PC の記録より 2 秒ほど早く出ることがある（時計のずれ）
4. 終わったら `computer_release_lock`

### 3-4. plan.json の書き方

```json
{
  "name": "swipe",
  "match": "DEVKIT",
  "wda": true,
  "target": "livecontainer",
  "actions": [
    {"do": "activate"},
    {"do": "screenshot", "name": "before"},
    {"do": "drag", "from": [195, 250], "to": [195, 550], "duration": 0.2},
    {"do": "tap", "at": [195, 400]},
    {"do": "wait", "sec": 2},
    {"do": "source"},
    {"do": "screenshot", "name": "after"}
  ]
}
```

- よく使う形（DevKit を前に出して上下にスワイプ）：`{"name":"check","match":"DEVKIT","wda":true,"target":"livecontainer","actions":[{"do":"activate"},{"do":"wait","sec":2},{"do":"drag","from":[195,250],"to":[195,550]},{"do":"wait","sec":2},{"do":"drag","from":[195,550],"to":[195,250]},{"do":"screenshot","name":"after"}]}`
  - `activate` で LiveContainer の一覧（マイアプリ）が出たときは、DevKit は動いていない。`{"do":"tapText","text":"実行"}` と `wait` 8 秒を足す。どちらが出るかは、その前の状態しだい
- `match`：iPhone のログから抜き出す文字（正規表現）。結果に最後の 60 行が出る。全部は `runs\...\syslog-match.txt`
- `wda: false`：操作せず、`seconds` 秒ログを取って前後の画面を撮るだけ
- 座標は WebDriverAgent の単位（iPhone 16e は 390×844）。結果の「画面の大きさ」に出る
- `source`：画面の部品一覧（XML）。ボタンの位置や文字を調べるのに使う
- `target`：`livecontainer` は config.ps1 の LiveContainer。ほかのアプリは bundle id を書く
- ほかの操作：`terminate`（アプリを終わらせる）、`tapText`（`"text":"実行"` のように文字で部品を探して押す。`index` で何番目か）、`tree` / `pull` / `push` / `pushdir`（開発者署名のアプリの中のファイル。書き方は go.ps1 の先頭）
- WDA の失敗は理由（返ってきた本文）も結果に出る

### 3-5. DevKit を LiveContainer に直接入れる（確認済み）

LiveContainer は起動前に `LCAppInfo.plist` の `LCPatchRevision` を見て、古ければ実行ファイルにパッチを当てて自分の証明書で署名し直す。これを使う。

1. 今の `LCAppInfo.plist` を取る（plan）：
   `{"name":"lc-pull","wda":false,"actions":[{"do":"pull","app":"livecontainer","path":"/Documents/Applications/io.github.iwataka793.devkit.app"}]}`
   → `device_stage_files` で `runs\<日時>-lc-pull\pulled\io.github.iwataka793.devkit.app\LCAppInfo.plist` を取る（これが控えにもなる）
2. クラウドで作る：Releases `build-N` の DevKit.ipa を取り、
   `python3 -I tools/lc-install/prepare.py DevKit.ipa LCAppInfo.plist out/`
   （.app の中身 ＋ `LCPatchRevision` を消した LCAppInfo.plist ができる）
3. `out/` の各ファイルを `SendUserFile` → `device_commit_files` で `devkit-setup\files\` に置く（`force: true`）
4. plan で入れて起動：
   ```json
   {"name":"lc-install","match":"DEVKIT","wda":true,"target":"livecontainer","actions":[
     {"do":"terminate"},
     {"do":"pushdir","app":"livecontainer","local":"files","remote":"/Documents/Applications/io.github.iwataka793.devkit.app"},
     {"do":"activate"},{"do":"wait","sec":2},
     {"do":"tapText","text":"実行"},{"do":"wait","sec":8},
     {"do":"screenshot","name":"after-run"}
   ]}
   ```
   - 先に `terminate` で LiveContainer を止める（動いている実行ファイルを上書きしない）
   - LiveContainer の画面は日本語（ボタンは「実行」）。`tapText` で押せる
   - LiveContainer を起動し直したとき、前に動かしていたアプリがそのまま立ち上がることがある（`tapText` が 0 個でも、DEVKIT のログが出ていれば起動している）
5. 確かめる：DEVKIT のログの `commit=` が新しい番号か、`after-run.png` の画面下 `build:`
- 注意：`pushdir` は上書きだけで、新しい版で無くなったファイルは消えずに残る
- うまくいかないときは、1 の控え（pulled の中身）を `files` にして `pushdir` すれば元に戻せる

### 3-6. 守ること（PC）

- 結果（last-result.txt・iPhone のログ）には、チーム ID 付きの bundle id や端末の名前が入る。LOG.md などに写すときは `<チームID>` のように伏せる
- iPhone の出力は**必要な行だけに絞ってから読む**。`ios apps --list` などはアプリ一覧が全部出る（個人情報）
- SideStore の記録（ConsoleLogs）には Apple ID のメールアドレスやログイン用の一時データが入る。チャットに出さない・repo に入れない
- PC のファイルを消すときは、先にカオルに確認する。消すときは完全削除ではなく「ごみ箱へ」（`Microsoft.VisualBasic.FileIO.FileSystem.DeleteFile(..., 'SendToRecycleBin')`）
- iloader のパスワード（資格情報マネージャー "iloader"）には触らない

---

## 4. go-ios のコツ

| やること | コマンド | 注意 |
| --- | --- | --- |
| ログ | `ios syslog` | **トンネルより先に始める**（後だと失敗する）。トンネル無しで動く |
| トンネル | `ios tunnel start --userspace` | 管理者権限いらない。15 秒ほど待つ |
| 画面 | `ios screenshot --output=x.png` | トンネルが要る |
| アプリ一覧 | `ios apps --list` | 全部出るので必ず絞る |
| WDA 起動 | `ios runwda --bundleid=B --testrunnerbundleid=B --xctestconfig=WebDriverAgentRunner` | テスト本体名は `.xctest` なし（7 章） |
| 転送 | `ios forward 8100 8100` | PC の 8100 番 → iPhone の WDA |
| 落ちた記録 | `ios crash ls` | 名前だけ絞って見る |
| アプリの中のファイル | `ios fsync --app=<bundle id> tree --path=/` / `pull --srcPath= --dstPath=` | 開発者署名のアプリ（LiveContainer など）の中が読める。Windows では tree の字下げが出ない（全部平ら） |
| 受け渡し場所 | `ios fsync tree --path=/PublicStaging` | インストール後は空になる |

- 詳しいやりとりを全部見るとき：`-t`（trace）。エラーの中身（NSError）は go-ios が説明文しか出さないことがある
- `runwda` は `--log-output=<file>` でテスト側のログが取れる
- LiveContainer の中の場所：`/Documents/Applications/<bundle id>.app`（入れたアプリ）、`/Documents/Data/Application/<UUID>/`（アプリのデータ）、`/Documents/SideStore/Documents/`（同梱 SideStore の書類。ConsoleLogs・ResignedApps）

---

## 5. WebDriverAgent（PC から実機を操作する部品）

- ビルド：wda.yml。Releases `wda-N` の **WebDriverAgentRunner-ss.ipa** を使う（`WebDriverAgentRunner.ipa` は旧形式）
- 入れ方：iPhone の Safari で ss.ipa を落とす → LocalDevVPN をオン → SideStore の My Apps「＋」→ 選ぶ
  - LiveContainer の中ではなく iPhone 本体に入る（LiveContainer の中では動かない）
- 更新：SideStore の 7 日更新（Refresh）に任せる。PC 側の自動更新はやめた
- 使う API（go.ps1 の中）：`/status`、`POST /session`、`/session/{id}/wda/apps/activate`、`/session/{id}/window/size`、`/session/{id}/wda/dragfromtoforduration`、`/session/{id}/wda/tap`、`/session/{id}/source`

---

## 6. SideStore / LiveContainer の注意

- SideStore は LiveContainer 同梱版（0.6.4-20260714、LiveContainer/SideStore の ddf08f58）
- **本番ビルドでも DEBUG が有効**で、署名の準備のときに PlugIns の中の「.xctest」を含む名前のものを黙って消す（2026-11-24 の LOG）。テスト部品を含むアプリは名前から `.xctest` を外す（`tools/wda-sidestore/make_ss_ipa.py`）
- LocalDevVPN の Tunnel IP は `10.7.0.0`（SideStore は Tunnel IP + 1 に話しかける）。SideStore の Health Check がすべて緑なら OK
- Anisette サーバーは「SideStore (New)」
- 無料 Apple ID の制限：有効なアプリ 3 つまで、App ID は 7 日で 10 個まで（My Apps の下に残り数）、証明書は 1 年
- 同じ ID のアプリを別の道具（iloader など）で入れると上書きし合い、SideStore の一覧から消える
- 隠しメニュー（DIAGNOSTICS）：Settings で 3 本指で下から上へ素早く 3 回スワイプ。詳しい記録は「Enable Verbose Ops Logging」
- 記録の場所：LiveContainer の中 `/Documents/SideStore/Documents/ConsoleLogs/console-*.log`（ログイン用データを含む）
- 「Export Resigned Apps」をオンにすると、送る直前の署名済みアプリが `ResignedApps/` に残る（調べるとき用）

---

## 7. 調べ方の決まり

- **とにかくログ**：推測やソースを読むより先に、実物（送られたファイル・実機のログ・書き出したもの）を見る。2026-11-24 は、ソースを読んで「入っているはず」と決めて遠回りした
- 原因が分からないときは、わざと同じ失敗を起こして形を比べる（例：無い名前を指定して同じエラーになるか）
- 実際に動いている版のソースを見る（SideStore は LiveContainer 版、AltSign + ldid。upstream とは別物）

---

## 8. 未検証・これから

- 新しいセッションでの試験は 2026-11-24 に済み（LOG 参照）。手順が大きく変わったらもう一度やる

### 次に PC がつながったらやること（2026-11-25 カオルに頼まれた）

PC の道具（[PC.md](PC.md)）を入れて確かめる：
1. （済 2026-11-25。2〜4 も済。5 は .bat の日本語と DPI が残り）Releases の最新の `pc-N`（いまは pc-4）の `pc-dev-kit.mcpb` と `pc-switch.zip` を PC のダウンロードに置く
2. カオルに `pc-switch.zip` の中の `pc-setup.bat` を 1 回実行してもらい、`pc-dev-kit.mcpb` を拡張機能に入れてもらう
3. カオルに PC-VIEW を押してもらい、`pc_status` → `pc_disk`（引数なし）→ `pc_problems` で、カオルの PC で実際に動くか確かめる
4. PC-ALLOW の流れ（UAC の「はい」→ 10 分の許可 → 切り替えで取り消し）を 1 回試す
5. PC.md の最後の「まだ確かめていないこと」（ストア版 Claude からの 127.0.0.1 と token.txt・UAC と %ProgramData% の書き込みの制限・.bat の日本語・画面の拡大率）を 1 つずつ確かめて、PC.md と LOG.md を直す

iPhone の道具（[MCP.md](MCP.md)）の残り：
6. （済 0.1.7）文字で押す（iphone_tap_text）：同じ文字が複数あるとき、見えていて押せる方を選ぶ（マップの「出発」で外れた）
7. （0.1.7 で iOS の状態で見るようにした。かかっているときの値はカオルにかけてもらって確かめる）ロックの判定：画面（真っ黒）ではなく iOS の状態で見て、外れていなければ「カオルに外してもらう」と返す
8. （済 0.1.7）ログを PC のファイルにも残す（3 万行からあふれて、前のことを確かめられなかった）
9. （設定は済 2026-11-25。ショートカットはまだ）設定アプリの操作（名前で開く → 検索欄に打つ → スクロールして探す。見るだけで何も変えない）と、ショートカットを 1 つ作って URL で実行するところまで確かめる
10. （LiveContainer に RefreshAllIntent があった。ショートカットで動くかはまだ）SideStore に、ショートカットから入れ直し（Refresh All）を呼ぶ機能があるか見る（次の入れ直しの期限は 12 月 1 日ごろ）

安全の見直し（2026-11-25）：

11. （済 2026-11-25）PC に残っていた SideStore のログの写しは消した

遠隔で PC の電源を入れる（Wake-on-LAN）の下調べ（2026-11-25 カオルの頼み）：

12. PC の道具で、見るだけで調べる（何も変えない）：スリープの方式（`powercfg /a`。モダン スタンバイか S3 か）、LAN アダプター（USB-C の変換）の種類と起こす機能（`Get-NetAdapter`・`Get-NetAdapterPowerManagement`・`Get-NetAdapterAdvancedProperty` のマジックパケット）、起こせる機器（`powercfg /devicequery wake_armed`）、最近の起きた理由（イベントログの Power-Troubleshooter）。見込み：USB の LAN アダプターは電源を切った状態からは起こせない。スリープからなら見込みあり。結果と、変えるなら要る設定をまとめてカオルに出し、変えるのは決めてから

iPhone の中身を調べる道具（PC の道具の iPhone 版。2026-11-25 カオルの頼み）：

13. （0.1.7 で大部分済。残り：仮の位置（開発者のディスクイメージ）、Safari の中（Web インスペクタ）、自分のアプリの受け口）今の iPhone の道具は画面の操作が中心。PC の道具のように、iPhone の中身を見る道具にする。先に、各窓口が iOS 27 で答えるかを見るだけで 1 つずつ試す（go-ios v1.3.2 に部品あり）：端末の情報と空き容量（lockdown・disk_usage）、アプリごとの大きさ（installationproxy）、クラッシュの記録（crashreport。ほかのアプリの分も）、プログラムごとの CPU・メモリ（instruments の sysmontap）、電池の状態（diagnostics の IORegistry）、構成プロファイル（mcinstall）。答えたものだけを道具にする（5〜7 個に抑える。見るだけで使える）。アプリの一覧・クラッシュの記録は個人の情報なので repo に写さない
    さらに、画面を毎回操作するのではなく、中から動かす分も同じ形で試す（カオルの頼み：PC の道具のように効率よく内部で動く）：アプリの起動・終了（instruments の processcontrol。WDA なし）、自分のアプリ・LiveContainer のファイルの読み書き（house_arrest。テストのデータ・設定）、写真などのフォルダ（afc）、クリップボードへの書き込み（pasteboard。打つ代わりに貼る）、URL・ショートカットの直接実行、開発用の仮の位置（simlocation）、言語・時刻の形式・タイムゾーンなどの一部の設定（lockdown。変える前にカオルに確かめる）、WDA なしの画面（instruments の screenshot）、Safari の中のページ（webinspector。設定でオンが要る）。ほかのアプリの細かい操作だけは画面（WDA。部品の名前で押す）が残る。自分のアプリには PC から呼べる受け口（のぞき窓）を入れる

