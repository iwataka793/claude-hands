# 経過の記録

新しいものが上。

## 2026-11-25 SideStore の「Refresh All」で日数が 7 日に戻らない理由

- カオルが My Apps の Refresh All を押した（08:41）。日数は WebDriverAgent 6 日・LiveContainer 7 日のまま
- SideStore の記録（ConsoleLogs。PC に取って必要な行だけ読み、取った写しはごみ箱へ）：ログイン → チームと証明書の確認 → App ID の一覧を取って終わり。プロファイルの取り直し・入れ直しはしていない（installd にも入れ直しの記録なし）
- 無料の Apple ID の App ID には期限がある：WebDriverAgent 2026-12-01 18:25・LiveContainer 2026-12-02 06:54（日本時間）。画面の日数はこれまでの日数。App ID の期限より先には延ばせないので、期限前の Refresh は何もしない
- つまり今は正常。入れ直しが効くのは App ID の期限が来てから。12 月 1 日の夕方以降に SideStore で Refresh All（か、ショートカット DevKitRefresh）をして、日数が 7 日に戻るかを見る。LiveContainer が切れると中の SideStore も開けなくなるので、LiveContainer の期限（12 月 2 日 06:54）より前に試す

## 2026-11-25 仮の位置・Safari のページ（mcp-12〜14）

- 仮の位置：go-ios の simlocation（com.apple.dt.simulatelocation）は iOS 27 では「InvalidService」。instruments の LocationSimulation（DTX）を使えば動いた。つないでいる間だけ効くので、道具がつないだまま持ち、reset か切るときに止める。東京駅にしてマップで確かめ、reset で家の近くに戻った
- Safari のページ：設定の「ウェブインスペクタ」をオンにした（設定 → アプリ → Safari → 詳細。設定の検索では出てこない）。iphone_web で example.com の題名・段落の数を読めた。機能拡張の裏のページ（safari-web-extension://）も一覧に出るので、page を省いたときは http / https のページを選ぶ
- スイッチ：1 回だけ「オフです」と出てすぐ「操作も可」に戻った。ファイルの読みそこねとみて、読み直すようにした（0.1.13）
- スクロールの端：設定のように同じ形の行が並ぶ一覧で、まだ途中なのに「端」と止まった。端の判定を細かくした（0.1.13。7 回スクロールして「詳細」まで行けた）
- 消してから打つ：設定の検索欄は押すと新しい欄に作り直され、前の部品は「stale」になる。0.1.14 の「いま打てる欄を探し直す」でもだめ（WDA が古い部品を返す）。0.1.15 はキーボードの削除キーで消してから打つ（未確認）
- 処理中のロック止め：今回も iPhone の自動ロックを「なし」・PC の画面を消さないにして、終わって 4 分・元に戻した

## 2026-11-25 通しの作業：ショートカット・SideStore の入れ直し・DevKit ののぞき窓・自動つなぎ直し（mcp-11・build-7）

- 処理中のロック止め：iPhone の自動ロックを 4 分 → なし → 終わって 4 分に戻した。PC は SetThreadExecutionState で画面とスリープを止め、終わって戻した（カオルは動画を見ていた。邪魔はしていない）
- つながりが切れる：カオルがロックをかけたあと、トンネルが落ちてログも止まった（07:47）。0.1.7 は切れたまま使い続けたので、スイッチのオフ → オンでつなぎ直した。0.1.8 から、道具を呼ぶたびに（3 秒おきに）USB に端末がいるか・トンネルの口が開いているかを見て、切れていればつなぎ直す
- ショートカット：iOS 27 のショートカットは「+」で説明を打つ画面が出る。右上の「編集」で、ふつうにアクションを足せる。名前は名前の横の「v」→「名称変更」
- SideStore の入れ直し：LiveContainer のアクション「Refresh All Apps」だけのショートカット `DevKitRefresh` を作り、`shortcuts://run-shortcut?name=DevKitRefresh` で動かした。「All apps have been refreshed.」が出た（4 秒ほど）。そのあとの SideStore の My Apps：WebDriverAgent あと 6 日・LiveContainer あと 7 日。ただ、その間に installd の入れ直しのログは無く、どちらが入れ直されたかは確かめられていない。次に日数が減ってきたら、前と後の日数で確かめる
- DevKit ののぞき窓：App/Sources/Probe.swift（iPhone の中の 8765 番）と、道具 iphone_devkit。1 回目（127.0.0.1:8765 に縛った書き方）は ready になるのに接続が届かなかった（道具は EOF）。ループバック全体で待つ書き方にしたら届いた。接続は 127.0.0.1 から来る。/state・/cmd?do=add・clear・スワイプした行が取れることを実機で確かめた
- Releases の mcp-N は一覧の並びが名前順のことがあり、mcp-11 が mcp-7 の下に出る。番号で選ぶ
- ログを PC のファイルから探すのも確かめた（3 万行からあふれた計算機のログを、ファイルから返した）

