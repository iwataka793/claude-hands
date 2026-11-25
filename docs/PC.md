# PC の道具（tools/pc-mcp）

この md は道具の動かし方だけを書く。GitHub の Windows（windows-latest）で通しの試験をしたもの。カオルの PC でまだ確かめていないものは「（未確認）」と書く。

Claude が Windows の PC を、画面を撮って押す（computer use）のではなく、PowerShell・ファイル・Windows の UI オートメーションで直接見て・動かすための道具。1 回の呼び出しは 0.1 秒前後（最初だけ PowerShell の起動で数秒）。

## 作り

| 役 | ファイル | 何をするか |
| --- | --- | --- |
| 拡張機能 | `pc-dev-kit.mcpb`（中に `pc-mcp.exe`） | Claude デスクトップの中で動く。道具を並べて、中身は常駐役に頼むだけ |
| 常駐役 | `%USERPROFILE%\devkit\pc-agent\pc-agent.exe` | カオルのスイッチ（.bat）で起動・停止する。PowerShell を開きっぱなしにして実際の作業をする。PC の中（127.0.0.1:60210）だけで受け付け、合言葉（token.txt）がないと答えない |

常駐役を分けたわけ：ストア版の Claude から動くプログラムは、AppData への書き込みが別の場所に振り分けられる（iPhone の道具で確かめた）。常駐役はカオルがエクスプローラーから起動するので、振り分けを受けない。

## 入れ方（カオル・最初と、新しい版にするとき）

1. Releases の最新の `pc-N` から `pc-dev-kit.mcpb` と `pc-switch.zip` を落とす
2. `pc-switch.zip` を開いて、中の `pc-setup.bat` を実行する（常駐役を `devkit\pc-agent` に、スイッチの窓を `devkit\switch-app` に置き、デスクトップに窓のショートカット「Claude 操作スイッチ」を作る（.bat は zip の中の予備）。動いている常駐役は止めてから入れ替える）
3. Claude デスクトップの「設定 → デスクトップアプリ → 拡張機能」に `pc-dev-kit.mcpb` をドラッグする
4. 道具ごとの「許可」の確認は、カオルが「常に許可」を選べば次から出ない

## スイッチと許可（カオルだけが切り替える）

| .bat | 状態 | Claude ができること |
| --- | --- | --- |
| PC-OFF | オフ | 何もできない（常駐役は数秒で終わる） |
| PC-VIEW | 見るだけ | 容量・不具合の記録・プロセス・アプリ・ファイルを読む・画面・窓・部品の一覧・読むだけの PowerShell |
| PC-OPERATE | 操作も可 | 上に加えて、PowerShell（何でも。ただし下の「許可が要るもの」は除く）・開く・部品を押す・打つ・ごみ箱 |
| PC-ALLOW | 許可（10 分） | 下の「許可が要るもの」も使える。管理者の確認（UAC の「はい」）を通る。スイッチを切り替えると取り消し |

- 使わない時間が 30 分続くとオフに戻り、常駐役が終わる
- 置き場所：スイッチ `%USERPROFILE%\devkit\pc-agent\mode.txt`、許可 `%ProgramData%\pc-agent\allow.txt`
- 許可のファイルは、管理者しか書けないフォルダにある。PC-ALLOW.bat が UAC を通ってから書く。Claude の PowerShell は普通の権限なので書けない（試験で確かめた）
- Claude 側の決まり：.bat・窓のボタンを押さない・mode.txt / allow.txt を書かない。オフなら「オンにしてください」、許可が要るなら「PC-ALLOW.bat（窓の「10 分 許可」）を押してください」と頼む

## スイッチの窓と「許可しますか？」（2026-11-25 から）

- `%USERPROFILE%\devkit\switch-app\devkit-switch.vbs` を開くと、iPhone と PC のスイッチを 1 つの窓で切り替えられる（中身は .bat と同じ：mode.txt を書く。PC の許可は UAC を通って allow.txt を書く）。右下の通知領域に常駐し、窓を閉じても残る。終わるのは通知領域の右クリック →「終わる」。デスクトップのショートカット「Claude 操作スイッチ」（アイコンは devkit-switch.ico）から開ける。もう動いているときは、2 つ目は起動せず、動いている方の窓を前に出す（名前つきの合図 Local\DevKitSwitchShow）
- 道具がスイッチ不足で止まるとき、道具は `%USERPROFILE%\devkit\requests\<種類>.txt`（iphone-view・iphone-operate・pc-view・pc-operate・pc-allow。中身は理由 1 行）を置き、窓を `/tray` で起こす。窓が「○○にしますか？」を出し、カオルが「許可する」か「断る」を押す
- 道具は返事を待つ：60 秒（PC の許可は 90 秒）。許可されたらそのまま続ける。断られた・時間切れなら、今までどおり「スイッチが足りません」を返す
- 窓は 55 秒（許可は 80 秒）で自動的に断る。2 分より古い頼みは捨てる。既定のボタンは「断る」（Enter・Esc で断る）
- Claude 側の決まりは変わらない：mode.txt・allow.txt を書かない、窓のボタンを押さない。requests に置くのは道具だけ（Claude が手で置いてよいのは、カオルに見た目を見せる試しのときだけ）
- `DEVKIT_NO_ASK` があるとき（CI の試験）や USERPROFILE が無いときは頼まない

