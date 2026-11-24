package main

// iPhone の中身を見る・中から動かす（画面を押さずに、iOS の窓口に直接たのむ）。
// どれも go-ios の部品を使う。窓口が iOS 27 で答えることは 2026-11-25 に go-ios のコマンドで確かめた。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/afc"
	"github.com/danielpaulus/go-ios/ios/diagnostics"
	"github.com/danielpaulus/go-ios/ios/fileservice"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/house_arrest"
	"github.com/danielpaulus/go-ios/ios/imagemounter"
	"github.com/danielpaulus/go-ios/ios/instruments"
	"github.com/danielpaulus/go-ios/ios/mcinstall"
	"github.com/danielpaulus/go-ios/ios/pasteboard"
	"github.com/danielpaulus/go-ios/ios/simlocation"
	"github.com/danielpaulus/go-ios/ios/webinspector"
)

// devs は USB の端末とトンネルの端末を返す（つないでから使う）。
func (p *Phone) devs() (ios.DeviceEntry, ios.DeviceEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.device, p.rsd
}

func gb(b uint64) string { return fmt.Sprintf("%.1f GB", float64(b)/1e9) }
func mb(b uint64) string { return fmt.Sprintf("%d MB", (b+500_000)/1_000_000) }

// ---- 端末の様子 ----

// Locked は、パスコードのロックがかかっているか（lockdown の PasswordProtected）。
func (p *Phone) Locked() (bool, error) {
	usb, _ := p.devs()
	v, err := ios.GetValues(usb)
	if err != nil {
		return false, err
	}
	return v.Value.PasswordProtected, nil
}

func (p *Phone) InfoSummary() string {
	usb, rsd := p.devs()
	var out []string
	add := func(k, v string) { out = append(out, k+"："+v) }
	if v, err := ios.GetValues(usb); err == nil {
		add("機種", v.Value.ProductType)
		add("iOS", v.Value.ProductVersion+"（"+v.Value.BuildVersion+"）")
		if v.Value.PasswordProtected {
			add("ロック", "かかっている（パスコードが要る。外すのはカオル）")
		} else {
			add("ロック", "外れている")
		}
	} else {
		add("端末の情報", "取れない（"+err.Error()+"）")
	}
	if c, err := afc.New(usb); err == nil {
		if d, err := c.DeviceInfo(); err == nil {
			add("容量", fmt.Sprintf("全体 %s・空き %s（%d%%）", gb(d.TotalBytes), gb(d.FreeBytes), d.FreeBytes*100/max(d.TotalBytes, 1)))
		}
		c.Close()
	}
	if b, err := ios.GetBatteryDiagnostics(usb); err == nil {
		s := fmt.Sprintf("%d%%", b.BatteryCurrentCapacity)
		if b.BatteryIsCharging {
			s += "・充電中"
		} else if b.ExternalConnected {
			s += "・つないでいる（充電していない）"
		}
		if d, err := diagnostics.New(usb); err == nil {
			if r, err := d.Battery(); err == nil && r.CycleCount > 0 {
				s += fmt.Sprintf("・充電の回数 %d", r.CycleCount)
			}
			d.Close()
		}
		add("電池", s)
	}
	if l, err := ios.GetLanguage(usb); err == nil {
		add("言語", l.Language+"（地域 "+l.Locale+"）")
	}
	if h24, err := ios.GetUses24HourClock(usb); err == nil {
		add("時刻の形式", map[bool]string{true: "24 時間", false: "12 時間"}[h24])
	}
	if on, err := imagemounter.IsDevModeEnabled(usb); err == nil {
		add("デベロッパモード", map[bool]string{true: "オン", false: "オフ"}[on])
	}
	if c, err := mcinstall.New(usb); err == nil {
		if l, err := c.HandleList(); err == nil {
			add("構成プロファイル", fmt.Sprintf("%d 個", len(l)))
		}
		c.Close()
	}
	if svc, err := instruments.NewDeviceInfoService(rsd); err == nil {
		if l, err := svc.ProcessList(); err == nil {
			n := 0
			for _, x := range l {
				if x.IsApplication {
					n++
				}
			}
			add("動いているもの", fmt.Sprintf("%d 個（アプリ %d）", len(l), n))
		}
		svc.Close()
	}
	return strings.Join(out, "\n")
}