## 2026-11-25 iPhone の道具 0.1.7（mcp-7）：中を見る・中から動かす

- 先に go-ios のコマンドで、iOS 27 で窓口が答えるかを 1 つずつ確かめた（PC の道具から ios.exe を実行。go-ios の場所は鍵があるので PC-ALLOW の間だけ使える）
  - 答えた：容量・日時・電池・言語・時刻の形式・デベロッパモード・構成プロファイル・端末の情報（lockdown・afc）、動いているもの（instruments。トンネルが要る）、アプリの起動・終了（instruments processcontrol。WDA なし）、クリップボード（coredevice）、WDA なしの画面（instruments 0.8 秒）、落ちた記録の一覧（coredevice の fileservice。昔の crashreport の窓口は EOF で答えない）、ホーム画面の並び
  - 答えない：Safari の中（設定の「Web インスペクタ」がオフ）、仮の位置（開発者のディスクイメージが要る）
  - 電池の 1 % ごとの温度・容量の細かい値は 0 で返る（iOS 27 では出さないらしい）。充電の回数は出る
- 0.1.7 で入れたもの：iphone_info・iphone_files（iphone_lc_list をまとめた）・iphone_clipboard・iphone_location。画面を撮る・前のアプリを知るためだけに WDA を起こさない。iphone_app の launch / terminate / state と、WDA が無いときの activate を中から。ログを PC のファイルにも残す。文字で押すは見えている方から
- 実機で確かめた：上のどれも WDA なしで 1〜3 秒。計算機の起動 → 終了、設定を中から開く（0.1 秒）→ 検索欄に打つ。ログのファイルからの検索（3 万行からあふれた分）
- ログは 1 分で 9 MB ほど。覚えている 3 万行は 1〜2 分ぶんしかない
- ロックの判定（lockdown の PasswordProtected）：カオルにサイドボタンでかけてもらい「かかっている」、unlock は「外すのはカオル」と断ることを確かめた
- iOS 27 では WDA の lock がサイドボタンの長押しになり、Siri の案内が出る（「今はしない」で閉じた）。lock は道具で断るようにした
- SideStore の入れ直し：LiveContainer が RefreshAllIntent（ショートカットから呼べる形）を持っている。ショートカットで動くかは未確認
- 失敗：アプリの一覧を PowerShell で読んだとき、文字コードの読み違いで JSON が読めず、エラーの文に一覧の中身が全部出た。repo にも会話にも写していない。iPhone の JSON は `-Encoding UTF8` で読み、エラーは try / catch で中身を出さない

## 2026-11-25 PC の道具をカオルの PC に入れた・片付け・遠隔で電源の下調べ

- pc-4 を入れた（pc-setup.bat・拡張機能）。拡張機能 → 常駐役のつながり、PC-ALLOW の UAC の流れ、容量・不具合の記録・PowerShell・ごみ箱が、カオルの PC で動いた
- 容量：C は 475 GB 中 141 GB 使用。大きいのは Docker の仮想ディスク 7.9 GB・休止状態のファイル 6.2 GB など。急ぎの片付けは要らない
- 不具合の記録：起動のたびに disk のコントローラーのエラー（Harddisk1・2）と BitLocker の鍵のエラーが出る。Intel の部品が 2 回落ちた。原因はまだ調べていない
- 片付け：PC に残っていた SideStore のログの写し 4 つ（署名用の Apple ID のメール・認証のための端末の情報入り。パスワードそのものは無し）はカオルが消した。鍵の写し selfIdentity.plist（devkit-setup に間違ってできたもの）と、調べものの古い記録 73 個（68 MB）をごみ箱へ
- 遠隔で電源：機種は Yoga Slim 7 14IMH9。スリープはモダン スタンバイ（S0）だけ。LAN は USB の ASIX アダプターで、「モダン スタンバイ中にマジックパケットで起こす：有効」。起こせる機器は USB4 ルート ルーター（アダプターの先）・キーボード・マウス。スリープからなら起こせる見込みが高い。電源を切った状態からは無理。高速スタートアップは有効。最後は実際に試す（スリープ → iPhone の Wake-on-LAN のアプリで合図）

## 2026-11-25 PC の道具（pc-mcp）を作った