### 許可が要るもの（道具が止める）

| 種類 | 例 |
| --- | --- |
| パスワード・鍵・資格情報 | ブラウザのパスワード・クッキー・カード（Login Data・Web Data・Cookies など）、Windows の資格情報・証明書、.ssh・.aws・.git-credentials・.npmrc・gh の hosts.yml、iPhone のトンネルの鍵（devkit\go-ios）・iloader のサインイン、.kdbx・.pfx・.pem・.key・.env、パスワード管理アプリ |
| お金・本人確認 | 名前に bank・銀行・証券・wallet・給与・源泉・確定申告・e-Tax・マイナンバー・パスポート・免許証・password・secret などを含む場所。窓の題名に銀行・証券・PayPay・カード・サインイン・ログイン・アカウントなどを含む窓 |
| セキュリティ・Windows の設定 | cmdkey・certutil・netsh・Defender・ファイアウォール・BitLocker・ユーザー・レジストリ・タスクの予約・サービス・実行の決まり・アクセス権・管理者で開く（runas） |
| 戻せない・外に出る | 完全に消す（Remove-Item・.Delete()。ごみ箱は pc_trash でよい）・ごみ箱を空にする・ドライブの初期化・外への通信（Invoke-WebRequest など）・別の PowerShell や cmd・中身が後で決まる実行（Invoke-Expression・& $x） |
| Windows 本体を変える | C:\Windows・Program Files・ProgramData・ドライブの直下・スタートアップ |

- スイッチ・許可の置き場所は、許可があっても触らせない
- 調べ方：PowerShell 自身に台本を読ませて（実行はしない）、呼ぶ命令・メソッド・書き出しを見る。別名（del・rm・iwr など）は元の名前に直して判断する
- 限り：台本の書き方をわざと工夫すれば、文字で探す守りは抜けられる。止めているのは「うっかり」。抜けられないのは、UAC を通す許可のファイルだけ。だから Claude 側の決まり（CLAUDE.md）と記録（audit.log）を組み合わせる

## 道具

| 道具 | スイッチ | 中身 |
| --- | --- | --- |
| pc_status | いつでも | 常駐役・スイッチ・許可・最近の記録 8 行 |
| pc_disk | 見るだけ〜 | ドライブの空き。`path` の中を大きい順（`files` で 50 MB 以上のファイルも）。膨らみやすい場所の大きさ（path を省いたとき） |
| pc_problems | 見るだけ〜 | イベントログのエラー（`warnings` で警告も）、落ちた・固まったアプリの回数、エラー報告（WER）、クラッシュのダンプ、`reliability` で信頼性モニター。`app` で絞る |
| pc_processes | 見るだけ〜 | メモリの重い順（同じ名前はまとめる）か、`sort: cpu` で CPU（1 秒測る） |
| pc_apps | 見るだけ〜 | 入っているアプリ（`sort: size/date`、`store` でストアのアプリも）か、`kind: startup` で起動時に動くもの |
| pc_files | 見るだけ〜 | list（一覧）・read（中身。`head`・`tail`・`pattern` で行を絞る。UTF-8・Shift_JIS・UTF-16 を見分ける）・find（名前の形で探す。`sort: new/big`）・info |
| pc_screenshot | 見るだけ〜 | 画面全体か窓（`window`）。返事に「画像の座標 → 画面の座標」の換算が付く |
| pc_windows | 見るだけ〜 | 開いている窓（題名・プログラム・pid・位置。いま前の窓に印） |
| pc_ui | 見るだけ〜 | 窓の中の部品（種類・名前・id・値・位置）。文書の欄は中身の先頭も出る。`filter` で絞る |
| pc_powershell | 見るだけ〜 | 見るだけでは Get- / Test- / Measure- / Select- などの読む命令だけ。操作も可で何でも（許可が要るものを除く） |
| pc_open | 操作も可 | アプリ・ファイル・フォルダ・URL を開く |
| pc_ui_do | 操作も可 | 部品を名前か id で探して click・set_value・toggle・expand・select など。位置で押す click_xy（`from_image` で画像の座標のまま） |
| pc_keys | 操作も可 | 打つ（`text`、日本語は `paste: true` が確実）・特別なキー（`keys`：^s・%{F4}・{TAB} など）・`enter`。`window` を前に出せなければ打たない |
| pc_trash | 操作も可 | ごみ箱へ送る（元に戻せる）。消す前にカオルに確かめる決まり |