// ---- 動いているもの ----

func (p *Phone) Processes(match string, all bool, limit int) (string, error) {
	_, rsd := p.devs()
	re, err := regexp.Compile("(?i)" + match)
	if err != nil {
		return "", fmt.Errorf("match の正規表現が正しくありません：%w", err)
	}
	svc, err := instruments.NewDeviceInfoService(rsd)
	if err != nil {
		return "", fmt.Errorf("動いているものの一覧の窓口につなげません：%w", err)
	}
	defer svc.Close()
	l, err := svc.ProcessList()
	if err != nil {
		return "", err
	}
	sort.Slice(l, func(i, j int) bool { return l[i].StartDate.After(l[j].StartDate) })
	p.mu.Lock()
	p.loadAppsLocked()
	byExec := map[string]string{}
	for id, e := range p.appExec {
		byExec[e] = p.appNames[id] + "（" + id + "）"
	}
	p.mu.Unlock()
	var lines []string
	total := 0
	for _, x := range l {
		if !all && !x.IsApplication {
			continue
		}
		name := x.Name
		if n, ok := byExec[x.Name]; ok {
			name = n
		}
		if match != "" && !re.MatchString(name) && !re.MatchString(x.RealAppName) {
			continue
		}
		total++
		if len(lines) < limit {
			lines = append(lines, fmt.Sprintf("%s  pid %d  起動 %s", name, x.Pid, x.StartDate.Local().Format("01/02 15:04")))
		}
	}
	kind := "アプリ"
	if all {
		kind = "プログラム"
	}
	head := fmt.Sprintf("動いている%s %d 個（新しく起動した順）", kind, total)
	if total > len(lines) {
		head += fmt.Sprintf("。上から %d 個", len(lines))
	}
	return head + "\n" + strings.Join(lines, "\n"), nil
}

// pidOf は bundle id のアプリのプロセス番号（動いていなければ 0）。
func (p *Phone) pidOf(bundle string) (uint64, error) {
	_, rsd := p.devs()
	p.mu.Lock()
	p.loadAppsLocked()
	exec := p.appExec[bundle]
	p.mu.Unlock()
	if exec == "" {
		return 0, fmt.Errorf("%s が入っていません", bundle)
	}
	svc, err := instruments.NewDeviceInfoService(rsd)
	if err != nil {
		return 0, err
	}
	defer svc.Close()
	l, err := svc.ProcessList()
	if err != nil {
		return 0, err
	}
	for _, x := range l {
		if x.Name == exec {
			return x.Pid, nil
		}
	}
	return 0, nil
}

// Launch は WDA を使わずにアプリを起動する（restart なら動いているものを終わらせてから）。
func (p *Phone) Launch(bundle string, restart bool) (uint64, error) {
	_, rsd := p.devs()
	pc, err := instruments.NewProcessControl(rsd)
	if err != nil {
		return 0, err
	}
	defer pc.Close()
	kill := uint64(0)
	if restart {
		kill = 1
	}
	return pc.LaunchApp(bundle, map[string]any{"KillExisting": kill})
}

// Kill は WDA を使わずにアプリを終わらせる。動いていなければ false。
func (p *Phone) Kill(bundle string) (bool, error) {
	pid, err := p.pidOf(bundle)
	if err != nil || pid == 0 {
		return false, err
	}
	_, rsd := p.devs()
	pc, err := instruments.NewProcessControl(rsd)
	if err != nil {
		return false, err
	}
	defer pc.Close()
	return true, pc.KillProcess(pid)
}

