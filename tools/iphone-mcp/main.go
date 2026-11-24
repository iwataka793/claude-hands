// iphone-mcp：Claude が iPhone を見て・動かすための MCP サーバー（Windows の Claude デスクトップの拡張機能として動く）。
//
// 中で go-ios を使い、トンネル・ログ・WebDriverAgent をつないだまま持つ。
// 3 段のスイッチ（オフ・見るだけ・操作も可）はカオルだけが切り替える（mode.txt。この中には切り替える道具を作らない）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var version = "dev"

type app struct {
	sw        *Switch
	phone     *Phone
	stateDir  string
	idleLimit time.Duration
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" && !strings.HasPrefix(v, "${") {
		return v
	}
	return def
}

func main() {
	// 置き場所は %USERPROFILE%\devkit\iphone-mcp。
	// %LOCALAPPDATA% にすると、ストア版の Claude から動いたときに書き込みが
	// AppData\Local\Packages\Claude_…\LocalCache に振り分けられ、スイッチの .bat と別のファイルを見てしまう（2026-11-24 に実物で確認）。
	stateDir := env("IPHONE_MCP_STATE_DIR", filepath.Join(os.Getenv("USERPROFILE"), "devkit", "iphone-mcp"))
	_ = os.MkdirAll(stateDir, 0o755)
	logf, _ := os.OpenFile(filepath.Join(stateDir, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if logf != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(logf, nil)))
	}
	// トンネルの鍵：前から使っている go-ios の場所があればそれを使う（iPhone 側の信頼をやり直さないため）
	pairDir := env("IPHONE_MCP_PAIR_DIR", "")
	if pairDir == "" {
		old := filepath.Join(os.Getenv("USERPROFILE"), "devkit", "go-ios")
		if _, err := os.Stat(filepath.Join(old, "selfIdentity.plist")); err == nil {
			pairDir = old
		} else {
			pairDir = filepath.Join(stateDir, "pair")
		}
	}
	_ = os.MkdirAll(pairDir, 0o755)
	idleMin, _ := strconv.Atoi(env("IPHONE_MCP_IDLE_MINUTES", "30"))
	if idleMin <= 0 {
		idleMin = 30
	}
	a := &app{
		sw:        NewSwitch(stateDir, time.Duration(idleMin)*time.Minute),
		phone:     NewPhone(pairDir, env("IPHONE_MCP_WDA_TEST", "WebDriverAgentRunner"), filepath.Join(stateDir, "logs")),
		stateDir:  stateDir,
		idleLimit: time.Duration(idleMin) * time.Minute,
	}
	slog.Info("start", "version", version, "state", stateDir, "pair", pairDir)

	// 見張り：スイッチがオフになった／しばらく使われていない → 全部切る
	go func() {
		for range time.Tick(15 * time.Second) {
			if a.sw.IdleExpired() || a.sw.Current() == ModeOff {
				if a.phone.Connected() {
					a.phone.Disconnect()
				}
			}
		}
	}()

	s := mcp.NewServer(&mcp.Implementation{Name: "iphone", Version: version}, nil)
	a.register(s)
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		slog.Error("server", "error", err)
	}
	a.phone.Disconnect()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func fail(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}
}

// gate はスイッチを確かめる。need = ModeView（見るだけで使える）か ModeOperate（操作）。
func (a *app) gate(need Mode) *mcp.CallToolResult {
	m := a.sw.Current()
	if m == ModeOff {
		return fail("スイッチがオフです。カオルに「見るだけ」か「操作も可」にしてもらってください（切り替えはカオルだけ。%s）", a.sw.Path())
	}
	if need == ModeOperate && m != ModeOperate {
		return fail("スイッチが「見るだけ」なので操作できません。操作が要るならカオルに「操作も可」にしてもらってください")
	}
	a.sw.Touch()
	return nil
}

func (a *app) lc(bundle string) string {
	if bundle == "" || strings.EqualFold(bundle, "livecontainer") {
		return a.phone.LiveContainer()
	}
	return bundle
}