- カオルの頼み：PC も速く。容量が膨らんだときやアプリの不具合を、どこを見ればいいか分からないので任せたい。パスワード・セキュリティ・お金はカオルの許可なしでは触れないようにする
- 決めたこと（カオル）：許可は道具で止める＋PC-ALLOW.bat（10 分）。触ってよい範囲は Windows 本体と要許可の場所以外は全部（消すのはごみ箱・先に確認）。iPhone の道具には今は付けない
- 作り：拡張機能（pc-mcp.exe）＋常駐役（pc-agent.exe。カオルの .bat で起動・停止、127.0.0.1 と合言葉）。ストア版 Claude の AppData の振り分けを避けるため、作業は常駐役がする。PowerShell は開きっぱなし（1 回 0.1 秒前後）
- 許可は、UAC を通して管理者しか書けない %ProgramData%\pc-agent に置く。Claude の PowerShell（普通の権限）からは書けないので、ここだけは本当に抜けられない。台本の中身で止める守り（命令・メソッド・場所）は「うっかり」用
- GitHub の Windows で通しの試験（test/scenario.py）：全部 OK（pc-4）。途中で見つけた不具合：窓を前に出すのに Alt キーを使うとメニューが開き、打った文字でメモ帳が閉じた → Alt を使わない・前に出せなければ打たない。メモ帳の文書の欄は TextPattern なので、読む・入れるをそれに合わせた
- カオルの PC での確認はまだ（PC.md の最後に一覧）

## 2026-11-24 公開 repo の見直し

- 個人の情報（行き先の名前・入っているアプリの名前）が LOG.md と MCP.md に入っていたので、一般的な書き方に直した。Apple ID・メール・チーム ID・端末の識別番号・端末名・PC のユーザー名は、今のファイルにも履歴にも無かった（作者のメールは GitHub の noreply）
- 個人の情報が入った 2 コミットは、1 つにまとめ直して履歴から消した（force push。カオル了承）

## 2026-11-24 MCP.md を「道具の動かし方」として書き直した

- カオルの頼み：判断（やってよい・だめ）は書かず、この道具の正しい動かし方だけにする。md を読み違えて過剰に動かないように
- 足したもの：使う前の確かめ方、外から見る（分かる・分からない、ログの読み方、確かめたレシピ：DevKit・アプリの通信・通信量・ロックの状態）、動かす（押す場所の決め方、返事の読み方、決まった動き、ロック）、うまくいかないとき
- 実機での例：マップで目的地を検索して案内を始めた（約 1 分・5 操作。「出発」が 2 個あり、文字で押すと見えない方を押した）。ほかのアプリでボタンを押し、画面は変わらないが、通信のログで機器へ命令を送ったのを確かめた
- この会話がスマホのアプリだけになると、iPhone の道具は一覧から消える（PC の Claude デスクトップとのつながりの道具だから）

## 2026-11-24 操作の道具を増やした（0.1.6）・ロックの試し・操作の決まり

- 足した道具：文字入力・ボタン（ホーム・音量・ロックなど）・長押し/ダブルタップ・ピンチ・ポップアップ・待つ・スクロールして探す・アプリを名前で・URL・まとめて実行（iphone_steps）。操作の道具は、あとの画面の変化・前のアプリ・合うログを返す
- jsonschema の説明文は「単語=」で始めると起動時に panic する（CI の起動の試験で見つけた）
- 実機で確かめた：まとめて実行でホーム画面のページ送り、スワイプのあとの画面付きの返事。ページ送りのスワイプは 1 回 2.5 秒ほど（縮められないか後で見る）
- ロック：かけるのはできる（WDA は「ロックの完了を待って時間切れ」と返すが、実際はかかる）。外すのはできない。WDA の unlock は画面をつけてパスコードの画面まで進めるだけで、「成功」と返す。iOS は画面を消えている扱いにしていて、顔認証は試されない。ホームに戻る命令は「ロックが外れていない」と断られる。何回も試したら顔認証が止まった（パスコードを入れるまで）
- カオルが「1 回目は勝手に立ち上がった」と見ていたが、その時刻のログは 3 万行からあふれて消えていて確かめられなかった
- 決まり（カオル）：お金・本人確認・パスワード・セキュリティにかかわるアプリと操作は許可が要る。それ以外の操作はいちいち許可は要らない（MCP.md に書いた）

## 2026-11-24 MCP の道具が実機で全部動いた（0.1.4）

