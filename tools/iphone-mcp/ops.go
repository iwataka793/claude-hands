package main

// 操作の中身（道具 1 つずつと、まとめて実行 iphone_steps の両方から使う）。
// 操作のあとは「画面が落ち着くまで待つ → 変わったか → 前のアプリ → 合うログ」を返す（after）。

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- 画面 ----

// shotPNG は画面を PNG で撮る。WDA が動いていれば WDA、動いていなければ instruments
// （画面を撮るためだけに WDA を起こさない。instruments は 1 秒かからない）。
func (a *app) shotPNG(ctx context.Context) ([]byte, error) {
	if a.phone.WDAReady() {
		if b, err := a.phone.WDA(ctx, "GET", "/screenshot", nil); err == nil {
			var v struct {
				Value string `json:"value"`
			}
			if json.Unmarshal(b, &v) == nil {
				if png, err := base64.StdEncoding.DecodeString(v.Value); err == nil && len(png) > 0 {
					return png, nil
				}
			}
		}
	}
	if err := a.phone.Ensure(ctx); err != nil {
		return nil, err
	}
	svc, err := instruments.NewScreenshotService(a.phone.rsd)
	if err != nil {
		return nil, fmt.Errorf("画面を撮れません：%v", err)
	}
	defer svc.Close()
	png, err := svc.TakeScreenshot()
	if err != nil {
		return nil, fmt.Errorf("画面を撮れません：%v", err)
	}
	return png, nil
}

// 比べるための小さな白黒の絵（78x168。1 点が 5pt 四方）。
const thumbW, thumbH = 78, 168

func thumb(png []byte) []uint8 {
	src, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		return nil
	}
	b := src.Bounds()
	out := make([]uint8, thumbW*thumbH)
	bw, bh := float64(b.Dx())/thumbW, float64(b.Dy())/thumbH
	for ty := 0; ty < thumbH; ty++ {
		for tx := 0; tx < thumbW; tx++ {
			var sum uint32
			for sy := 0; sy < 3; sy++ {
				for sx := 0; sx < 3; sx++ {
					x := b.Min.X + int((float64(tx)+(float64(sx)+0.5)/3)*bw)
					y := b.Min.Y + int((float64(ty)+(float64(sy)+0.5)/3)*bh)
					r, g, bl, _ := src.At(x, y).RGBA()
					sum += (r*299 + g*587 + bl*114) / 1000 >> 8
				}
			}
			out[ty*thumbW+tx] = uint8(sum / 9)
		}
	}
	return out
}

// changedDots は 2 枚の絵で、はっきり違う点の数。
func changedDots(a, b []uint8) int {
	if len(a) != len(b) || len(a) == 0 {
		return math.MaxInt32
	}
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > 20 {
			n++
		}
	}
	return n
}

// ---- 操作のあと ----

type afterOpts struct {
	Show   string  // changed（変わったら画面を付ける・既定）／always／never
	Log    string  // 操作のあとに出たログのうち、これに合う行を返す
	Settle float64 // 画面が落ち着くまで最大何秒待つか（既定 3）
}

// act は「前の画面 → 操作 → 後の様子」をまとめて返す。
func (a *app) act(ctx context.Context, o afterOpts, op func() (string, error)) *mcp.CallToolResult {
	start := time.Now()
	var before []uint8
	if o.Show != "never" && o.Show != "always" {
		if png, err := a.shotPNG(ctx); err == nil {
			before = thumb(png)
		}
	}
	msg, err := op()
	if err != nil {
		return fail("%v", err)
	}
	return a.after(ctx, o, start, before, msg, false)
}

