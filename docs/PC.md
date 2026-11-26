# PC の道具（tools/pc-mcp）

この md は道具の動かし方だけを書く。GitHub の Windows（windows-latest）で通しの試験をしたもの。カオルの PC でまだ確かめていないものは「（未確認）」と書く。

Claude が Windows の PC を、画面を撮って押す（computer use）のではなく、PowerShell・ファイル・Windows の UI オートメーションで直接見て・動かすための道具。1 回の呼び出しは 0.1 秒前後（最初だけ PowerShell の起動で数秒）。

## 作り

| 役 | ファイル | 何をするか |
| --- | --- | --- |
| 拡張機能 | `claude-hands-pc.mcpb`（中に `pc-mcp.exe`） | Claude デスクトップの中で動く。道具を並べて、中身は常駐役に頼むだけ |
| 常駐役 | `%USERPROFILE%\devkit\pc-agent\pc-agent.exe` | カオルのスイッチ（.bat）で起動・停止する。PowerShell を開きっぱなしにして実際の作業をする。PC の中（127.0.0.1:60210）だけで受け付け、合言葉（token.txt）がないと答えない |

常駐役を分けたわけ：ストア版の Claude から動くプログラムは、AppData への書き込みが別の場所に振り分けられる（iPhone の道具で確かめた）。常駐役はカオルがエクスプローラーから起動するので、振り分けを受けない。

## 入れ方（カオル・最初と、新しい版にするとき）

1. Releases の最新の `pc-N` から `claude-hands-pc.mcpb` と `pc-switch.zip` を落とす
2. `pc-switch.zip` を開いて、中の `pc-setup.bat` を実行する（常駐役を `devkit\pc-agent` に、スイッチの窓を `devkit\switch-app` に置き、デスクトップに窓のショートカット「Claude 操作スイッチ」を作る（.bat は zip の中の予備）。動いている常駐役は止めてから入れ替える）
3. Claude デスクトップの「設定 → デスクトップアプリ → 拡張機能」に `claude-hands-pc.mcpb` をドラッグする
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

- `%USERPROFILE%\devkit\switch-app\devkit-switch.vbs` を開くと、iPhone・PC・ブラウザのスイッチを 1 つの窓で切り替えられる（「パスワード・セキュリティ・お金」の 10 分 許可は PC とブラウザで共通）（中身は .bat と同じ：mode.txt を書く。PC の許可は UAC を通って allow.txt を書く）。右下の通知領域に常駐し、窓を閉じても残る。右上の「—」で最小化（タスクバーへ）、ボタン以外のどこをつかんでも窓を動かせる（2026-11-26）。終わるのは通知領域の右クリック →「終わる」。デスクトップのショートカット「Claude 操作スイッチ」（アイコンは devkit-switch.ico）から開ける。もう動いているときは、2 つ目は起動せず、動いている方の窓を前に出す（名前つきの合図 Local\DevKitSwitchShow）
- 道具がスイッチ不足で止まるとき、道具は `%USERPROFILE%\devkit\requests\<種類>.txt`（iphone-view・iphone-operate・pc-view・pc-operate・browser-view・browser-operate・pc-allow。中身は理由 1 行）を置き、窓を `/tray` で起こす。窓が「○○にしますか？」を出し、カオルが「許可する」か「断る」を押す
- 道具は返事を待つ：60 秒（PC の許可は 90 秒）。許可されたらそのまま続ける。断られた・時間切れなら、今までどおり「スイッチが足りません」を返す
- 窓は 55 秒（許可は 80 秒）で自動的に断る。2 分より古い頼みは捨てる。既定のボタンは「断る」（Enter・Esc で断る）
- Claude 側の決まりは変わらない：mode.txt・allow.txt を書かない、窓のボタンを押さない。requests に置くのは道具だけ（Claude が手で置いてよいのは、カオルに見た目を見せる試しのときだけ）
- 許可の窓は、許可が要る命令（Remove-Item・HKLM のレジストリ。読むだけでも）で自動で出る。カオルの手間になるので、ごみ箱（pc_trash）・アプリの一覧（pc_apps）など許可の要らない道具で済むならそちらを使う。どうしても要るときは、先にカオルに一言伝える
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

## ブラウザ（2026-11-26 から。browser_ で始まる道具）

カオルのいつもの Chrome（ログイン済み）を、画面を撮らずに**文字と番号**で読んで、**何手もまとめて**動かす。1 回の呼び出しは PC の中で 0.01〜0.1 秒（手元の試験）。往復が減るので、Claude in Chrome や内蔵ブラウザより速い。

### つなぎ方（カオル・最初に 1 回）

