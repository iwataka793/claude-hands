// iphone-mcp：Claude が iPhone を見て・動かすための MCP サーバー（Windows の Claude デスクトップの拡張機能として動く）。
//
// 中で go-ios を使い、トンネル・ログ・WebDriverAgent をつないだまま持つ。
// 3 段のスイッチ（オフ・見るだけ・操作も可）はカオルだけが切り替える（mode.txt。この中には切り替える道具を作らない）。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danielpaulus/go-ios/ios/instruments"
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
		phone:     NewPhone(pairDir, env("IPHONE_MCP_WDA_TEST", "WebDriverAgentRunner")),
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

type tapIn struct {
	X float64 `json:"x" jsonschema:"横の位置（WDA の単位。iPhone 16e は幅 390）"`
	Y float64 `json:"y" jsonschema:"縦の位置（高さ 844）"`
}

type swipeIn struct {
	FromX    float64 `json:"from_x"`
	FromY    float64 `json:"from_y"`
	ToX      float64 `json:"to_x"`
	ToY      float64 `json:"to_y"`
	Duration float64 `json:"duration,omitempty" jsonschema:"秒（既定 0.2）"`
}

type tapTextIn struct {
	Text  string `json:"text" jsonschema:"押したい部品の文字（label / name / value のどれかと一致）"`
	Index int    `json:"index,omitempty" jsonschema:"同じ文字が複数あるときの番号（0 から）"`
}

type appIn struct {
	Action string `json:"action" jsonschema:"activate（前に出す）か terminate（終わらせる）"`
	Bundle string `json:"bundle,omitempty" jsonschema:"bundle id。省略か livecontainer なら LiveContainer"`
}

type lcInstallIn struct {
	Tag   string `json:"tag,omitempty" jsonschema:"iphone-dev-kit の Releases のタグ（例 build-5）。省略で最新"`
	Asset string `json:"asset,omitempty" jsonschema:"Releases の中のファイル名（既定 DevKit.ipa）"`
}

type lcListIn struct {
	Path string `json:"path" jsonschema:"LiveContainer の中のフォルダ。例 /Documents/Applications"`
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
			var png []byte
			if b, err := a.phone.WDA(ctx, "GET", "/screenshot", nil); err == nil {
				var v struct {
					Value string `json:"value"`
				}
				if json.Unmarshal(b, &v) == nil {
					png, _ = base64.StdEncoding.DecodeString(v.Value)
				}
			}
			if len(png) == 0 {
				if err := a.phone.Ensure(ctx); err != nil {
					return fail("%v", err), nil, nil
				}
				svc, err := instruments.NewScreenshotService(a.phone.rsd)
				if err != nil {
					return fail("画面を撮れません：%v", err), nil, nil
				}
				png, err = svc.TakeScreenshot()
				svc.Close()
				if err != nil {
					return fail("画面を撮れません：%v", err), nil, nil
				}
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

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_tap", Description: "画面の (x, y) をタップする（操作も可のときだけ）"},
		func(ctx context.Context, req *mcp.CallToolRequest, in tapIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/tap", map[string]any{"x": in.X, "y": in.Y}); err != nil {
				return fail("%v", err), nil, nil
			}
			return text(fmt.Sprintf("タップ (%v,%v) %s", in.X, in.Y, time.Now().Format("15:04:05.000"))), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_swipe", Description: "画面をスワイプ（ドラッグ）する（操作も可のときだけ）"},
		func(ctx context.Context, req *mcp.CallToolRequest, in swipeIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if in.Duration <= 0 {
				in.Duration = 0.2
			}
			if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/dragfromtoforduration",
				map[string]any{"fromX": in.FromX, "fromY": in.FromY, "toX": in.ToX, "toY": in.ToY, "duration": in.Duration}); err != nil {
				return fail("%v", err), nil, nil
			}
			return text(fmt.Sprintf("スワイプ (%v,%v)→(%v,%v) %s", in.FromX, in.FromY, in.ToX, in.ToY, time.Now().Format("15:04:05.000"))), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_tap_text", Description: "画面の部品を文字で探して押す（例：LiveContainer の「実行」）。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in tapTextIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			q := strings.ReplaceAll(in.Text, "'", "\\'")
			b, err := a.phone.WDA(ctx, "POST", "/session/{s}/elements",
				map[string]any{"using": "predicate string", "value": fmt.Sprintf("label == '%s' OR name == '%s' OR value == '%s'", q, q, q)})
			if err != nil {
				return fail("%v", err), nil, nil
			}
			var v struct {
				Value []map[string]string `json:"value"`
			}
			_ = json.Unmarshal(b, &v)
			var ids []string
			for _, e := range v.Value {
				id := e["ELEMENT"]
				if id == "" {
					id = e["element-6066-11e4-a52e-4f735466cecf"]
				}
				ids = append(ids, id)
			}
			if len(ids) <= in.Index {
				return fail("「%s」が見つかりません（%d 個）。iphone_elements で画面の部品を確かめてください", in.Text, len(ids)), nil, nil
			}
			if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+ids[in.Index]+"/click", map[string]any{}); err != nil {
				return fail("%v", err), nil, nil
			}
			return text(fmt.Sprintf("「%s」を押した（%d 個中 %d 番）%s", in.Text, len(ids), in.Index, time.Now().Format("15:04:05.000"))), nil, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_app", Description: "アプリを前に出す（activate）か終わらせる（terminate）。bundle を省くと LiveContainer。操作も可のときだけ"},
		func(ctx context.Context, req *mcp.CallToolRequest, in appIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeOperate); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			b := a.lc(in.Bundle)
			if b == "" {
				return fail("LiveContainer が見つかりません"), nil, nil
			}
			switch in.Action {
			case "activate", "terminate":
			default:
				return fail("action は activate か terminate"), nil, nil
			}
			r, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/apps/"+in.Action, map[string]any{"bundleId": b})
			if err != nil {
				return fail("%v", err), nil, nil
			}
			return text(fmt.Sprintf("%s %s → %s", in.Action, b, strings.TrimSpace(string(r)))), nil, nil
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

	mcp.AddTool(s, &mcp.Tool{Name: "iphone_lc_list", Description: "LiveContainer の中のフォルダの一覧（1 段）。見るだけで使える"},
		func(ctx context.Context, req *mcp.CallToolRequest, in lcListIn) (*mcp.CallToolResult, any, error) {
			if r := a.gate(ModeView); r != nil {
				return r, nil, nil
			}
			if err := a.phone.Ensure(ctx); err != nil {
				return fail("%v", err), nil, nil
			}
			names, err := a.phone.LCList(in.Path)
			if err != nil {
				return fail("%v", err), nil, nil
			}
			return text(strings.Join(names, "\n")), nil, nil
		})
}