func (a *app) after(ctx context.Context, o afterOpts, start time.Time, before []uint8, msg string, isErr bool) *mcp.CallToolResult {
	lines := []string{msg}
	var last []byte
	changed := true
	if o.Show != "never" {
		settle := o.Settle
		if settle <= 0 {
			settle = 3
		}
		deadline := time.Now().Add(time.Duration(settle * float64(time.Second)))
		time.Sleep(400 * time.Millisecond) // 押した直後はまだ動き出していないことがある
		var lastT []uint8
		stable, polls := false, 0
		for {
			png, err := a.shotPNG(ctx)
			if err != nil {
				lines = append(lines, "画面：撮れない（"+err.Error()+"）")
				break
			}
			t := thumb(png)
			polls++
			if lastT != nil && changedDots(t, lastT) <= 2 {
				// 前と同じ。ただし操作前とも同じなら、動き出しが遅いだけかもしれないので 1 回だけ待ち直す
				if before != nil && changedDots(t, before) <= 3 && polls < 4 && time.Now().Before(deadline) {
					last, lastT = png, t
					time.Sleep(500 * time.Millisecond)
					continue
				}
				last, lastT, stable = png, t, true
				break
			}
			last, lastT = png, t
			if time.Now().After(deadline) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if lastT != nil {
			if before != nil {
				changed = changedDots(before, lastT) > 3
			}
			s := "画面："
			switch {
			case before == nil:
				s += "撮った"
			case changed:
				s += "変わった"
			default:
				s += "変わらない"
			}
			if stable {
				s += fmt.Sprintf("（%.1f 秒で落ち着いた）", time.Since(start).Seconds())
			} else {
				s += fmt.Sprintf("（%.0f 秒たってもまだ動いている）", settle)
			}
			lines = append(lines, s)
		}
	}
	if fa := a.frontApp(ctx); fa != "" {
		lines = append(lines, "前のアプリ："+fa)
	}
	if o.Log != "" {
		since := int(math.Ceil(time.Since(start).Seconds())) + 1
		got, err := a.phone.Logs(ctx, o.Log, since, 0, 30)
		switch {
		case err != nil:
			lines = append(lines, "ログ："+err.Error())
		case len(got) == 0:
			lines = append(lines, fmt.Sprintf("ログ：「%s」に合う行はまだない", o.Log))
		default:
			lines = append(lines, "ログ：\n"+strings.Join(got, "\n"))
		}
	}
	res := &mcp.CallToolResult{IsError: isErr}
	attach := last != nil && (o.Show == "always" || (o.Show != "never" && changed))
	if attach {
		if jpg, _, _, err := shrink(last, 3, 80); err == nil {
			res.Content = append(res.Content, &mcp.ImageContent{Data: jpg, MIMEType: "image/jpeg"})
		}
	}
	res.Content = append(res.Content, &mcp.TextContent{Text: strings.Join(lines, "\n")})
	return res
}

// frontApp は前に出ているアプリ（名前と bundle id）。
func (a *app) frontApp(ctx context.Context) string {
	if !a.phone.WDAReady() {
		return "" // 前のアプリを知るためだけに WDA を起こさない
	}
	b, err := a.phone.WDA(ctx, "GET", "/wda/activeAppInfo", nil)
	if err != nil {
		return ""
	}
	var v struct {
		Value struct {
			BundleID string `json:"bundleId"`
		} `json:"value"`
	}
	if json.Unmarshal(b, &v) != nil || v.Value.BundleID == "" {
		return ""
	}
	if n := a.phone.AppName(v.Value.BundleID); n != "" {
		return n + "（" + v.Value.BundleID + "）"
	}
	return v.Value.BundleID
}

// ---- 部品を探す ----

func predQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `'`, `\'`)
}

// textPred は文字で部品を探す条件。partial なら一部が合えばよい。
func textPred(text string, partial bool) string {
	q := predQuote(text)
	op := "=="
	if partial {
		op = "CONTAINS"
	}
	var ors []string
	for _, f := range []string{"label", "name", "value", "placeholderValue"} {
		ors = append(ors, fmt.Sprintf("%s %s '%s'", f, op, q))
	}
	return "(" + strings.Join(ors, " OR ") + ")"
}

