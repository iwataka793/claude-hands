# iPhone の MCP サーバー（tools/iphone-mcp）

Claude が iPhone を「その場で」見て・動かすための道具。Claude デスクトップの拡張機能（.mcpb）として PC に入れる。
go.bat ＋ plan.json（OPERATIONS.md 3 章）より速い：トンネル・ログ・WebDriverAgent をつないだまま持ち、1 回の操作が数秒で返る。

## 入れ方（カオル・初回だけ）

1. Releases の最新の `mcp-N` から `iphone-dev-kit.mcpb` と `iphone-switch.zip` を PC に落とす
2. Claude デスクトップの「設定 → デスクトップアプリ → 拡張機能」の画面に `iphone-dev-kit.mcpb` をドラッグする（.mcpb のダブルクリックは Windows の「開くアプリ」選びになるだけ。Claude は自分で Claude デスクトップを操作できないので、ここはカオルの作業）
3. `iphone-switch.zip` を開いて、3 つの .bat をデスクトップに置く（スイッチ）
- 更新するときは、新しい `mcp-N` の .mcpb で同じように入れ直す

## スイッチ（カオルだけが切り替える）

| .bat | 状態 | Claude ができること |
| --- | --- | --- |
| iPhone-OFF | オフ | 何もできない（iPhone につながない） |
| iPhone-VIEW | 見るだけ | 状態・ログ・画面・部品の一覧・LiveContainer の中の一覧 |
| iPhone-OPERATE | 操作も可 | 上に加えて、タップ・スワイプ・文字で押す・アプリの前出し/終了・LiveContainer への入れ替え |

- 状態は `%USERPROFILE%\devkit\iphone-mcp\mode.txt`。MCP サーバーには切り替える道具を作っていない
- 置き場所を `%LOCALAPPDATA%` にしないこと：ストア版の Claude から動くと、AppData への書き込みが `AppData\Local\Packages\Claude_…\LocalCache\Local\` に振り分けられ、.bat が書いたファイルとずれる（0.1.2 で実際に起きた）
- 最後の動き（.bat で切り替えた時刻か、道具を使った時刻の新しい方）から 30 分（拡張機能の設定で変えられる）たつとオフに戻り、iPhone から切る。PC を起動し直しても、30 分以内ならスイッチはそのまま
- Claude デスクトップはこのサーバーを 2 つ同時に動かしたり、途中で起動し直したりする。だから起動のたびにオフを書く作りにはしない（0.1.3 まではそうしていて、.bat の切り替えが消えた）。時刻は `mode.txt` と `last-used.txt` の更新時刻で持つ
- iPhone につなぐのは 1 つのサーバーだけ（PC の中だけで開く 60204 番を札にする）。札を取れないと「別の iPhone の道具が使っています」と返す
- Claude 側の決まり：スイッチの .bat を押さない・mode.txt を書き換えない。オフなら「オンにしてください」と頼む

## 道具

| 道具 | スイッチ | 中身 |
| --- | --- | --- |
| iphone_status | いつでも | スイッチ・つながり・WDA の状態 |
| iphone_logs | 見るだけ〜 | つないだ時から覚えているログを正規表現で絞る。`wait_seconds` で出るまで待つ |
| iphone_screenshot | 見るだけ〜 | 画面（既定は 1/3 の JPEG、`full` で元の PNG） |
| iphone_elements | 見るだけ〜 | 画面の部品を「種類 「文字」 中心(x,y)」で |
| iphone_lc_list | 見るだけ〜 | LiveContainer の中のフォルダ（1 段） |
| iphone_tap / iphone_swipe | 操作も可 | WDA の単位（390x844） |
| iphone_tap_text | 操作も可 | 文字で部品を探して押す（例「実行」） |
| iphone_app | 操作も可 | activate / terminate（省略時 LiveContainer） |
| iphone_lc_install | 操作も可 | Releases の IPA（既定 latest の DevKit.ipa）を LiveContainer に入れ替える。前の中身は `%USERPROFILE%\devkit\iphone-mcp\backup\` に控え |

## 使い方のこつ（2026-11-24 に実機で確認）

- 最初の呼び出しでつなぐ（トンネルは 1 秒かからない）。WDA は最初に画面・部品・操作が要ったときに起動する（数秒）
- WDA の起動直後に撮った画面は、切り替えの途中でアイコンが大きく写ることがある。おかしければもう一度撮る
- 画面の部品（iphone_elements）はホーム画面だと 10 秒ほどかかる。押す場所を決めるだけなら、画面を撮るか iphone_tap_text の方が早い
- DevKit の 1 周：iphone_lc_install → iphone_app activate → iphone_tap_text「実行」→ iphone_screenshot（下の「build:」で版を確かめる）→ iphone_swipe → iphone_logs match=DEVKIT。全部で 30 秒ほど
- ログの行の頭は「PC が受け取った時刻」、その後ろが iPhone の時刻。iPhone の時計は 2〜3 秒遅れている
- 道具の返事には、LiveContainer の bundle id（チーム ID 入り）や iPhone の名前が出る。repo に写すときは伏せる
- 道具がおかしいときは server.log を読む。この会話から読むには、`~/devkit/iphone-mcp` の許可をカオルに 1 回もらう（device_request_folder_access。会話ごとに要る）。`AppData` の中は許可が出ない

## 中の作り

- Go。go-ios v1.3.2 をライブラリとして組み込み（トンネルは userspace、鍵は `%USERPROFILE%\devkit\go-ios` の selfIdentity.plist を使い回す）
- WDA への HTTP は USB の経路を直接つなぐ（PC にポートを開けない。`ios forward` は 0.0.0.0 で開くので使わない）
- ログは USB の経路で取り続け、3 万行まで覚える
- 記録：`%USERPROFILE%\devkit\iphone-mcp\server.log`
- ビルド：`.github/workflows/iphone-mcp.yml`（Linux で MCP の受け答えとスイッチの試験 → Windows exe → .mcpb → Releases `mcp-N`）