// ---- 入っているアプリと大きさ ----

func (p *Phone) AppSizes(match string, limit int) (string, error) {
	usb, _ := p.devs()
	re, err := regexp.Compile("(?i)" + match)
	if err != nil {
		return "", fmt.Errorf("match の正規表現が正しくありません：%w", err)
	}
	conn, err := ios.ConnectToService(usb, "com.apple.mobile.installation_proxy")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	codec := ios.NewPlistCodec()
	req, err := codec.Encode(map[string]any{"Command": "Browse", "ClientOptions": map[string]any{
		"ApplicationType":  "User",
		"ReturnAttributes": []any{"CFBundleIdentifier", "CFBundleDisplayName", "CFBundleName", "CFBundleShortVersionString", "StaticDiskUsage", "DynamicDiskUsage"},
	}})
	if err != nil {
		return "", err
	}
	if err := conn.Send(req); err != nil {
		return "", err
	}
	type row struct {
		name, id, ver string
		app, data     uint64
	}
	var rows []row
	r := conn.Reader()
	for i := 0; i < 1000; i++ {
		b, err := codec.Decode(r)
		if err != nil {
			return "", err
		}
		m, err := ios.ParsePlist(b)
		if err != nil {
			return "", err
		}
		if l, ok := m["CurrentList"].([]any); ok {
			for _, x := range l {
				a, _ := x.(map[string]any)
				if a == nil {
					continue
				}
				s := func(k string) string { v, _ := a[k].(string); return v }
				name := s("CFBundleDisplayName")
				if name == "" {
					name = s("CFBundleName")
				}
				rows = append(rows, row{name: name, id: s("CFBundleIdentifier"), ver: s("CFBundleShortVersionString"), app: toU64(a["StaticDiskUsage"]), data: toU64(a["DynamicDiskUsage"])})
			}
		}
		if st, _ := m["Status"].(string); st == "Complete" {
			break
		}
		if e, _ := m["Error"].(string); e != "" {
			return "", fmt.Errorf("一覧を取れません：%s", e)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].app+rows[i].data > rows[j].app+rows[j].data })
	var lines []string
	total, shown := uint64(0), 0
	for _, x := range rows {
		if match != "" && !re.MatchString(x.name) && !re.MatchString(x.id) {
			continue
		}
		total += x.app + x.data
		if shown < limit {
			lines = append(lines, fmt.Sprintf("%s（%s）%s  本体 %s・データ %s", x.name, x.id, x.ver, mb(x.app), mb(x.data)))
			shown++
		}
	}
	head := fmt.Sprintf("自分で入れたアプリ %d 個（大きい順）。合わせて %s", len(rows), gb(total))
	if match != "" {
		head = fmt.Sprintf("「%s」に合うアプリ（大きい順）。合わせて %s", match, gb(total))
	}
	return head + "\n" + strings.Join(lines, "\n"), nil
}

func toU64(v any) uint64 {
	switch x := v.(type) {
	case uint64:
		return x
	case int64:
		return uint64(x)
	case int:
		return uint64(x)
	case float64:
		return uint64(x)
	}
	return 0
}

// ---- 落ちた記録（クラッシュ） ----