type noInput struct{}

type logsIn struct {
	Match        string `json:"match" jsonschema:"iPhone のログから抜き出す正規表現。例 DEVKIT"`
	SinceSeconds int    `json:"since_seconds,omitempty" jsonschema:"何秒前からのログを見るか（既定 60）"`
	WaitSeconds  int    `json:"wait_seconds,omitempty" jsonschema:"合う行がまだ無ければ、この秒数まで待つ（既定 0）"`
	MaxLines     int    `json:"max_lines,omitempty" jsonschema:"返す行数の上限（既定 100、新しい方を残す）"`
}

type screenshotIn struct {
	Full bool `json:"full,omitempty" jsonschema:"true なら縮めない元の大きさ（既定は 1/3 に縮めた JPEG）"`
}

type elementsIn struct {
	All bool `json:"all,omitempty" jsonschema:"true なら文字の無い部品も含める"`
	Max int  `json:"max,omitempty" jsonschema:"行数の上限（既定 200）"`
}

// 操作の道具に共通：操作のあとの様子の返し方
// （jsonschema の都合で埋め込みにせず、それぞれの型に同じ 2 つを書く）

type tapIn struct {
	X     float64 `json:"x" jsonschema:"横の位置（WDA の単位。iPhone 16e は幅 390）"`
	Y     float64 `json:"y" jsonschema:"縦の位置（高さ 844）"`
	Count int     `json:"count,omitempty" jsonschema:"2 ならダブルタップ"`
	Hold  float64 `json:"hold,omitempty" jsonschema:"長押しの秒数（0 なら普通のタップ）"`
	Show  string  `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（変わったときだけ・既定）／always／never"`
	Log   string  `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す（例 DEVKIT）"`
}

type swipeIn struct {
	FromX    float64 `json:"from_x"`
	FromY    float64 `json:"from_y"`
	ToX      float64 `json:"to_x"`
	ToY      float64 `json:"to_y"`
	Duration float64 `json:"duration,omitempty" jsonschema:"秒（既定 0.2）"`
	Show     string  `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
	Log      string  `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す"`
}

type tapTextIn struct {
	Text    string  `json:"text" jsonschema:"押したい部品の文字（label / name / value / placeholderValue のどれかと一致）"`
	Index   int     `json:"index,omitempty" jsonschema:"同じ文字が複数あるときの番号（0 から）"`
	Partial bool    `json:"partial,omitempty" jsonschema:"true なら文字の一部が合えばよい"`
	Count   int     `json:"count,omitempty" jsonschema:"2 ならダブルタップ"`
	Hold    float64 `json:"hold,omitempty" jsonschema:"長押しの秒数"`
	Show    string  `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
	Log     string  `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す"`
}

type typeIn struct {
	Text    string `json:"text" jsonschema:"打つ文字"`
	Field   string `json:"field,omitempty" jsonschema:"入力欄の文字（ラベルや薄く出ている案内文）。指定するとその欄を押してから打つ。省略すると今選ばれている欄に打つ"`
	Partial bool   `json:"partial,omitempty" jsonschema:"true なら field の一部が合えばよい"`
	Clear   bool   `json:"clear,omitempty" jsonschema:"true なら欄の中身を消してから打つ（field が要る）"`
	Enter   bool   `json:"enter,omitempty" jsonschema:"true なら最後に改行（決定・検索）を押す"`
	Show    string `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
	Log     string `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す"`
}

type buttonIn struct {
	Name string `json:"name" jsonschema:"home / volumeup / volumedown / lock / unlock / hide_keyboard（キーボードを閉じる）/ notifications（通知センター）/ control_center / back（画面の左端から戻る）"`
	Show string `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
	Log  string `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す"`
}

