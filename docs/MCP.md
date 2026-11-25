# iPhone の MCP サーバー（tools/iphone-mcp）

この md は道具の動かし方だけを書く。書いてあることは 2026-11-24・25 に実機（iPhone 16e・iOS 27.2.1）で確かめたもの。確かめていないものは「（未確認）」と書く。

Claude が iPhone を「その場で」見て・動かすための道具。Claude デスクトップの拡張機能（.mcpb）として PC に入れる。
go.bat ＋ plan.json（OPERATIONS.md 3 章）より速い：トンネル・ログ・WebDriverAgent をつないだまま持ち、1 回の操作が数秒で返る。

## 入れ方（カオル・初回だけ）

1. Releases の `mcp-N` のうち番号がいちばん大きいもの（一覧の並びは名前順のことがあるので、`mcp-11` が `mcp-7` より下に出ることがある）から `iphone-dev-kit.mcpb` と `iphone-switch.zip` を PC に落とす
2. Claude デスクトップの「設定 → デスクトップアプリ → 拡張機能」の画面に `iphone-dev-kit.mcpb` をドラッグする（.mcpb のダブルクリックは Windows の「開くアプリ」選びになるだけ。Claude は自分で Claude デスクトップを操作できないので、ここはカオルの作業）
3. スイッチは PC のデスクトップの「Claude 操作スイッチ」の窓で切り替える（pc-switch.zip の pc-setup.bat が置く。[PC.md](PC.md)）。`iphone-switch.zip` の .bat は予備
- 更新するときは、新しい `mcp-N` の .mcpb で同じように入れ直す

## スイッチ（カオルだけが切り替える）

| .bat | 状態 | Claude ができること |
| --- | --- | --- |
| iPhone-OFF | オフ | 何もできない（iPhone につながない） |
| iPhone-VIEW | 見るだけ | 状態・ログ・画面・部品の一覧・ポップアップを読む・文字が出るまで待つ・中の様子（iphone_info）・アプリの中のファイルを見る・読む・PC に取る |
| iPhone-OPERATE | 操作も可 | 上に加えて、タップ・長押し・スワイプ・ピンチ・文字で押す・文字入力・ボタン・ポップアップを押す・スクロールして探す・アプリの操作・まとめて実行・LiveContainer への入れ替え・アプリの中のファイルを書く・クリップボード・仮の位置 |