1. Chrome で `chrome://inspect/#remote-debugging` を開き、リモートデバッグを許可する（オン）。Chrome が User Data の `DevToolsActivePort` に番号を書く
2. 道具を初めて使うと、Chrome が「リモートデバッグを許可しますか？」を出す。カオルが「許可」を押す（押さなければ 75 秒で止まる）
3. つないでいる間、Chrome の上に「自動テスト ソフトウェアによって制御されています」の帯が出る。ブラウザのスイッチをオフにする・30 分使わないと切れる。次に使うとき、また「許可」が出る
- カオルの PC で確かめた（2026-11-26）：User Data の DevToolsActivePort で見つかり、Chrome の「許可」1 回でつながった。ポーカーのゲームで 1 手 1.7〜4 秒（道具の往復は 0.1 秒ほど。残りはゲームの待ち）
- つなぐのは PC の中（127.0.0.1）だけ。Chrome 144 からの公式の仕組み（Chrome DevTools MCP の autoConnect と同じ）。2026-11-26 の PC の Chrome は 158

### スイッチと許可（ブラウザは PC と別のスイッチ・許可は共通。2026-11-26 から）

スイッチの窓の「ブラウザ」の段で切り替える（置き場所 `%USERPROFILE%\devkit\browser\mode.txt`。書くのはカオルの窓だけ）。PC がオフでもブラウザだけ使える（常駐役はどちらかがオンなら動き、両方オフで終わる）。ブラウザをオフにする・30 分使わないと、Chrome とのつながりをすぐ切る（上の帯も消える）。足りないときは道具が「ブラウザ（Chrome）を〜にしますか？」の窓を出して 60 秒待つ（頼みの種類 browser-view・browser-operate）。PC かブラウザのスイッチを切り替えると、許可（10 分）は取り消し。

| ブラウザのスイッチ | できること |
| --- | --- |
| 見るだけ | browser_tabs（一覧）・browser_read・browser_screenshot |
| 操作も可 | 上に加えて browser_do・browser_js・タブを開く／前に出す／Claude が開いたタブを閉じる |
| 許可（PC-ALLOW・10 分） | 銀行・証券・決済・ログイン・パスワード・2 段階認証などのページの操作、クッキー・保存領域・パスワードにかかわる JavaScript、file:// のページ、カオルが開いたタブを閉じる |

- **許可があっても打たない**：パスワード欄・カード番号・セキュリティコード・ワンタイムコードの欄（type="password"・autocomplete の cc- など）。その欄では Tab・Enter・Esc・矢印以外のキー（貼り付けの Ctrl+V も）を押さない。値は読んでも「（入力あり・伏せる）」になる
- **行かない**：chrome://・chrome-extension://・devtools:// などの設定・拡張機能のページ（パスワードの設定を含む）
- 決まりは文字で見る守り（「うっかり」止め）。Claude 側の決まり（CLAUDE.md：パスワード・2 段階認証のコードはカオル、お金は先に確認）と組み合わせる

### 道具

| 道具 | スイッチ | 中身 |
| --- | --- | --- |
| browser_tabs | 見るだけ〜 | タブの一覧（id・★＝道具がいま使うタブ・＋＝Claude が開いたタブ）。`action`：new（url）・select・close |
| browser_read | 見るだけ〜 | 押せる部品の一覧（`[番号] 役「名前」 = 値`・画面の外・押せない・パスワード欄の印）。`mode`：elements（既定）・text・all。`filter` で絞る。canvas・iframe があれば数を出す |
| browser_do | 操作も可 | `steps` を順に：click（番号・`text=…`・`css=…`）・double・click_xy・type（ref・text・clear・enter）・select・key（Enter・Ctrl+A など）・scroll・wait・wait_for（文字か css）・goto・back・js。終わったら部品の一覧を返す（`read`：elements・text・all・none）。`front: true` でタブを前に出す（見えていないと止まるゲームなど） |
| browser_js | 操作も可 | ページの中で JavaScript（コンソールと同じ書き方・await 可・最後の値を返す） |
| browser_screenshot | 見るだけ〜 | 見えている所か部品 1 つ（`target`）を JPEG で。画像の座標 → click_xy の座標の換算が付く。文字で読めないときだけ |

- `tab` は一覧の id の頭（4 文字以上）か、題名・URL の一部。省くと最後に使ったタブ（無ければ見えているタブ）。**並び順の番号は使えない**（カオルがタブを開け閉めすると変わり、2026-11-26 に別のタブへ移ろうとしてしまった）
- 番号は次に読むまで有効。ページが変わったら読み直す。番号の代わりに `text=決める` のように名前で押してもよい
- 押す・打つは本物のマウス・キーの合図（CDP の Input）なので、canvas のゲームも押せる（isTrusted=true を試験で確かめた）

### Claude 側の決まり（ブラウザ）