func (p *Phone) Crashes(match, file string, limit, maxChars int) (string, error) {
	_, rsd := p.devs()
	c, err := fileservice.New(rsd, fileservice.DomainSystemCrashLogs, "")
	if err != nil {
		return "", fmt.Errorf("落ちた記録の窓口につなげません：%w", err)
	}
	defer c.Close()
	if file != "" {
		var buf bytes.Buffer
		if err := c.PullFile(file, &buf); err != nil {
			return "", fmt.Errorf("「%s」を読めません：%w", file, err)
		}
		s := buf.String()
		if len(s) > maxChars {
			s = cut(s, maxChars) + fmt.Sprintf("\n…（全部で %d 文字。max_chars で増やせる）", buf.Len())
		}
		return s, nil
	}
	re, err := regexp.Compile("(?i)" + match)
	if err != nil {
		return "", fmt.Errorf("match の正規表現が正しくありません：%w", err)
	}
	names, err := c.ListDirectory(".")
	if err != nil {
		return "", err
	}
	var hit []string
	for _, n := range names {
		if n == "." || n == ".." || (match != "" && !re.MatchString(n)) {
			continue
		}
		hit = append(hit, n)
	}
	// 名前に日時が入っている（アプリ名-2026-11-25-073012.ips）ので、日時の新しい順に並べる
	dateRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2}-\d{6}`)
	sort.SliceStable(hit, func(i, j int) bool { return dateRe.FindString(hit[i]) > dateRe.FindString(hit[j]) })
	head := fmt.Sprintf("落ちた記録 %d 個（新しい順）", len(hit))
	if len(hit) > limit {
		head += fmt.Sprintf("。上から %d 個", limit)
		hit = hit[:limit]
	}
	return head + "。中身は file に名前を入れて読む\n" + strings.Join(hit, "\n"), nil
}

// cut は UTF-8 の文字の途中で切らないように切る。
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// ---- アプリの中のファイル（開発者署名のアプリ）と写真などのフォルダ ----

// filesClient は、app が media なら写真などのフォルダ、それ以外はそのアプリの書類の場所。
func (p *Phone) filesClient(bundle string) (*afc.Client, string, error) {
	usb, _ := p.devs()
	if strings.EqualFold(bundle, "media") {
		c, err := afc.New(usb)
		return c, "写真などのフォルダ", err
	}
	c, err := house_arrest.New(usb, bundle)
	if err != nil {
		return nil, "", fmt.Errorf("%s の中を開けません（開けるのは開発者の署名のアプリだけ：LiveContainer・WebDriverAgent など）：%w", bundle, err)
	}
	return c, bundle, nil
}

func (p *Phone) FilesList(bundle, dir string, recursive bool, limit int) (string, error) {
	c, where, err := p.filesClient(bundle)
	if err != nil {
		return "", err
	}
	defer c.Close()
	if dir == "" {
		dir = "/"
	}
	var lines []string
	more := false
	if recursive {
		_ = c.WalkDir(dir, func(pth string, fi afc.FileInfo, err error) error {
			if err != nil || pth == dir {
				return nil
			}
			if strings.Count(strings.TrimPrefix(pth, dir), "/") > 4 && fi.IsDir() {
				return fs.SkipDir
			}
			if len(lines) >= limit {
				more = true
				return fs.SkipAll
			}
			if fi.IsDir() {
				lines = append(lines, pth+"/")
			} else {
				lines = append(lines, fmt.Sprintf("%s  %s", pth, size(fi.Size)))
			}
			return nil
		})
	} else {
		names, err := c.List(dir)
		if err != nil {
			return "", fmt.Errorf("%s の %s を見られません：%w", where, dir, err)
		}
		for _, n := range names {
			if n == "." || n == ".." {
				continue
			}
			if len(lines) >= limit {
				more = true
				break
			}
			fi, err := c.Stat(path.Join(dir, n))
			switch {
			case err != nil:
				lines = append(lines, n)
			case fi.IsDir():
				lines = append(lines, n+"/")
			default:
				lines = append(lines, fmt.Sprintf("%s  %s", n, size(fi.Size)))
			}
		}
	}
	head := fmt.Sprintf("%s の %s（%d 個）", where, dir, len(lines))
	if more {
		head += "。多いので途中まで"
	}
	return head + "\n" + strings.Join(lines, "\n"), nil
}

func size(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%d KB", (n+500)/1000)
	}
	return fmt.Sprintf("%d B", n)
}

func (p *Phone) FilesRead(bundle, file string, maxChars int) (string, error) {
	c, _, err := p.filesClient(bundle)
	if err != nil {
		return "", err
	}
	defer c.Close()
	b, err := afcRead(c, file)
	if err != nil {
		return "", fmt.Errorf("%s を読めません：%w", file, err)
	}
	head := b
	if len(head) > 8000 {
		head = head[:8000]
	}
	if bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(bytes.ToValidUTF8(head, nil)) {
		return fmt.Sprintf("%s は文字のファイルではありません（%s）。PC に取るなら action=pull", file, size(int64(len(b)))), nil
	}
	s := string(b)
	if len(s) > maxChars {
		s = cut(s, maxChars) + fmt.Sprintf("\n…（全部で %s。max_chars で増やせる）", size(int64(len(b))))
	}
	return s, nil
}

func (p *Phone) FilesWrite(bundle, file string, data []byte) error {
	c, _, err := p.filesClient(bundle)
	if err != nil {
		return err
	}
	defer c.Close()
	if dir := path.Dir(file); dir != "/" && dir != "." {
		_ = c.MkDir(dir)
	}
	return afcWrite(c, file, data)
}

// pcPath は PC の置き場所を決める。空なら道具の files フォルダ。ユーザーフォルダの外は使わない。
func pcPath(stateDir, local, remote string) (string, error) {
	if local == "" {
		return filepath.Join(stateDir, "files", path.Base(remote)), nil
	}
	l, err := filepath.Abs(local)
	if err != nil {
		return "", err
	}
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	rel, err := filepath.Rel(home, l)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("PC の場所はユーザーフォルダ（%s）の中だけ", home)
	}
	return l, nil
}

// ---- クリップボード・仮の位置 ----

func (p *Phone) ClipboardGet() (string, bool, error) {
	_, rsd := p.devs()
	c, err := pasteboard.New(rsd)
	if err != nil {
		return "", false, err
	}
	defer c.Close()
	return c.GetText()
}

func (p *Phone) ClipboardSet(s string) error {
	_, rsd := p.devs()
	c, err := pasteboard.New(rsd)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.SetText(s)
}

// Location は仮の位置にする／戻す。iOS 17 からは instruments の LocationSimulation を使い、
// つないだままの間だけ効く（だから p.loc に持ち続け、reset か切るときに止める）。
func (p *Phone) Location(reset bool, lat, lon float64) error {
	usb, rsd := p.devs()
	p.locMu.Lock()
	defer p.locMu.Unlock()
	if reset {
		if p.loc != nil {
			err := p.loc.StopSimulateLocation()
			p.loc = nil
			if err == nil {
				return nil
			}
		}
		if err := simlocation.ResetLocation(usb); err == nil {
			return nil
		}
		return nil // もともと仮の位置にしていない
	}
	if p.loc == nil {
		l, err := instruments.NewLocationSimulationService(rsd)
		if err != nil {
			if err2 := simlocation.SetLocation(usb, strconv.FormatFloat(lat, 'f', 6, 64), strconv.FormatFloat(lon, 'f', 6, 64)); err2 == nil {
				return nil
			}
			return fmt.Errorf("仮の位置の窓口が答えません：%w", err)
		}
		p.loc = l
	}
	if err := p.loc.StartSimulateLocation(lat, lon); err != nil {
		p.loc.Close()
		p.loc = nil
		return fmt.Errorf("仮の位置にできません：%w", err)
	}
	return nil
}

// stopLocation は切るときに仮の位置を止める。
func (p *Phone) stopLocation() {
	p.locMu.Lock()
	defer p.locMu.Unlock()
	if p.loc != nil {
		_ = p.loc.StopSimulateLocation()
		p.loc = nil
	}
}

// ---- Safari などの中のページ（Web インスペクタ。iPhone の設定でオンが要る） ----

func (p *Phone) webClient(ctx context.Context) (*webinspector.Client, error) {
	_, rsd := p.devs()
	c, err := webinspector.New(rsd)
	if err != nil {
		return nil, fmt.Errorf("Web インスペクタの窓口につなげません：%w", err)
	}
	if err := c.Connect(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("Web インスペクタが答えません（iPhone の設定 → アプリ → Safari → 詳細 → Web インスペクタ がオンか）：%w", err)
	}
	return c, nil
}

func webPages(ctx context.Context, c *webinspector.Client) ([]webinspector.ApplicationPage, error) {
	l, err := c.ListPages(ctx, 700*time.Millisecond)
	if err != nil {
		return nil, err
	}
	var out []webinspector.ApplicationPage
	for _, x := range l {
		if x.Page.Type == webinspector.WIRTypeWeb || x.Page.Type == webinspector.WIRTypeWebPage || x.Page.Type == webinspector.WIRTypeJavaScript {
			out = append(out, x)
		}
	}
	return out, nil
}

func (p *Phone) WebList(ctx context.Context) (string, error) {
	c, err := p.webClient(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	l, err := webPages(ctx, c)
	if err != nil {
		return "", err
	}
	if len(l) == 0 {
		return "見られるページはありません（Safari でページを開くか、アプリの中の Web の画面を出す）", nil
	}
	var lines []string
	for _, x := range l {
		lines = append(lines, fmt.Sprintf("page=%s  %s  「%s」 %s", x.Page.Key, x.Application.Name, x.Page.Title, x.Page.URL))
	}
	return fmt.Sprintf("ページ %d 個（page の値を eval で使う）\n%s", len(l), strings.Join(lines, "\n")), nil
}

// WebEval はページで JavaScript を動かして結果を返す。page が空なら Safari のいちばん上のページ。
func (p *Phone) WebEval(ctx context.Context, page, expr string) (string, error) {
	c, err := p.webClient(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	l, err := webPages(ctx, c)
	if err != nil {
		return "", err
	}
	var hit *webinspector.ApplicationPage
	for i := range l {
		if (page != "" && l[i].Page.Key == page) || (page == "" && l[i].Application.BundleID == webinspector.SafariBundleID) {
			hit = &l[i]
			break
		}
	}
	if hit == nil {
		return "", fmt.Errorf("ページが見つかりません（action=list で page を確かめる）")
	}
	r, err := c.Evaluate(ctx, hit.Application, hit.Page, expr)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(r)
	s := string(b)
	if len(s) > 20000 {
		s = cut(s, 20000) + "…"
	}
	return fmt.Sprintf("「%s」 %s\n%s", hit.Page.Title, hit.Page.URL, s), nil
}

// waitPid は、起動したアプリが一覧に出るまで少し待つ（確かめ）。
func (p *Phone) waitPid(bundle string, sec float64) uint64 {
	deadline := time.Now().Add(time.Duration(sec * float64(time.Second)))
	for time.Now().Before(deadline) {
		if pid, err := p.pidOf(bundle); err == nil && pid > 0 {
			return pid
		}
		time.Sleep(300 * time.Millisecond)
	}
	return 0
}

// ---- 自分のアプリ（DevKit）ののぞき窓 ----

const devkitPort = 8765

// DevKitCall は DevKit ののぞき窓（iPhone の中の 127.0.0.1:8765）に USB 経由で聞く。
func (p *Phone) DevKitCall(ctx context.Context, target string) (int, string, error) {
	usb, _ := p.devs()
	deviceID := usb.DeviceID
	cl := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				a, b := net.Pipe()
				go func() { _ = forward.StartNewProxyConnection(ctx, b, deviceID, devkitPort) }()
				return a, nil
			},
			DisableKeepAlives: true,
		},
	}
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://devkit"+target, nil)
	if err != nil {
		return 0, "", err
	}
	res, err := cl.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("DevKit ののぞき窓に届きません（DevKit が前に出ていて動いているか。裏に回ると止まる）：%w", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, string(b), nil
}