- 状態は `%USERPROFILE%\devkit\iphone-mcp\mode.txt`。MCP サーバーには切り替える道具を作っていない
- 置き場所を `%LOCALAPPDATA%` にしないこと：ストア版の Claude から動くと、AppData への書き込みが `AppData\Local\Packages\Claude_…\LocalCache\Local\` に振り分けられ、.bat が書いたファイルとずれる（0.1.2 で実際に起きた）
- 最後の動き（.bat で切り替えた時刻か、道具を使った時刻の新しい方）から 30 分（拡張機能の設定で変えられる）たつとオフに戻り、iPhone から切る。PC を起動し直しても、30 分以内ならスイッチはそのまま
- Claude デスクトップはこのサーバーを 2 つ同時に動かしたり、途中で起動し直したりする。だから起動のたびにオフを書く作りにはしない（0.1.3 まではそうしていて、.bat の切り替えが消えた）。時刻は `mode.txt` と `last-used.txt` の更新時刻で持つ
- iPhone につなぐのは 1 つのサーバーだけ（PC の中だけで開く 60204 番を札にする）。札を取れないと「別の iPhone の道具が使っています」と返す
- Claude 側の決まり：スイッチの .bat・窓のボタンを押さない・mode.txt を書き換えない。オフなら「オンにしてください」と頼む
- スイッチが足りないとき、道具は PC に「iPhone を『○○』にしますか？」の窓を出して 60 秒待つ（窓の作りは [PC.md](PC.md) の「スイッチの窓と『許可しますか？』」）。窓は 1 つで、iPhone と PC の両方を切り替えられる
- Claude 側の決まり（操作の中身）：お金（支払い・送金・銀行・証券・ポイント）・本人確認（マイナンバー・ワンタイムパスワード）・パスワード・セキュリティの設定にかかわるアプリと操作は、開く前にカオルの許可をもらう。それ以外の見る・動かす・アプリを開く・打つ・スクロールなどは、いちいち許可を取らずに進めてよい（2026-11-24 カオルが決めた）
- ロックは Claude からは外せない（パスコード・顔認証の端末）。外すのはカオル

## 使う前に

1. この会話で道具が見えるか：道具の一覧に `iphone_status` があるか。無ければ、この会話が PC の Claude デスクトップとつながっていない（カオルがスマホのアプリだけで話しているときなど）。カオルに「PC の Claude デスクトップでこの会話を開いてほしい」と頼む
2. `iphone_status` を呼ぶ（iPhone にはつながない・スイッチがオフでも使える）。見るところ：`スイッチ`、`つながっている`、`WebDriverAgent`、`版`
3. スイッチがオフなら、カオルにオンにしてもらう
4. ほかの道具を初めて呼んだとき、USB でつなぐ（1 秒かからない）。部品を使う操作（押す・打つ・部品の一覧など）が要るときだけ WDA を起動する（数秒〜10 秒）。画面を撮る・アプリの起動と終了・中の様子・ファイルは WDA なしで動く
5. Claude デスクトップは、道具ごとに初回だけ「許可」を聞いてくることがある。カオルが「常に許可」を選べば次から出ない（Claude からは変えられない）

## 外から見る（観察）

### 何が分かって、何が分からないか

| 分かる | 道具 |
| --- | --- |
| 前に出ているアプリ（名前と bundle id） | 操作の道具の返事の「前のアプリ」、`iphone_app` action=state |
| 画面の見た目 | `iphone_screenshot` |
| 画面の部品（種類・文字・作った人が付けた名前・位置・大きさ） | `iphone_elements` |
| アプリや iOS が出したログ | `iphone_logs` |
| アプリの通信の相手のポート・量・時間（相手の IP は伏せ字） | `iphone_logs`（下のレシピ） |
| アプリごとの通信量の合計と増えた分 | `iphone_logs`（下のレシピ） |
| ポップアップの文とボタン | `iphone_alert` read |
| 機種・iOS・ロック・容量・電池・言語・時刻の形式・デベロッパモード・構成プロファイルの数 | `iphone_info`（既定） |
| 動いているアプリ（新しく起動した順・pid） | `iphone_info` what=processes（`all` でアプリ以外も） |
| 入れたアプリの大きさ（本体・データ） | `iphone_info` what=apps |
| 落ちた記録（クラッシュなど。ほかのアプリの分も） | `iphone_info` what=crashes（`file` で中身） |
| 開発者の署名のアプリ（LiveContainer など）の中のファイル・写真などのフォルダ | `iphone_files` |

分からないもの：ほかのアプリのコード・関数・変数、暗号化された通信の中身、ほかのアプリが保存したデータ（中を開けるのは開発者の署名のアプリだけ）、ロック中の画面（真っ黒に写る）。

### ログの読み方

- 1 行の形：`PC が受け取った時刻 iPhone の時刻 端末名 プロセス名(サブシステム)[pid] <レベル>: 本文`
- iPhone の時計は PC より 2〜3 秒遅れている。操作の時刻（道具の返事に出る）と比べるときは、この分をずらす
- 道具が覚えているのは最新 3 万行（1〜2 分ぶん。iPhone のログは 1 分で 9 MB ほど流れる）。それより前は PC のファイル `%USERPROFILE%\devkit\iphone-mcp\logs\syslog.txt` から探す（`since_seconds` を大きくすれば自動。ファイルから来た行は先頭に日付が付く）。ファイルは 200 MB で `syslog.1.txt` に回すので、残るのは 1 時間ほど。**見たいことが起きたら、なるべく早く取る**
- ログのファイルは PC の中だけ。端末の名前などが入るので repo に写さない
- `max_lines` は新しい方から数える。うるさい行（例：ロック中の `bioLockoutState` は 1 秒ごとに出る）に合う正規表現にすると、見たい古い行が押し出される。**正規表現はできるだけ狭くする**
- 本文の中の値は、アプリが公開にしていないものは `<private>` になる（自分のアプリは `privacy: .public` を付ければ出る）
- アプリの実行ファイル名（ログのプロセス名）は、bundle id や画面の名前と違うことがある（画面の名前とまったく違う英数字のこともある）。まず操作した直後のログを広く見て、名前をつかむ

### レシピ（実機で確かめたもの）

| 知りたいこと | iphone_logs の match | 読み方 |
| --- | --- | --- |
| 自分のアプリ（DevKit）の動き | `DEVKIT` | コードの `logger.notice("DEVKIT ...")` が出す |
| あるアプリの通信 1 回ずつ | `process: 実行ファイル名:\d+ Duration.*bytes in/out` | `[伏せ字:自分のポート<->伏せ字:相手のポート]`、`bytes in/out: 受信/送信`。ボタンを押したのと同じ時刻に送信があれば、押した命令が出ている |
| あるアプリの通信量 | `Data Usage for bundle id` | WiFi・モバイルの合計と増えた分（delta） |
| アプリのプロセス名を探す | `実行ファイル名\(` か、操作の直後を広めに | `名前(サブシステム)[pid]` の名前の部分 |
| ロック・画面の状態 | 表の下の (1) | `backlightState=0` は画面が消えている扱い。`authed: false` はロック中 |
| 顔認証が止まっていないか | `bioLockoutState:forUser: -> ` | `-> 1` ならパスコードを入れるまで顔認証は使えない |

(1) `Passcode lock view|backlightState|lockScreenActive`

## 動かす（操作）

### 押す場所の決め方

- 画面の画像の座標は、そのまま `iphone_tap` の座標に使える（縮めた画像 390x844 = WDA の単位）。画像を見て押すのがいちばん早い
- 設定の検索欄は、押すと中身が消えた新しい欄に作り直される。`clear` は要らない（欄を押してそのまま打つ）。0.1.15 からは作り直されたらキーボードで消して打つ（未確認）
- 文字がはっきりしているボタンは `iphone_tap_text`。見えている部品から先に探す（0.1.7 から。前はマップの「出発」で隠れた方を押した）。それでも画面が変わらなかったら、`index` を変えるか、画像の座標で `iphone_tap` する
- `iphone_elements` は確実だが遅い（ホーム画面で 10 秒ほど）。押す場所が画像で分からないときに使う

### 返事の読み方

- 「画面：変わった／変わらない（○秒で落ち着いた）」：押した結果、画面が変わったか。「3 秒たってもまだ動いている」は、地図の案内中など動き続ける画面
- 「前のアプリ」：いま前に出ているアプリ。ほかのアプリに飛んだかが分かる
- `show: never` にすると待たずに返る。そのとき `log` はまだ何も拾えていないことが多いので、あとで `iphone_logs` を `wait_seconds` 付きで呼ぶ
- 画面が変わらないアプリもある（ボタンを押しても見た目が同じアプリなど）。その時は通信のログで、命令が出たかを確かめる

### 決まった動き

| したいこと | やり方 |
| --- | --- |
| アプリを開く | `iphone_app` action=activate、bundle=ホーム画面の名前（標準アプリは「マップ」「設定」など日本語）か bundle id。WDA が動いていなければ中から開く（0.1 秒） |
| アプリを起動し直す・終わらせる | `iphone_app` action=launch / terminate（中から。WDA なし） |
| 長い文を入れる | `iphone_clipboard` set で入れて、欄を長押しして「ペースト」 |
| 設定の項目を探す | app activate「設定」→ tap_text「検索」→ type で項目名（見るだけなら、ここで止める） |
| ホーム画面のページ送り | `iphone_swipe` (330,400)→(60,400)。戻るは逆。1 回 2.5 秒ほど |
| 検索欄に打って検索 | `iphone_type` field=欄の薄い文字（`partial: true`）、`enter: true` |
| 何回か続けて操作 | `iphone_steps`。途中で失敗したらそこで止まり、その時の画面が付く |
| 文字が出るまで待つ | `iphone_wait` |
| ポップアップ | `iphone_alert` read で文とボタンを見てから、accept / dismiss（`button` で文字を選ぶ） |
| ショートカットを動かす | `iphone_app` action=url、url=`shortcuts://run-shortcut?name=名前`（名前は英数字にしておくと楽）。ショートカットのアプリが前に出て動く |
| SideStore の入れ直し（7 日ごと） | 上のやり方で `DevKitRefresh`（LiveContainer の「Refresh All Apps」だけのショートカット。2026-11-25 に作った。オートメーションで毎週火曜 21:00 にも動く） |
| 例：マップで目的地へ案内 | app activate「マップ」→ type field「Apple マップ」に目的地・enter → 経路のボタン（「○分」）→ 緑の「出発」を座標で押す |

### ロック

- ロック中かどうかは `iphone_info` の「ロック」（iOS の lockdown の PasswordProtected）。かかっているとき「かかっている」、外れているとき「外れている」と出ることを確かめた（2026-11-25。サイドボタンでかけて 10 秒後）
- `iphone_button` lock：使えない。iOS 27 では WDA の lock がサイドボタンの長押しになり、Siri の案内が出るだけ（2026-11-25）。道具は断る。かけるならカオルにサイドボタンを押してもらう
- `iphone_button` unlock：外れない（パスコード・顔認証の端末）。ロックがかかっていれば「外すのはカオル」と返す
- ロック中は画面が真っ黒に写り、`iphone_button` home は「ロックが外れていない」と断られる
- 外す試しを続けると顔認証が止まる。くり返さない

### 処理中にロックさせない

- カオルに頼まれて長い処理をするときは、iPhone は設定の「自動ロック」を「なし」に（設定 → 検索「自動ロック」→ 1 つ目）、PC は pc_powershell で `SetThreadExecutionState`（ES_CONTINUOUS・ES_SYSTEM_REQUIRED・ES_DISPLAY_REQUIRED）。PC の常駐役の PowerShell は開きっぱなしなので、戻すまで効く
- 終わったら必ず元に戻す。iPhone の自動ロックの元の値は 4 分（2026-11-25）。PC は ES_CONTINUOUS だけを渡して戻す

### 仮の位置

- `iphone_location` set で仮の位置、reset で本当の位置に戻る（2026-11-25 に東京駅にして、マップで確かめた）
- iOS 17 からは instruments の LocationSimulation の窓口を使う（古い com.apple.dt.simulatelocation は iOS 27 では答えない）。道具がつないでいる間だけ効き、切れると本当の位置に戻る

### Safari などの中のページ（iphone_web）

- iPhone の設定 → アプリ → Safari → 詳細 →「ウェブインスペクタ」がオンのときだけ（2026-11-25 にオンにした）
- `list`：開いているページ（page の値・アプリ・題名・URL）。Safari の機能拡張の裏のページも出る
- `eval`：ページで JavaScript を動かして結果を返す（例 `document.title`）。page を省くと Safari のふつうのページ（http / https）
- 自分のアプリの WKWebView を見るには、アプリ側で `isInspectable = true` が要る（未確認）

## うまくいかないとき

| 症状 | まずやること |
| --- | --- |
| 道具が一覧に無い | 上の「使う前に」1 |
| 「スイッチが足りません」 | 「許可しますか？」の窓に 60 秒返事が無かったか、断られた。カオルに聞く |
| 「別の iPhone の道具が使っています」 | もう 1 つのサーバーがつないでいる。30 分待つか、カオルにスイッチをいったんオフ → オンにしてもらう |
| 「iPhone が見つかりません」 | USB・ロック・「このコンピュータを信頼」をカオルに確かめてもらう |
| 画面が真っ黒 | ロック中か画面が消えている。ログで確かめる |
| WDA のエラー | `iphone_status` の `WDA の最後のエラー`。7 日ごとの SideStore の更新が切れていないか |
| 道具が急に「つなげません」「トンネル」のエラーを返す | ロックや USB の抜き差しで切れた。0.1.8 からは次に呼んだとき自分でつなぎ直す。それでもだめならスイッチをオフ → 操作も可 |
| 道具そのものがおかしい | `%USERPROFILE%\devkit\iphone-mcp\server.log` を読む（device_list_dir / device_stage_files で。このフォルダが会話につながっていなければ、カオルに許可をもらう。`AppData` の中は許可が出ない） |

## 道具の一覧

| 道具 | スイッチ | 中身 |
| --- | --- | --- |
| iphone_status | いつでも | スイッチ・つながり・WDA の状態 |
| iphone_logs | 見るだけ〜 | つないだ時から覚えているログを正規表現で絞る。`wait_seconds` で出るまで待つ |
| iphone_screenshot | 見るだけ〜 | 画面（既定は 1/3 の JPEG、`full` で元の PNG） |
| iphone_elements | 見るだけ〜 | 画面の部品を「種類 「文字」 中心(x,y)」で |
| iphone_info | 見るだけ〜 | 中の様子：summary（既定）/ processes / apps / crashes。WDA なし |
| iphone_files（list / read / pull） | 見るだけ〜 | アプリの中のファイル（省略で LiveContainer）、`app=media` で写真などのフォルダ。pull は PC の `devkit\iphone-mcp\files` へ |
| iphone_alert（read） | 見るだけ〜 | ポップアップの文とボタンを読む |
| iphone_wait | 見るだけ〜 | 文字が出るまで（`gone` なら消えるまで）待つ |
| iphone_tap | 操作も可 | (x, y) をタップ。`count=2` でダブルタップ、`hold` で長押し。WDA の単位（390x844） |
| iphone_swipe | 操作も可 | スワイプ |
| iphone_tap_text | 操作も可 | 文字で部品を探して押す（例「実行」）。`partial` で一部一致 |
| iphone_type | 操作も可 | 文字を打つ。`field` で欄を指定、`clear` で消してから、`enter` で改行 |
| iphone_button | 操作も可 | home / volumeup / volumedown / lock / unlock / hide_keyboard / notifications / control_center / back |
| iphone_pinch | 操作も可 | 拡大・縮小（`scale`） |
| iphone_alert（accept / dismiss） | 操作も可 | ポップアップのボタンを押す（`button` で文字を選ぶ） |
| iphone_scroll_to | 操作も可 | 文字が出るまでスクロール。端で止まる。`tap` で押す |
| iphone_app | 操作も可 | activate / launch / terminate / state / url。`bundle` はホーム画面の名前（標準アプリは日本語名）か bundle id。省略時 LiveContainer |
| iphone_steps | 操作も可 | 上の操作を並べて 1 回で行う。失敗したらそこで止めてその時の画面を返す |
| iphone_files（write / push） | 操作も可 | アプリの中にファイルを書く・置く（テストのデータ・設定） |
| iphone_clipboard | 操作も可 | クリップボードを読む・入れる（パスワードが入っていることがあるので、読むのは要るときだけ） |
| iphone_location | 操作も可 | 仮の位置（set / reset） |
| iphone_web（list） | 見るだけ〜 | Safari などの中のページの一覧 |
| iphone_web（eval） | 操作も可 | ページで JavaScript を動かす |
| iphone_devkit | 見るだけ〜（/cmd は操作も可） | DevKit ののぞき窓に聞く（下の「DevKit の開発の 1 周」） |
| iphone_lc_install | 操作も可 | Releases の IPA（既定 latest の DevKit.ipa）を LiveContainer に入れ替える。前の中身は `%USERPROFILE%\devkit\iphone-mcp\backup\` に控え |


## DevKit の開発の 1 周

`iphone_lc_install` → `iphone_app` launch（LiveContainer）→ `iphone_tap_text`「実行」→ `iphone_devkit`（版と中の様子）→ 操作 → `iphone_logs` match=DEVKIT。全部で 30 秒ほど。

### のぞき窓（iphone_devkit）

- DevKit は iPhone の中のループバックの 8765 番で待つ（Wi-Fi など外からは届かない）。道具は USB の経路で直接つなぐ（届いた接続は 127.0.0.1 から来る）
- `/state`：版（commit）・画面に出ている行・行の数・呼ばれた回数
- `/cmd?do=clear`（行を消す）・`/cmd?do=add&text=…`（行を足す）：画面を触らずに DevKit を動かす
- DevKit が前で動いているときだけ答える（裏に回ると iOS が止める。前に戻ると待ち受けを始め直す）
- 新しい画面・機能を作ったら、ここに道を足す（App/Sources/Probe.swift の route）

## 写すときの注意

道具の返事には、LiveContainer の bundle id（チーム ID 入り）・iPhone の名前・アプリの一覧・端末の識別番号が出る。repo や記録に写すときは伏せる。

## 中の作り

- Go。go-ios v1.3.2 をライブラリとして組み込み（トンネルは userspace、鍵は `%USERPROFILE%\devkit\go-ios` の selfIdentity.plist を使い回す）
- WDA への HTTP は USB の経路を直接つなぐ（PC にポートを開けない。`ios forward` は 0.0.0.0 で開くので使わない）
- ログは USB の経路で取り続け、3 万行まで覚え、PC のファイルにも書く
- 画面は、WDA が動いていれば WDA、動いていなければ instruments（1 秒かからない）で撮る。前のアプリは WDA が動いているときだけ返す
- 中の様子・アプリの起動と終了は instruments・lockdown・installation_proxy・coredevice の窓口に直接たのむ（WDA なし）
- 記録：`%USERPROFILE%\devkit\iphone-mcp\server.log`
- ビルド：`.github/workflows/iphone-mcp.yml`（Linux で MCP の受け答えとスイッチの試験 → Windows exe → .mcpb → Releases `mcp-N`）