- **カオルのタブ（＋が付いていないタブ）は、頼まれたときだけ読む・動かす**。ふだんの作業は browser_tabs の new で自分のタブを開いてそこでやり、終わったら閉じる。カオルのタブにはカオルが見ているもの（個人的なページを含む）があるので、題名や中身を返事・repo・記録に写さない
- 一覧（browser_tabs）にはカオルのタブの題名・URL も出る。必要な行（自分のタブ）だけを見て、ほかには触れない
- 常駐役の記録（audit.log）には、道具の返事の 1 行目（タブの題名など）が残る。PC の中だけの記録なので repo には写さない
- ダウンロードは、頼まれたものだけ。ファイルはダウンロードのフォルダに入るので、使ったら片付ける（消す前に確認）
- ダウンロードは browser_do の goto でファイルの URL へ行く（GitHub の Releases の一覧の a を押す方法は、一覧があとから読み込まれるので押せないことがある）。goto は「net::ERR_ABORTED」で止まったと返すが、ダウンロードは始まっている。pc_powershell でダウンロードのフォルダを見て確かめる（1 つの goto ごとに 1 回の browser_do に分ける）

### canvas（絵で描かれた）ページ・ゲーム

- 部品の一覧に出ない。まず browser_js でページの中の作り（どのライブラリか：Phaser・PixiJS など）を見て、**画面に出ている文字と絵だけ**を読む。相手の伏せたカードなど、画面に出ていないものは読めても使わない
- Phaser 3 の例（2026-11-26 のポーカー）：`Phaser.Scenes.SceneManager.prototype.update` を包んで game を拾い、見えているシーンの Text と、カードの絵の番号（frame）を読む。押すのは canvas への pointerdown／pointerup（ページの中から送る）か、browser_do の click_xy
- browser_js は「コンソールと同じ書き方」なので、一番上の `const`・`let` は次の呼び出しまで残り、同じ名前を 2 回書くとエラーになる。続けて使う関数は `window.名前 = …` に置く

### 速く使うコツ

- 次の手が読めるときは 1 回の browser_do にまとめる（開く → 打つ → 押す → 待つ → 読む）
- 読む量を絞る（`filter`・`max`）。画像は最後
- 毎回同じ手順なら、browser_js に台本をまとめるか、PC に置くプログラムにする

## よく使う流れ

| したいこと | やり方 |
| --- | --- |
| 容量が減った原因 | `pc_disk`（引数なし）でドライブと膨らみやすい場所 → 大きい所を `pc_disk path=... files=true` で掘る |
| アプリの不具合 | `pc_problems app=アプリ名 hours=72` → 落ちた回数・エラーの中身・WER の控えの場所 → そのアプリのログを `pc_files find`（`*.log`・`sort: new`）→ `pc_files read tail=200` |
| 重い | `pc_processes`（メモリ）と `pc_processes sort=cpu` |
| アプリを操作 | `pc_open` → `pc_windows` → `pc_ui window=... filter=...` で名前を見る → `pc_ui_do` → 結果を `pc_ui` か `pc_screenshot window=...` で確かめる |
| 画像を見て押す | `pc_screenshot` → 画像の座標を読む → `pc_ui_do action=click_xy from_image=true x=.. y=..` |
| ブラウザで作業 | `browser_tabs` → `browser_read`（部品の番号）→ `browser_do`（何手もまとめて。終わりに部品の一覧が返る）→ 足りなければ `browser_read mode=text filter=…` |

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
- 試験：`tools/pc-mcp/test/scenario.py`（GitHub の Windows で毎回。手元では Linux＋pwsh でも動く）。ブラウザは headless の Chrome（一時のプロフィール・`--remote-debugging-port=0`）と `test/site/` の試験のページで試す（`PC_BROWSER_USERDATA` で User Data の場所を変える）
- ブラウザ：`browser*.go`。WebSocket と CDP は外の部品を使わず手で書いた（127.0.0.1 だけ）。ページの中に置く小さな道具箱（dkLib）が、部品の番号・名前・伏せる欄の見分けをする
- ビルド：`.github/workflows/pc-mcp.yml`（Linux でビルド → Windows で通しの試験 → Releases `pc-N`）

## カオルの PC で確かめたこと（2026-11-25）

- ストア版の Claude の中の拡張機能から、常駐役（127.0.0.1）と token.txt に届く：OK
- PC-ALLOW.bat：UAC の「はい」→ 許可 10 分 → 鍵の場所（selfIdentity.plist）をごみ箱へ送れた：OK
- 容量・不具合の記録・PowerShell・ごみ箱（73 個）：OK。1 回 1〜2 秒
- .bat の日本語の表示（CP932）：OK（カオルに見てもらった）
- まだ：画面の拡大率（DPI）が 100% でないときの座標、画面・部品の操作

## 守りの細かいところ

- 文字で見る守りは、レジストリの HKLM: という文字があると、読むだけ（Get-ItemProperty）でも許可が要る。読むだけで要るなら、powercfg などの別の命令で代わりに調べる