- 状態 → ログ → 画面 → 部品 → LiveContainer を前に → 「実行」→ スワイプ → ログ → LiveContainer の中の一覧 → 入れ替え（build-3 → build-5）まで、PC の画面を一度も触らずに通った
- スワイプはアプリのログに届いた（`DEVKIT swipe=right commit=e1e0f66`、入れ替え後は `swipe=left commit=d6c6cd7`）
- かかった時間（Claude 側の待ちも含む目安）：最初のログ 10 秒（つなぐ時間と待ち 5 秒を含む）、最初の画面 9 秒（WDA の起動込み）、2 回目からの画面 数秒、部品の一覧 10 秒（ホーム画面）、入れ替え 7 秒、入れ替えから起動を確かめるまで 25 秒ほど。go.bat の 1 周（40〜90 秒）より速く、PC の画面も使わない
- WDA の起動直後の画面は、切り替えの途中でアイコンが大きく写った。撮り直せば正しい
- server.log を読むと、Claude デスクトップはサーバーを 2 つ同時に動かしていた（0.1.4 の札で、iPhone につなぐのは片方だけ）

## 2026-11-24 MCP 拡張機能を入れた・スイッチが効かない原因

- 拡張機能「iPhone（iphone-dev-kit）」を Claude の 設定 → デスクトップアプリ → 拡張機能 から入れた（.mcpb のダブルクリックは Windows の「開くアプリ」選びになるだけ）。この会話から 10 個の道具が見え、iphone_status は返事をした
- iPhone-OPERATE.bat を押してもスイッチが「オフ」のまま。中身は正しかった
- 原因：Claude がストア版（MSIX）のため、そこから動く道具の AppData への書き込みは `AppData\Local\Packages\Claude_…\LocalCache\Local\iphone-mcp` に振り分けられていた。.bat は本物の `AppData\Local\iphone-mcp` に書くので、別のファイルを見ていた（両方のフォルダが実在するのを確認）
- 直し：置き場所（mode.txt・server.log・backup）を `%USERPROFILE%\devkit\iphone-mcp` に変えた（0.1.3）。.bat も同じ場所に書く
- 0.1.3 で .bat は効いたが、すぐまたオフに戻った。server.log を見ると、サーバーが同じ時刻に 2 つ起動（20:24:43 に 2 回 start）し、20:25:40 に 1 つが終わって起動し直していた。起動のたびに「オフ」を書く作りだったため、そのたびに消えていた
- 直し（0.1.4）：起動時にオフを書かない。30 分の時計は mode.txt と last-used.txt の更新時刻で持ち、どのサーバーからも同じに見える。iPhone につなぐのは札（127.0.0.1:60204）を取れた 1 つだけ

## 2026-11-24（MCP サーバーの試作：tools/iphone-mcp）

- 速くするため、PC の画面をクリックして台本を動かすやり方から、Claude デスクトップの拡張機能（MCP サーバー）に移る（カオルの了承済み）
- Go で書き、go-ios を組み込んだ 1 つの Windows exe。トンネル・ログ・WDA をつないだまま持つ。道具は 10 個（docs/MCP.md）
- カオル用の 3 段スイッチ（オフ・見るだけ・操作も可）。起動時はオフ、30 分使わないとオフ。サーバーには切り替える道具を作らない
- Actions：Linux 版で MCP の受け答え（道具 10 個・スイッチがオフなら画面を撮れない）を確かめてから、Windows exe と .mcpb を作って Releases の mcp-N に置く。mcp-2 まで通った
- 未検証：PC に入れて、実機でつなぐところ（次に PC と iPhone がつながったとき）

## 2026-11-24（新しいセッション役の試験：手順書だけで実機確認ができた）

- 前の会話を知らない Claude に repo だけを渡し、「iPhone の DevKit が最新ビルドか」を PC から確かめさせた
  - 結果：latest は build-3（e1e0f66）。DEVKIT の commit=e1e0f66、画面の build: e1e0f66 で一致（19:23）。iPhone には何も入れていない