func (a *app) findIDs(ctx context.Context, pred string) ([]string, error) {
	b, err := a.phone.WDA(ctx, "POST", "/session/{s}/elements", map[string]any{"using": "predicate string", "value": pred})
	if err != nil {
		return nil, err
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
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (a *app) center(ctx context.Context, id string) (float64, float64, error) {
	b, err := a.phone.WDA(ctx, "GET", "/session/{s}/element/"+id+"/rect", nil)
	if err != nil {
		return 0, 0, err
	}
	var v struct {
		Value struct{ X, Y, Width, Height float64 } `json:"value"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return 0, 0, err
	}
	return v.Value.X + v.Value.Width/2, v.Value.Y + v.Value.Height/2, nil
}

// ---- 1 つずつの操作 ----

func (a *app) opTap(ctx context.Context, x, y float64, count int, hold float64) (string, error) {
	var err error
	what := "タップ"
	switch {
	case hold > 0:
		what = fmt.Sprintf("長押し %.1f 秒", hold)
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/touchAndHold", map[string]any{"x": x, "y": y, "duration": hold})
	case count == 2:
		what = "ダブルタップ"
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/doubleTap", map[string]any{"x": x, "y": y})
	default:
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/tap", map[string]any{"x": x, "y": y})
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s (%.0f,%.0f) %s", what, x, y, time.Now().Format("15:04:05.000")), nil
}

func (a *app) opSwipe(ctx context.Context, fx, fy, tx, ty, dur float64) (string, error) {
	if dur <= 0 {
		dur = 0.2
	}
	if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/dragfromtoforduration",
		map[string]any{"fromX": fx, "fromY": fy, "toX": tx, "toY": ty, "duration": dur}); err != nil {
		return "", err
	}
	return fmt.Sprintf("スワイプ (%.0f,%.0f)→(%.0f,%.0f) %s", fx, fy, tx, ty, time.Now().Format("15:04:05.000")), nil
}

func (a *app) opTapText(ctx context.Context, text string, index int, partial bool, count int, hold float64) (string, error) {
	// 見えている部品を先に探す（同じ文字の隠れた部品を押さないように。2026-11-24 マップの「出発」で外れた）
	ids, err := a.findIDs(ctx, textPred(text, partial)+" AND visible == 1")
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		if ids, err = a.findIDs(ctx, textPred(text, partial)); err != nil {
			return "", err
		}
	}
	if len(ids) <= index {
		return "", fmt.Errorf("「%s」が見つかりません（%d 個）。iphone_elements で画面の部品を確かめてください", text, len(ids))
	}
	id := ids[index]
	if count == 2 || hold > 0 {
		x, y, err := a.center(ctx, id)
		if err != nil {
			return "", err
		}
		m, err := a.opTap(ctx, x, y, count, hold)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("「%s」（%d 個中 %d 番）を%s", text, len(ids), index, m), nil
	}
	if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+id+"/click", map[string]any{}); err != nil {
		return "", err
	}
	return fmt.Sprintf("「%s」を押した（%d 個中 %d 番）%s", text, len(ids), index, time.Now().Format("15:04:05.000")), nil
}

func (a *app) opType(ctx context.Context, text, field string, partial, clear, enter bool) (string, error) {
	if enter {
		text += "\n"
	}
	if field == "" {
		if clear {
			return "", fmt.Errorf("消してから打つときは field（入力欄の文字）を指定してください")
		}
		if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/keys", map[string]any{"value": []string{text}}); err != nil {
			return "", fmt.Errorf("打てません（入力欄が選ばれていないかも。field で欄を指定するか、先に欄を押す）：%v", err)
		}
		return fmt.Sprintf("打った「%s」", strings.TrimSuffix(text, "\n")), nil
	}
	ids, err := a.findIDs(ctx, textPred(field, partial))
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("入力欄「%s」が見つかりません", field)
	}
	id := ids[0]
	if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+id+"/click", map[string]any{}); err != nil {
		return "", err
	}
	if clear {
		if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+id+"/clear", map[string]any{}); err != nil {
			return "", fmt.Errorf("消せません：%v", err)
		}
	}
	if text != "" {
		if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+id+"/value", map[string]any{"value": []string{text}}); err != nil {
			return "", err
		}
	}
	act := "打った"
	if clear {
		act = "消してから打った"
	}
	return fmt.Sprintf("「%s」に%s「%s」", field, act, strings.TrimSuffix(text, "\n")), nil
}

var buttonNames = "home / volumeup / volumedown / lock / unlock / hide_keyboard / notifications / control_center / back"

func (a *app) opButton(ctx context.Context, name string) (string, error) {
	var err error
	switch strings.ToLower(name) {
	case "home":
		_, err = a.phone.WDA(ctx, "POST", "/wda/homescreen", map[string]any{})
	case "volumeup", "volumedown":
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/pressButton", map[string]any{"name": strings.ToLower(name)})
	case "lock":
		// iOS 27 では WDA の lock はサイドボタンの長押しになり、Siri の画面が出るだけ（2026-11-25 に実機で確認）
		return "", fmt.Errorf("この iPhone（iOS 27）では道具から鍵をかけられません（Siri が出る）。かけるならカオルにサイドボタンを押してもらう")
	case "unlock":
		if locked, e := a.phone.Locked(); e == nil && locked {
			return "", fmt.Errorf("パスコードのロックがかかっています。外すのはカオル（Face ID かパスコード）")
		}
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/unlock", map[string]any{})
		if err == nil {
			if locked, e := a.phone.Locked(); e == nil && locked {
				return "", fmt.Errorf("ロックが外れません（パスコードが要る。外すのはカオル）")
			}
		}
	case "hide_keyboard":
		_, err = a.phone.WDA(ctx, "POST", "/session/{s}/wda/keyboard/dismiss", map[string]any{})
	case "notifications":
		_, err = a.opSwipe(ctx, 120, 2, 120, 600, 0.3)
	case "control_center":
		_, err = a.opSwipe(ctx, 360, 2, 360, 500, 0.3)
	case "back":
		_, err = a.opSwipe(ctx, 1, 422, 300, 422, 0.3)
	default:
		return "", fmt.Errorf("name は %s のどれか", buttonNames)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s を押した %s", name, time.Now().Format("15:04:05.000")), nil
}

func (a *app) opPinch(ctx context.Context, scale, velocity float64) (string, error) {
	if scale <= 0 {
		return "", fmt.Errorf("scale は 0 より大きく（2 で 2 倍に拡大、0.5 で縮小）")
	}
	if velocity == 0 {
		velocity = 1
		if scale < 1 {
			velocity = -1
		}
	}
	if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/pinch", map[string]any{"scale": scale, "velocity": velocity}); err != nil {
		return "", err
	}
	return fmt.Sprintf("ピンチ %.2f 倍", scale), nil
}

// opAlert：read（読むだけ）／accept／dismiss。button で押すボタンの文字を選べる。
func (a *app) opAlert(ctx context.Context, action, button string) (string, error) {
	switch action {
	case "read", "":
		b, err := a.phone.WDA(ctx, "GET", "/session/{s}/alert/text", nil)
		if err != nil {
			if strings.Contains(err.Error(), "no such alert") || strings.Contains(err.Error(), "404") {
				return "ポップアップは出ていません", nil
			}
			return "", err
		}
		var t struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(b, &t)
		var bs struct {
			Value []string `json:"value"`
		}
		if b2, err := a.phone.WDA(ctx, "GET", "/session/{s}/wda/alert/buttons", nil); err == nil {
			_ = json.Unmarshal(b2, &bs)
		}
		return fmt.Sprintf("ポップアップ：%s\nボタン：%s", t.Value, strings.Join(bs.Value, " / ")), nil
	case "accept", "dismiss":
		body := map[string]any{}
		if button != "" {
			body["name"] = button
		}
		if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/alert/"+action, body); err != nil {
			return "", err
		}
		if button != "" {
			return fmt.Sprintf("ポップアップの「%s」を押した", button), nil
		}
		return "ポップアップを " + action + " した", nil
	}
	return "", fmt.Errorf("action は read / accept / dismiss")
}

// opWait は文字が出る（gone なら消える）まで待つ。
func (a *app) opWait(ctx context.Context, text string, partial, gone bool, seconds float64) (string, error) {
	if seconds <= 0 {
		seconds = 10
	}
	pred := textPred(text, partial) + " AND visible == 1"
	start := time.Now()
	for {
		ids, err := a.findIDs(ctx, pred)
		if err != nil {
			return "", err
		}
		if (len(ids) > 0) != gone {
			if gone {
				return fmt.Sprintf("「%s」が消えた（%.1f 秒）", text, time.Since(start).Seconds()), nil
			}
			return fmt.Sprintf("「%s」が出た（%.1f 秒）", text, time.Since(start).Seconds()), nil
		}
		if time.Since(start).Seconds() >= seconds {
			if gone {
				return "", fmt.Errorf("%.0f 秒待っても「%s」が消えません", seconds, text)
			}
			return "", fmt.Errorf("%.0f 秒待っても「%s」が出ません", seconds, text)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
}

// opScrollTo は文字が画面に出るまでスクロールする。tap なら見つけたら押す。
func (a *app) opScrollTo(ctx context.Context, text string, partial bool, direction string, max int, tap bool) (string, error) {
	if max <= 0 {
		max = 10
	}
	// direction は「中身をどちらへ進めるか」。down = 下の方を見る（指は下から上へ）
	var fx, fy, tx, ty float64
	switch direction {
	case "", "down":
		fx, fy, tx, ty = 195, 620, 195, 260
	case "up":
		fx, fy, tx, ty = 195, 260, 195, 620
	case "right":
		fx, fy, tx, ty = 330, 422, 60, 422
	case "left":
		fx, fy, tx, ty = 60, 422, 330, 422
	default:
		return "", fmt.Errorf("direction は down / up / left / right")
	}
	pred := textPred(text, partial) + " AND visible == 1"
	var prev []uint8
	for i := 0; i <= max; i++ {
		ids, err := a.findIDs(ctx, pred)
		if err != nil {
			return "", err
		}
		if len(ids) > 0 {
			x, y, _ := a.center(ctx, ids[0])
			// 画面の下の端（ホームバーの辺り）に半分隠れているなら、もう少し進める
			if y > 0 && y < 790 && y > 50 {
				if tap {
					if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/element/"+ids[0]+"/click", map[string]any{}); err != nil {
						return "", err
					}
					return fmt.Sprintf("「%s」が出たので押した（%d 回スクロール、中心 %.0f,%.0f）", text, i, x, y), nil
				}
				return fmt.Sprintf("「%s」が出た（%d 回スクロール、中心 %.0f,%.0f）", text, i, x, y), nil
			}
		}
		if i == max {
			break
		}
		if png, err := a.shotPNG(ctx); err == nil {
			t := thumb(png)
			if prev != nil && changedDots(prev, t) <= 3 {
				return "", fmt.Errorf("端まで来ても「%s」が見つかりません（%d 回スクロール）", text, i)
			}
			prev = t
		}
		if _, err := a.opSwipe(ctx, fx, fy, tx, ty, 0.25); err != nil {
			return "", err
		}
		time.Sleep(600 * time.Millisecond)
	}
	return "", fmt.Errorf("%d 回スクロールしても「%s」が見つかりません", max, text)
}

// ---- アプリ ----

// 標準アプリの日本語名（インストールの一覧には英語の名前しか入っていないため）
var systemApps = map[string]string{
	"設定": "com.apple.Preferences", "safari": "com.apple.mobilesafari", "写真": "com.apple.mobileslideshow",
	"カメラ": "com.apple.camera", "メッセージ": "com.apple.MobileSMS", "メール": "com.apple.mobilemail",
	"電話": "com.apple.mobilephone", "app store": "com.apple.AppStore", "ファイル": "com.apple.DocumentsApp",
	"ミュージック": "com.apple.Music", "マップ": "com.apple.Maps", "カレンダー": "com.apple.mobilecal",
	"時計": "com.apple.mobiletimer", "メモ": "com.apple.mobilenotes", "リマインダー": "com.apple.reminders",
	"天気": "com.apple.weather", "計算機": "com.apple.calculator", "ショートカット": "com.apple.shortcuts",
	"ヘルスケア": "com.apple.Health", "ウォレット": "com.apple.Passbook", "連絡先": "com.apple.MobileAddressBook",
	"翻訳": "com.apple.Translate", "ボイスメモ": "com.apple.VoiceMemos", "探す": "com.apple.findmy",
}

var bundleRe = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)

// resolveApp は「名前か bundle id」を bundle id にする。
func (a *app) resolveApp(ctx context.Context, name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" || strings.EqualFold(n, "livecontainer") {
		if lc := a.phone.LiveContainer(); lc != "" {
			return lc, nil
		}
		return "", fmt.Errorf("LiveContainer が見つかりません")
	}
	if id, ok := systemApps[strings.ToLower(n)]; ok {
		return id, nil
	}
	if id := a.phone.FindApp(n); id != "" {
		return id, nil
	}
	if bundleRe.MatchString(n) {
		return n, nil
	}
	return "", fmt.Errorf("アプリ「%s」が見つかりません（bundle id で指定するか、ホーム画面の名前で）", name)
}

func (a *app) opApp(ctx context.Context, action, name, url string) (string, error) {
	if err := a.phone.Ensure(ctx); err != nil {
		return "", err
	}
	if action == "url" {
		if url == "" {
			return "", fmt.Errorf("url を指定してください")
		}
		if _, err := a.phone.WDA(ctx, "POST", "/session/{s}/url", map[string]any{"url": url}); err != nil {
			return "", err
		}
		return "URL を開いた：" + url, nil
	}
	b, err := a.resolveApp(ctx, name)
	if err != nil {
		return "", err
	}
	// まず中から（WDA を使わない）。だめなら下の WDA で
	switch action {
	case "launch":
		if pid, err := a.phone.Launch(b, true); err == nil {
			return fmt.Sprintf("launch %s（pid %d。中から起動し直した）", b, pid), nil
		}
	case "terminate":
		if ok, err := a.phone.Kill(b); err == nil {
			if !ok {
				return fmt.Sprintf("terminate %s → 動いていなかった", b), nil
			}
			return fmt.Sprintf("terminate %s → 終わらせた（中から）", b), nil
		}
	case "activate":
		if !a.phone.WDAReady() {
			// WDA が動いていなければ、中から起動をたのむ（動いているアプリは前に出る）
			if pid, err := a.phone.Launch(b, false); err == nil {
				return fmt.Sprintf("activate %s（pid %d。中から）", b, pid), nil
			}
		}
	case "state":
		if pid, err := a.phone.pidOf(b); err == nil {
			if pid == 0 {
				return fmt.Sprintf("%s：動いていない", b), nil
			}
			s := fmt.Sprintf("%s：動いている（pid %d）", b, pid)
			if a.phone.WDAReady() {
				if r, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/apps/state", map[string]any{"bundleId": b}); err == nil && strings.TrimSpace(gjsonValue(r)) == "4" {
					s += "・前に出ている"
				}
			}
			return s, nil
		}
	}
	switch action {
	case "activate", "terminate", "launch":
		r, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/apps/"+action, map[string]any{"bundleId": b})
		if err != nil {
			return "", err
		}
		if action == "terminate" {
			return fmt.Sprintf("terminate %s → %s", b, strings.TrimSpace(gjsonValue(r))), nil
		}
		return fmt.Sprintf("%s %s", action, b), nil
	case "state":
		r, err := a.phone.WDA(ctx, "POST", "/session/{s}/wda/apps/state", map[string]any{"bundleId": b})
		if err != nil {
			return "", err
		}
		st := map[string]string{"0": "わからない", "1": "動いていない", "2": "裏で止まっている", "3": "裏で動いている", "4": "前に出ている"}
		v := strings.TrimSpace(gjsonValue(r))
		if s, ok := st[v]; ok {
			v = s
		}
		return fmt.Sprintf("%s：%s", b, v), nil
	}
	return "", fmt.Errorf("action は activate / launch / terminate / state / url")
}

// gjsonValue は WDA の返事の value だけを文字にする。
func gjsonValue(b []byte) string {
	var v struct {
		Value any `json:"value"`
	}
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	o, _ := json.Marshal(v.Value)
	return string(o)
}

// ---- アプリの一覧（名前 → bundle id）----

func (p *Phone) loadAppsLocked() {
	if p.appNames != nil {
		return
	}
	conn, err := installationproxy.New(p.device)
	if err != nil {
		return
	}
	defer conn.Close()
	apps, err := conn.BrowseAllApps()
	if err != nil {
		return
	}
	p.appNames = map[string]string{}
	p.appExec = map[string]string{}
	for _, x := range apps {
		id := x.CFBundleIdentifier()
		if e := x.CFBundleExecutable(); id != "" && e != "" {
			p.appExec[id] = e
		}
		name, _ := x["CFBundleDisplayName"].(string)
		if name == "" {
			name = x.CFBundleName()
		}
		if id != "" {
			p.appNames[id] = name
		}
	}
}

// AppName は bundle id からアプリの名前。
func (p *Phone) AppName(bundle string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasRSD {
		return ""
	}
	p.loadAppsLocked()
	return p.appNames[bundle]
}

// FindApp は名前（大文字小文字は無視）から bundle id。
func (p *Phone) FindApp(name string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.hasRSD {
		return ""
	}
	p.loadAppsLocked()
	for id, n := range p.appNames {
		if strings.EqualFold(n, name) || strings.EqualFold(id, name) {
			return id
		}
	}
	return ""
}