type pinchIn struct {
	Scale    float64 `json:"scale" jsonschema:"2 なら 2 倍に拡大、0.5 なら縮小"`
	Velocity float64 `json:"velocity,omitempty" jsonschema:"速さ（省略で拡大 1・縮小 -1）"`
	Show     string  `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
}

type alertIn struct {
	Action string `json:"action,omitempty" jsonschema:"read（文とボタンを読む・既定）／accept（許可・OK 側）／dismiss（キャンセル側）"`
	Button string `json:"button,omitempty" jsonschema:"accept / dismiss のとき、押すボタンの文字（例「Appの使用中は許可」）"`
	Show   string `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
}

type waitIn struct {
	Text    string  `json:"text" jsonschema:"待つ文字（画面の部品の label / name / value）"`
	Partial bool    `json:"partial,omitempty" jsonschema:"true なら一部が合えばよい"`
	Gone    bool    `json:"gone,omitempty" jsonschema:"true なら消えるまで待つ（「読み込み中」など）"`
	Seconds float64 `json:"seconds,omitempty" jsonschema:"最大何秒待つか（既定 10）"`
	Show    string  `json:"show,omitempty" jsonschema:"待ったあとの画面を付けるか：changed（既定）／always／never"`
}

type scrollToIn struct {
	Text      string `json:"text" jsonschema:"出したい部品の文字"`
	Partial   bool   `json:"partial,omitempty" jsonschema:"true なら一部が合えばよい"`
	Direction string `json:"direction,omitempty" jsonschema:"どちらの中身を見に行くか：down（既定。下の方）／up／left／right"`
	Max       int    `json:"max,omitempty" jsonschema:"最大何回スクロールするか（既定 10）"`
	Tap       bool   `json:"tap,omitempty" jsonschema:"true なら見つけたら押す"`
	Show      string `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
}

type appIn struct {
	Action string `json:"action" jsonschema:"activate（前に出す）／launch（起動し直す）／terminate（終わらせる）／state（動いているか）／url（URL を開く）"`
	Bundle string `json:"bundle,omitempty" jsonschema:"アプリの名前（ホーム画面の名前。標準アプリは「設定」など日本語でよい）か bundle id。省略か livecontainer なら LiveContainer"`
	URL    string `json:"url,omitempty" jsonschema:"開く URL（action が url のとき。https:// や アプリの URL スキーム）"`
	Show   string `json:"show,omitempty" jsonschema:"操作のあとの画面を付けるか：changed（既定）／always／never"`
	Log    string `json:"log,omitempty" jsonschema:"操作のあとに出たログのうち、この正規表現に合う行も返す"`
}

type stepsIn struct {
	Steps []map[string]any `json:"steps" jsonschema:"上から順に行う操作。1 つは {\"do\": 種類, ...引数}。種類と引数は同じ名前の道具と同じ：tap{x,y,count,hold} / swipe{from_x,from_y,to_x,to_y,duration} / tap_text{text,index,partial,count,hold} / type{text,field,partial,clear,enter} / button{name} / pinch{scale} / alert{action,button} / wait{text,partial,gone,seconds} / scroll_to{text,partial,direction,max,tap} / app{action,bundle,url} / sleep{seconds}"`
	Pause float64          `json:"pause,omitempty" jsonschema:"操作と操作のあいだに待つ秒数（既定 0.3）"`
	Show  string           `json:"show,omitempty" jsonschema:"最後の画面を付けるか：always（既定）／changed／never"`
	Log   string           `json:"log,omitempty" jsonschema:"始めてから出たログのうち、この正規表現に合う行も返す"`
}

type lcInstallIn struct {
	Tag   string `json:"tag,omitempty" jsonschema:"iphone-dev-kit の Releases のタグ（例 build-5）。省略で最新"`
	Asset string `json:"asset,omitempty" jsonschema:"Releases の中のファイル名（既定 DevKit.ipa）"`
}

type infoIn struct {
	What     string `json:"what,omitempty" jsonschema:"summary（機種・iOS・ロック・容量・電池・言語など。既定）／processes（動いているアプリ）／apps（入れたアプリと大きさ）／crashes（落ちた記録）"`
	Match    string `json:"match,omitempty" jsonschema:"名前を絞る正規表現（processes・apps・crashes）"`
	All      bool   `json:"all,omitempty" jsonschema:"processes で、アプリ以外のプログラムも出す"`
	File     string `json:"file,omitempty" jsonschema:"crashes で、この名前の記録の中身を読む"`
	Max      int    `json:"max,omitempty" jsonschema:"行数の上限（既定 50）"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"記録の中身を何文字まで返すか（既定 12000）"`
}

type filesIn struct {
	Action    string `json:"action,omitempty" jsonschema:"list（一覧・既定）／read（文字のファイルを読む）／pull（PC に取る）／write（文字を書く）／push（PC のファイルを置く）。write・push は操作も可のときだけ"`
	App       string `json:"app,omitempty" jsonschema:"どのアプリの中か：名前か bundle id（省くと LiveContainer。開けるのは開発者の署名のアプリだけ）。media なら写真などのフォルダ"`
	Path      string `json:"path,omitempty" jsonschema:"iPhone の中の場所。例 /Documents"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"list で、下のフォルダも（5 段まで）"`
	Text      string `json:"text,omitempty" jsonschema:"write で書く文字"`
	Local     string `json:"local,omitempty" jsonschema:"pull・push の PC の場所（ユーザーフォルダの中）。pull で省くと %USERPROFILE%\\devkit\\iphone-mcp\\files"`
	Max       int    `json:"max,omitempty" jsonschema:"list の行数の上限（既定 300）"`
	MaxChars  int    `json:"max_chars,omitempty" jsonschema:"read で何文字まで返すか（既定 20000）"`
}

type clipboardIn struct {
	Action string `json:"action" jsonschema:"get（読む）／set（文字を入れる）"`
	Text   string `json:"text,omitempty" jsonschema:"set で入れる文字"`
}

type locationIn struct {
	Action string  `json:"action" jsonschema:"set（仮の位置にする）／reset（元に戻す）"`
	Lat    float64 `json:"lat,omitempty" jsonschema:"緯度（set のとき）"`
	Lon    float64 `json:"lon,omitempty" jsonschema:"経度（set のとき）"`
}

func (a *app) register(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{Name: "iphone_status", Description: "スイッチ（オフ・見るだけ・操作も可）とつながり具合を見る。iPhone にはつながない"},
		func(ctx context.Context, req *mcp.CallToolRequest, in noInput) (*mcp.CallToolResult, any, error) {
			st := a.phone.Status()
			st["スイッチ"] = a.sw.Current().String()
			st["自動オフまで（使わない時間）"] = a.idleLimit.String()
			st["版"] = version
			b, _ := json.MarshalIndent(st, "", "  ")
			return text(string(b)), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_logs", Description: "iPhone のログ（システムログ）から正規表現に合う行を返す。つないだ時からずっと覚えている。wait_seconds で出るまで待てる"},
		func(ctx context.Context, req *mcp.CallToolRequest, in logsIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			if in.SinceSeconds <= 0 {
				in.SinceSeconds = 60
			}
			if in.MaxLines <= 0 {
				in.MaxLines = 100
			}
			lines, err := a.phone.Logs(ctx, in.Match, in.SinceSeconds, in.WaitSeconds, in.MaxLines)
			if err != nil {
				return fail("%v", err), nil, nil
			}
			if len(lines) == 0 {
				return text(fmt.Sprintf("「%s」に合う行はありません（%d 秒前から）", in.Match, in.SinceSeconds)), nil, nil
			}
			return text(strings.Join(lines, "\n")), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_screenshot", Description: "iPhone の画面を撮って画像で返す（既定は 1/3 に縮めた JPEG。座標は WDA の単位とほぼ同じ 390x844）"},
		func(ctx context.Context, req *mcp.CallToolRequest, in screenshotIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			png, err := a.shotPNG(ctx)
			if err != nil {
				return fail("%v", err), nil, nil
			}
			if in.Full {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: png, MIMEType: "image/png"}}}, nil, nil
			}
			jpg, w, h, err := shrink(png, 3, 80)
			if err != nil {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: png, MIMEType: "image/png"}}}, nil, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{
				&mcp.ImageContent{Data: jpg, MIMEType: "image/jpeg"},
				&mcp.TextContent{Text: fmt.Sprintf("%dx%d に縮めた画面（%s）", w, h, time.Now().Format("15:04:05"))},
			}}, nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_elements", Description: "今の画面の部品（ボタン・文字など）を「種類 「文字」 中心(x,y)」の行で返す。押す場所を決めるのに使う"},
		func(ctx context.Context, req *mcp.CallToolRequest, in elementsIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			b, err := a.phone.WDA(ctx, "GET", "/session/{s}/source", nil)
			if err != nil {
				return fail("%v", err), nil, nil
			}
			var v struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(b, &v)
			if in.Max <= 0 {
				in.Max = 200
			}
			out, total := compactSource(v.Value, in.All, in.Max)
			return text(fmt.Sprintf("部品 %d 個のうち：\n%s", total, out)), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_tap", Description: "画面の (x, y) をタップする。count=2 でダブルタップ、hold で長押し。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in tapIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) { return a.opTap(ctx, in.X, in.Y, in.Count, in.Hold) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_swipe", Description: "画面をスワイプ（ドラッグ）する。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in swipeIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) { return a.opSwipe(ctx, in.FromX, in.FromY, in.ToX, in.ToY, in.Duration) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_tap_text", Description: "画面の部品を文字で探して押す（例：LiveContainer の「実行」）。count=2 でダブルタップ、hold で長押し。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in tapTextIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) {
				return a.opTapText(ctx, in.Text, in.Index, in.Partial, in.Count, in.Hold)
			}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_type", Description: "文字を打つ。field で入力欄を指定するとその欄を押してから打つ（clear で消してから、enter で最後に改行）。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in typeIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) {
				return a.opType(ctx, in.Text, in.Field, in.Partial, in.Clear, in.Enter)
			}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_button", Description: "ボタンや決まった動き：home（ホームに戻る）/ volumeup / volumedown / lock / unlock（パスコードが要るときは外せない）/ hide_keyboard / notifications / control_center / back。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in buttonIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) { return a.opButton(ctx, in.Name) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_pinch", Description: "前のアプリの画面をピンチで拡大・縮小する。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in pinchIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show}, func() (string, error) { return a.opPinch(ctx, in.Scale, in.Velocity) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_alert", Description: "ポップアップ（「許可しますか？」など）を読む（read・見るだけで使える）か、ボタンを押す（accept / dismiss・操作も可のときだけ。button で押す文字を選べる）"},
		func(ctx context.Context, req *mcp.CallToolRequest, in alertIn) (*mcp.CallToolResult, any, error) {
			if in.Action == "" || in.Action == "read" {
				if r := a.gate(ModeView); r != nil {
					return r, nil, nil
				}
				m, err := a.opAlert(ctx, "read", "")
				if err != nil {
					return fail("%v", err), nil, nil
				}
				return text(m), nil, nil
			}
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show}, func() (string, error) { return a.opAlert(ctx, in.Action, in.Button) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_wait", Description: "画面に文字が出るまで（gone なら消えるまで）待つ。読み込みの終わりを待つのに使う。見るだけで使える"},
		func(ctx context.Context, req *mcp.CallToolRequest, in waitIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			start := time.Now()
			m, err := a.opWait(ctx, in.Text, in.Partial, in.Gone, in.Seconds)
			if err != nil {
				return a.after(ctx, afterOpts{Show: "always", Settle: 0.5}, start, nil, err.Error(), true), nil, nil
			}
			return a.after(ctx, afterOpts{Show: in.Show, Settle: 1}, start, nil, m, false), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_scroll_to", Description: "文字が画面に出るまでスクロールする（tap なら見つけて押す）。端まで来たら止まる。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in scrollToIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show}, func() (string, error) {
				return a.opScrollTo(ctx, in.Text, in.Partial, in.Direction, in.Max, in.Tap)
			}), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_app", Description: "アプリを前に出す（activate）・起動し直す（launch）・終わらせる（terminate）・様子を見る（state）・URL を開く（url）。bundle はホーム画面の名前か bundle id（省くと LiveContainer）。操作のあと、画面が落ち着くまで待ち、変わったか・前のアプリ・（log を指定すれば）合うログを返す。画面が変わったら縮めた画面も付く（show で変えられる）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in appIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if in.Action == "state" {
				m, err := a.opApp(ctx, "state", in.Bundle, "")
				if err != nil {
					return fail("%v", err), nil, nil
				}
				return text(m), nil, nil
			}
			return a.act(ctx, afterOpts{Show: in.Show, Log: in.Log}, func() (string, error) { return a.opApp(ctx, in.Action, in.Bundle, in.URL) }), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_steps", Description: "いくつかの操作を上から順に 1 回で行う（例：アプリを開く → 欄に打つ → ボタンを押す → 文字が出るまで待つ）。途中で失敗したらそこで止め、何番目で何が起きたかと、その時の画面を返す。最後に画面（既定）と、log を指定すれば合うログを返す。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in stepsIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			return a.runSteps(ctx, in), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_lc_install", Description: "GitHub の iphone-dev-kit の Releases から IPA を取り、LiveContainer の中のアプリを上書きする（前の中身は PC に控え）。LiveContainer を終わらせてから入れる。入れた後は iphone_app activate → iphone_tap_text「実行」で起動。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in lcInstallIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			lc := a.phone.LiveContainer()
			if lc == "" {
				return fail("LiveContainer が見つかりません"), nil, nil
			}
			if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/apps/terminate", map[string]any{"bundleId": lc}); err != nil {
				return fail("LiveContainer を終わらせられません：%v", err), nil, nil
			}
			msg, err := a.phone.LCInstall(ctx, in.Tag, in.Asset, filepath.Join(a.stateDir, "backup"))
			if err != nil {
				return fail("%v", err), nil, nil
			}
			return text(msg), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_info", Description: "iPhone の中の様子を、画面を使わずに見る：summary（機種・iOS・ロック・容量・電池・言語など。既定）／processes（動いているアプリ）／apps（入れたアプリと大きさ）／crashes（落ちた記録の一覧。file で中身）。見るだけで使える"},
		func(ctx context.Context, req *mcp.CallToolRequest, in infoIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			if in.Max <= 0 {
				in.Max = 50
			}
			if in.MaxChars <= 0 {
				in.MaxChars = 12000
			}
			var m string
			var err error
			switch in.What {
			case "", "summary":
				m = a.phone.InfoSummary()
			case "processes":
				m, err = a.phone.Processes(in.Match, in.All, in.Max)
			case "apps":
				m, err = a.phone.AppSizes(in.Match, in.Max)
			case "crashes":
				m, err = a.phone.Crashes(in.Match, in.File, in.Max, in.MaxChars)
			default:
				return fail("what は summary / processes / apps / crashes"), nil, nil
			}
			if err != nil {
				return fail("%v", err), nil, nil
			}
			return text(m), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_files", Description: "アプリの中のファイルを、画面を使わずに見る・読む・PC に取る（list / read / pull）、書く・置く（write / push・操作も可のときだけ）。開けるのは開発者の署名のアプリ（LiveContainer など。省くと LiveContainer）と、app=media の写真などのフォルダ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in filesIn) (*mcp.CallToolResult, any, error) {
			need := ModeView
			if in.Action == "write" || in.Action == "push" {
				need = ModeOperate
			}
			if r := a.gate(need); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			bundle := "media"
			if !strings.EqualFold(in.App, "media") {
				b, err := a.resolveApp(ctx, in.App)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				bundle = b
			}
			if in.Path == "" && in.Action != "" && in.Action != "list" {
				return fail("path（iPhone の中の場所）を指定してください"), nil, nil
			}
			switch in.Action {
			case "", "list":
				if in.Max <= 0 {
					in.Max = 300
				}
				m, err := a.phone.FilesList(bundle, in.Path, in.Recursive, in.Max)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				return text(m), nil, nil
			case "read":
				if in.MaxChars <= 0 {
					in.MaxChars = 20000
				}
				m, err := a.phone.FilesRead(bundle, in.Path, in.MaxChars)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				return text(m), nil, nil
			case "pull":
				dst, err := pcPath(a.stateDir, in.Local, in.Path)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				c, _, err := a.phone.filesClient(bundle)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				defer c.Close()
				b, err := afcRead(c, in.Path)
				if err != nil {
					return fail("%s を読めません：%v", in.Path, err), nil, nil
				}
				_ = os.MkdirAll(filepath.Dir(dst), 0o755)
				if err := os.WriteFile(dst, b, 0o644); err != nil {
					return fail("PC に書けません：%v", err), nil, nil
				}
				return text(fmt.Sprintf("PC に取った：%s（%s）", dst, size(int64(len(b))))), nil, nil
			case "write":
				if err := a.phone.FilesWrite(bundle, in.Path, []byte(in.Text)); err != nil {
					return fail("書けません：%v", err), nil, nil
				}
				return text(fmt.Sprintf("%s に書いた（%d 文字）", in.Path, len([]rune(in.Text)))), nil, nil
			case "push":
				if in.Local == "" {
					return fail("local（PC のファイル）を指定してください"), nil, nil
				}
				src, err := pcPath(a.stateDir, in.Local, in.Path)
				if err != nil {
					return fail("%v", err), nil, nil
				}
				b, err := os.ReadFile(src)
				if err != nil {
					return fail("PC のファイルを読めません：%v", err), nil, nil
				}
				if err := a.phone.FilesWrite(bundle, in.Path, b); err != nil {
					return fail("置けません：%v", err), nil, nil
				}
				return text(fmt.Sprintf("%s を %s に置いた（%s）", src, in.Path, size(int64(len(b))))), nil, nil
			}
			return fail("action は list / read / pull / write / push"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_clipboard", Description: "iPhone のクリップボードを読む（get）・文字を入れる（set。長い文を打つ代わりに入れて貼る）。中にパスワードなどが入っていることがあるので、読むのは要るときだけ。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in clipboardIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			switch in.Action {
			case "get":
				t, ok, err := a.phone.ClipboardGet()
				if err != nil {
					return fail("クリップボードを読めません：%v", err), nil, nil
				}
				if !ok {
					return text("クリップボードに文字はありません"), nil, nil
				}
				return text(t), nil, nil
			case "set":
				if err := a.phone.ClipboardSet(in.Text); err != nil {
					return fail("クリップボードに入れられません：%v", err), nil, nil
				}
				return text(fmt.Sprintf("クリップボードに入れた（%d 文字）", len([]rune(in.Text)))), nil, nil
			}
			return fail("action は get / set"), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_location", Description: "開発用に iPhone の位置を仮の場所にする（set・緯度経度）／元に戻す（reset）。地図や位置を使うアプリを試すとき用。使い終わったら必ず reset。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in locationIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			switch in.Action {
			case "set":
				if in.Lat == 0 && in.Lon == 0 {
					return fail("lat と lon を指定してください"), nil, nil
				}
				if err := a.phone.Location(false, in.Lat, in.Lon); err != nil {
					return fail("%v", err), nil, nil
				}
				return text(fmt.Sprintf("仮の位置にした（%.5f, %.5f）。戻すときは action=reset", in.Lat, in.Lon)), nil, nil
			case "reset":
				if err := a.phone.Location(true, 0, 0); err != nil {
					return fail("%v", err), nil, nil
				}
				return text("本当の位置に戻した"), nil, nil
			}
			return fail("action は set / reset"), nil, nil
		})
}