## よく使う流れ

| したいこと | やり方 |
| --- | --- |
| 容量が減った原因 | `pc_disk`（引数なし）でドライブと膨らみやすい場所 → 大きい所を `pc_disk path=... files=true` で掘る |
| アプリの不具合 | `pc_problems app=アプリ名 hours=72` → 落ちた回数・エラーの中身・WER の控えの場所 → そのアプリのログを `pc_files find`（`*.log`・`sort: new`）→ `pc_files read tail=200` |
| 重い | `pc_processes`（メモリ）と `pc_processes sort=cpu` |
| アプリを操作 | `pc_open` → `pc_windows` → `pc_ui window=... filter=...` で名前を見る → `pc_ui_do` → 結果を `pc_ui` か `pc_screenshot window=...` で確かめる |
| 画像を見て押す | `pc_screenshot` → 画像の座標を読む → `pc_ui_do action=click_xy from_image=true x=.. y=..` |

## 気をつけること（試験で分かったこと）

- 窓を前に出すのに Alt キーを使ってはいけない。メニューが開いて、そのあと打った文字がメニューの操作になる（メモ帳が「閉じる」になった）。今は空のキーと UI オートメーションで前に出し、出せなければ打たない
- メモ帳などの文書の欄は「値」ではなく「文書」の部品。`set_value` は、全部選んで貼る形で入れる
- 最初の画面系の呼び出し（pc_windows など）は、部品の読み込みで 5 秒ほどかかる。2 回目からは 0.1 秒前後
- 道具の返事には、アプリの一覧・ファイル名・窓の題名など個人の情報が出る。repo や記録に写すときは伏せる

## うまくいかないとき

| 症状 | まずやること |
| --- | --- |
| 「スイッチがオフ」 | 道具が「許可しますか？」の窓を出して 60 秒待つ。それでもだめなら、カオルに PC-VIEW か PC-OPERATE を押してもらう |
| 「カオルの許可が要ります」 | 道具が理由つきで「許可しますか？」の窓を出して 90 秒待つ。本当に要る操作のときだけ呼ぶ |
| 「常駐役が断りました」 | 常駐役を起動し直す（カオルに PC-OFF → PC-VIEW） |
| 道具がおかしい | `pc_status` の最近の記録。全部は `%USERPROFILE%\devkit\pc-agent\audit.log`・`agent.log` |

## 中の作り

- Go（`tools/pc-mcp`）。同じソースから、拡張機能（コンソール）と常駐役（窓なし）の 2 つの exe を作る
- PowerShell（Windows PowerShell 5.1）を開きっぱなしにして、台本を base64 の 1 行で送り、終わりの印まで読む。時間切れなら起動し直す
- 容量の計算とファイルは Go（速い）。つなぎ目（ジャンクション）はたどらない。OneDrive のクラウドのみのファイルは分けて数える
- 台本は `tools/pc-mcp/scripts/*.ps1`（exe に埋め込む）
- 試験：`tools/pc-mcp/test/scenario.py`（GitHub の Windows で毎回。手元では Linux＋pwsh でも動く）
- ビルド：`.github/workflows/pc-mcp.yml`（Linux でビルド → Windows で通しの試験 → Releases `pc-N`）

## カオルの PC で確かめたこと（2026-11-25）

- ストア版の Claude の中の拡張機能から、常駐役（127.0.0.1）と token.txt に届く：OK
- PC-ALLOW.bat：UAC の「はい」→ 許可 10 分 → 鍵の場所（selfIdentity.plist）をごみ箱へ送れた：OK
- 容量・不具合の記録・PowerShell・ごみ箱（73 個）：OK。1 回 1〜2 秒
- .bat の日本語の表示（CP932）：OK（カオルに見てもらった）
- まだ：画面の拡大率（DPI）が 100% でないときの座標、画面・部品の操作

## 守りの細かいところ

- 文字で見る守りは、レジストリの HKLM: という文字があると、読むだけ（Get-ItemProperty）でも許可が要る。読むだけで要るなら、powercfg などの別の命令で代わりに調べる