- 出てきた手順書の穴を直した（OPERATIONS.md）：build-N のコミットの調べ方、比べる相手、エクスプローラーの開き方と前に出す方法、台本が最新かの確かめ方、終わった印とかかる時間、よく使う plan の形、結果に入る ID・端末名を写すときは伏せる、時計のずれ
- ios.yml：scripts/pc などの変更では DevKit をビルドしないようにした（paths を scripts/*.sh に）。比べるとき紛らわしいため

## 2026-11-24（PC の新しい台本で 1 周＋DevKit を人の手なしで LiveContainer に入れた）

- scripts/pc の go.bat ＋ plan.json を PC に置いて実行（設定 config.ps1 は devkit-setup に置く）
  - 19:09 スワイプ 4 回とも DEVKIT のログに出た（commit=3dd056a の旧 DevKit）
  - 直したこと：WDA の失敗の理由を記録に出す／WDA に送る本文を UTF-8 にする（PowerShell 5.1 だと日本語が化けて「実行」を探せなかった）／go-ios をいつもの場所で動かす（鍵 selfIdentity.plist が devkit-setup にできてしまったため）
  - 足した操作：terminate、tapText（文字で押す）、tree / pull / push / pushdir（アプリの中のファイル）
- DevKit を PC から LiveContainer に直接入れた（Safari の手作業なし）
  - 控え：LiveContainer の /Documents/Applications/io.github.iwataka793.devkit.app を取り出した（中身は DevKit・Info.plist・PkgInfo・LCAppIconLight.png・LCAppInfo.plist。署名は実行ファイルの中だけ）
  - build-3（commit e1e0f66）の中身 ＋ LCPatchRevision を消した LCAppInfo.plist を pushdir で上書き → LiveContainer を起動し直して「実行」を tapText
  - 19:19 画面下が build: e1e0f66、スワイプが DEVKIT swipe=right/left commit=e1e0f66 で記録に出た → LiveContainer が自分でパッチと署名をし直して動いた
  - 道具：tools/lc-install/prepare.py、手順は OPERATIONS.md 3-5
  - 失敗した点：一度、一覧の並びが変わって config.ps1 をダブルクリックした（開くだけで実行はされない）。押す前の確認を別の呼び出しに分けるよう手順に書いた

## 2026-11-24（引き継ぎの準備：手順書・CLAUDE.md・PC の台本を repo に）

- docs/OPERATIONS.md：Claude 向けの手順書（開発の 1 周、PC の使い方、go-ios のコツ、WDA、SideStore の注意、調べ方の決まり、未検証のこと）
- CLAUDE.md：動き方の決まりだけ。手順書と LOG を指す（カオルの了承済み）
- scripts/pc/：PC の台本を一本化。go.bat + plan.json（計画ファイル）で「ログ・画面・WDA 操作」を一度に。ID は PC の %USERPROFILE%\devkit\config.ps1（config.example.ps1 が見本）
  - 未検証：PC にはまだ置いていない（PC がつながっていない）。次に PC がつながったら置いて試す
- wda.yml：SideStore 向けの WebDriverAgentRunner-ss.ipa も Releases に置くようにした

## 2026-11-24（片付け：PC 方式の自動更新をやめて SideStore に一本化）

- 確認：SideStore の My Apps に WebDriverAgentRunner-Runner が載っている（残り 7 日）。18:31 にもう一度 PC から起動して、DevKit の上下左右スワイプ 4 回とも記録に出た
- PC：スケジューラーのタスク「iPhone WebDriverAgent 自動更新」を削除。使わなくなった台本（1〜16）・wda-refresh.exe・古い WDA の IPA 2 つをごみ箱へ（18-cleanup。記録の txt / png / フォルダは残した）
  - PC に残した実行道具：17-ss（WDA を `--xctestconfig=WebDriverAgentRunner` で起動して操作）、WebDriverAgentRunner-ss.ipa（SideStore で入れ直す用）
- repo：tools/wda-refresh・tools/sign-compare とそれぞれの Actions を削除（履歴には残る）。Releases（tools-1〜4、diag-1、wda-1〜3）はこのセッションの権限では消せないので残っている
- これからの WDA の更新：SideStore の 7 日更新に任せる。WDA を作り直したときは tools/wda-sidestore/make_ss_ipa.py で SideStore 向けにしてから入れる

## 2026-11-24（SideStore で入れた WebDriverAgent が動いた）

- tools/wda-sidestore/make_ss_ipa.py で作った WebDriverAgentRunner-ss.ipa（PlugIns/WebDriverAgentRunner.xctest → PlugIns/WebDriverAgentRunner、テスト本体の ID を本体と同じに）を SideStore で入れた
- PC（17-ss）から `ios runwda --xctestconfig=WebDriverAgentRunner` で起動 → 5 秒で /status OK
  - セッション作成 → LiveContainer を前に → 下・上・右・左のスワイプ → DevKit の記録に 4 行とも出た（18:27:13〜23）
- 分かったこと：テスト実行係は「.xctest」で終わらない名前のテスト本体でも受け付ける。SideStore の署名（AltSign + ldid）でも中身はちゃんと動く
- これで WebDriverAgent も SideStore の 7 日更新に乗せられる（PC の iloader 方式の自動更新と同じ ID なので、両方動かすと上書きし合う）

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
