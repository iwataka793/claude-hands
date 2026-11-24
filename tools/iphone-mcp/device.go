package main

// iPhone とのつながりを 1 か所で持つ（トンネル・ログ・WebDriverAgent）。
// 一度つないだら切らずに持ち続け、スイッチがオフになったら全部切る。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	ios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/forward"
	"github.com/danielpaulus/go-ios/ios/installationproxy"
	"github.com/danielpaulus/go-ios/ios/syslog"
	"github.com/danielpaulus/go-ios/ios/testmanagerd"
	"github.com/danielpaulus/go-ios/ios/tunnel"
)

const (
	wdaPort        = 8100
	tunnelBasePort = 60205 // go-ios のコマンド（既定 60105）とぶつからないように
	// holdPort は「いま iPhone を使っているのは自分」という札。Claude デスクトップがこのサーバーを
	// 2 つ同時に動かすことがあるので、つなぐのは札を取れた 1 つだけにする（PC の中だけで開く）。
	holdPort = 60204
	logKeep  = 30000 // ログを何行まで覚えておくか
)

type logLine struct {
	at   time.Time
	text string
}

type Phone struct {
	mu sync.Mutex

	pairDir string // トンネルの鍵（selfIdentity.plist など）を置く場所
	wdaTest string // WebDriverAgent のテスト本体の名前（SideStore 向けは .xctest なし）

	device ios.DeviceEntry // USB（usbmux）経由
	rsd    ios.DeviceEntry // トンネル経由（iOS 17+ の開発者サービス用）
	hasRSD bool

	cancel    context.CancelFunc
	wdaCancel context.CancelFunc
	tm        *tunnel.TunnelManager

	logMu sync.Mutex
	logs  []logLine

	wdaRunning bool
	wdaErr     string
	wdaBundle  string
	session    string
	http       *http.Client

	lcBundle string
	appNames map[string]string // bundle id → 名前（つないだあと最初に要ったとき読む）
	appExec  map[string]string // bundle id → 実行ファイルの名前（動いているものの一覧と合わせる）

	logDir string // ログを PC にも残す場所
	logF   *logFile

	hold net.Listener // holdPort の札
}

func NewPhone(pairDir, wdaTest, logDir string) *Phone {
	return &Phone{pairDir: pairDir, wdaTest: wdaTest, logDir: logDir}
}

// WDAReady は WebDriverAgent が動いていてセッションがあるか（起こさずに見るだけ）。
func (p *Phone) WDAReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.wdaRunning && p.session != ""
}

// Connected はつながっているか（トンネルまで）。
func (p *Phone) Connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasRSD
}

// Disconnect は全部切る（スイッチがオフになったとき）。
func (p *Phone) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wdaCancel != nil {
		p.wdaCancel()
	}
	if p.cancel != nil {
		p.cancel()
	}
	if p.tm != nil {
		_ = p.tm.Close()
	}
	p.cancel, p.wdaCancel, p.tm = nil, nil, nil
	p.hasRSD, p.wdaRunning, p.session, p.wdaBundle = false, false, "", ""
	p.http = nil
	p.appNames, p.appExec = nil, nil
	p.logMu.Lock()
	p.logF.flush()
	p.logMu.Unlock()
	if p.hold != nil {
		_ = p.hold.Close()
		p.hold = nil
	}
	slog.Info("disconnected")
}

// Ensure はつながっていなければつなぐ（USB の端末 → ログ → トンネル）。
func (p *Phone) Ensure(ctx context.Context) (err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() {
		// つなげなかったら、途中まで持ったもの（札・ログ読み）を手放す
		if err != nil && !p.hasRSD {
			if p.cancel != nil {
				p.cancel()
				p.cancel = nil
			}
			if p.tm != nil {
				_ = p.tm.Close()
				p.tm = nil
			}
			if p.hold != nil {
				_ = p.hold.Close()
				p.hold = nil
			}
		}
	}()
	if p.hasRSD {
		return nil
	}
	if p.hold == nil {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", holdPort))
		if err != nil {
			return fmt.Errorf("別の iPhone の道具（Claude のほかの会話など）が iPhone を使っています。そちらを使い終わるか、30 分たつか、スイッチをいったんオフにすると使えます")
		}
		p.hold = l
		slog.Info("hold", "pid", os.Getpid())
	}
	list, err := ios.ListDevices()
	if err != nil {
		return fmt.Errorf("USB の iPhone を探せません（Apple のドライバーがあるか）：%w", err)
	}
	if len(list.DeviceList) == 0 {
		return fmt.Errorf("iPhone が見つかりません。USB でつなぎ、ロックを外してください")
	}
	dev := list.DeviceList[0]
	udid := dev.Properties.SerialNumber
	dev, err = ios.GetDevice(udid)
	if err != nil {
		return fmt.Errorf("iPhone の情報を取れません：%w", err)
	}
	p.device = dev

	bg, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	// ログは USB の経路で取る（トンネルとは別）
	go p.readLogs(bg, dev)

	pm, err := tunnel.NewPairRecordManager(p.pairDir)
	if err != nil {
		return fmt.Errorf("トンネルの鍵の場所（%s）を使えません：%w", p.pairDir, err)
	}
	tm := tunnel.NewTunnelManagerForDevice(pm, true, udid, tunnelBasePort)
	p.tm = tm
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-bg.Done():
				return
			case <-t.C:
				if err := tm.UpdateTunnels(bg); err != nil {
					slog.Warn("tunnel update", "error", err)
				}
			}
		}
	}()

	var info tunnel.Tunnel
	deadline := time.Now().Add(40 * time.Second)
	for {
		if err := tm.UpdateTunnels(bg); err != nil {
			slog.Warn("tunnel update", "error", err)
		}
		info, err = tm.FindTunnel(udid)
		if err == nil && info.Address != "" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("トンネルを張れません（40 秒）：%v", err)
		}
		time.Sleep(time.Second)
	}
	d := dev
	d.UserspaceTUN = info.UserspaceTUN
	d.UserspaceTUNPort = info.UserspaceTUNPort
	d.UserspaceTUNHost = ios.HttpApiHost()
	rsdSvc, err := ios.NewWithAddrPortDevice(info.Address, info.RsdPort, d)
	if err != nil {
		return fmt.Errorf("トンネルの先（RSD）につなげません：%w", err)
	}
	provider, err := rsdSvc.Handshake()
	rsdSvc.Close()
	if err != nil {
		return fmt.Errorf("RSD の握手に失敗：%w", err)
	}
	rd, err := ios.GetDeviceWithAddress(udid, info.Address, provider)
	if err != nil {
		return fmt.Errorf("トンネル経由の端末を作れません：%w", err)
	}
	rd.UserspaceTUN, rd.UserspaceTUNPort, rd.UserspaceTUNHost = d.UserspaceTUN, d.UserspaceTUNPort, d.UserspaceTUNHost
	p.rsd = rd
	p.hasRSD = true
	slog.Info("connected", "tunnel", info.Address)
	return nil
}

func (p *Phone) readLogs(ctx context.Context, dev ios.DeviceEntry) {
	for ctx.Err() == nil {
		conn, err := syslog.New(dev)
		if err != nil {
			slog.Warn("syslog", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for ctx.Err() == nil {
			msg, err := conn.ReadLogMessage()
			if err != nil {
				slog.Warn("syslog read", "error", err)
				break
			}
			msg = strings.TrimRight(msg, "\x00\r\n")
			if msg == "" {
				continue
			}
			now := time.Now()
			p.logMu.Lock()
			if p.logF == nil && p.logDir != "" {
				p.logF = newLogFile(p.logDir)
			}
			p.logF.write(now, msg)
			p.logs = append(p.logs, logLine{at: now, text: msg})
			if len(p.logs) > logKeep {
				p.logs = append([]logLine(nil), p.logs[len(p.logs)-logKeep*2/3:]...)
			}
			p.logMu.Unlock()
		}
		conn.Close()
	}
}

// Logs は、ここ sinceSec 秒のログから正規表現に合う行を返す。waitSec > 0 なら、合う行が出るまで待つ。
func (p *Phone) Logs(ctx context.Context, pattern string, sinceSec, waitSec, max int) ([]string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正規表現が正しくありません：%w", err)
	}
	from := time.Now().Add(-time.Duration(sinceSec) * time.Second)
	until := time.Now().Add(time.Duration(waitSec) * time.Second)
	// 覚えている分より前を見たいときは、PC に残したファイルから探す
	var older []string
	p.logMu.Lock()
	oldest := time.Now()
	if len(p.logs) > 0 {
		oldest = p.logs[0].at
	}
	p.logF.flush()
	p.logMu.Unlock()
	if from.Before(oldest) && p.logDir != "" {
		older = searchLogFiles(p.logDir, re, from, oldest, max)
	}
	for {
		out := append([]string(nil), older...)
		p.logMu.Lock()
		for _, l := range p.logs {
			if l.at.Before(from) || !re.MatchString(l.text) {
				continue
			}
			out = append(out, l.at.Format("15:04:05.000")+" "+l.text)
		}
		p.logMu.Unlock()
		if len(out) > 0 || time.Now().After(until) || ctx.Err() != nil {
			if len(out) > max {
				out = out[len(out)-max:]
			}
			return out, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// findApps は WebDriverAgent と LiveContainer の bundle id を探す（一覧は外に出さない）。
func (p *Phone) findApps() error {
	conn, err := installationproxy.New(p.device)
	if err != nil {
		return err
	}
	defer conn.Close()
	apps, err := conn.BrowseUserApps()
	if err != nil {
		return err
	}
	for _, a := range apps {
		id := a.CFBundleIdentifier()
		if strings.Contains(id, "WebDriverAgentRunner") {
			p.wdaBundle = id
		}
		if strings.HasPrefix(id, "com.kdt.livecontainer") {
			p.lcBundle = id
		}
	}
	return nil
}

// LiveContainer の bundle id（見つからなければ空）。
func (p *Phone) LiveContainer() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lcBundle == "" && p.hasRSD {
		_ = p.findApps()
	}
	return p.lcBundle
}

// EnsureWDA は WebDriverAgent を起動してセッションを作る（起動済みなら何もしない）。
func (p *Phone) EnsureWDA(ctx context.Context) error {
	if err := p.Ensure(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	if p.wdaRunning && p.session != "" {
		if _, err := p.wdaCallLocked("GET", "/status", nil); err == nil {
			p.mu.Unlock()
			return nil
		}
		p.wdaRunning, p.session = false, ""
	}
	if p.wdaBundle == "" {
		if err := p.findApps(); err != nil {
			p.mu.Unlock()
			return fmt.Errorf("アプリの一覧を取れません：%w", err)
		}
		if p.wdaBundle == "" {
			p.mu.Unlock()
			return fmt.Errorf("WebDriverAgent が入っていません（SideStore で WebDriverAgentRunner-ss.ipa を入れる）")
		}
	}
	// HTTP は USB の経路で直接 WDA（8100 番）へ。PC にポートを開けない
	deviceID := p.device.DeviceID
	p.http = &http.Client{
		Timeout: 90 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				a, b := net.Pipe()
				go func() { _ = forward.StartNewProxyConnection(ctx, b, deviceID, wdaPort) }()
				return a, nil
			},
			DisableKeepAlives: true,
		},
	}
	p.wdaErr = ""
	bundle, test, rsd := p.wdaBundle, p.wdaTest, p.rsd
	if p.wdaCancel != nil {
		p.wdaCancel()
	}
	runCtx, wdaCancel := context.WithCancel(context.Background())
	p.wdaCancel = wdaCancel
	go func() {
		_, err := testmanagerd.RunTestWithConfig(runCtx, testmanagerd.TestConfig{
			BundleId:           bundle,
			TestRunnerBundleId: bundle,
			XctestConfigName:   test,
			Env:                map[string]any{},
			Device:             rsd,
			Listener:           testmanagerd.NewTestListener(io.Discard, io.Discard, os.TempDir()),
		})
		p.mu.Lock()
		p.wdaRunning, p.session = false, ""
		if err != nil {
			p.wdaErr = err.Error()
		}
		p.mu.Unlock()
		slog.Info("wda ended", "error", err)
	}()
	p.wdaRunning = true
	p.mu.Unlock()

	deadline := time.Now().Add(60 * time.Second)
	for {
		p.mu.Lock()
		_, err := p.wdaCallLocked("GET", "/status", nil)
		werr := p.wdaErr
		p.mu.Unlock()
		if err == nil {
			break
		}
		if werr != "" {
			return fmt.Errorf("WebDriverAgent が起動できません：%s", werr)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("WebDriverAgent が返事をしません（60 秒）")
		}
		time.Sleep(time.Second)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	r, err := p.wdaCallLocked("POST", "/session", map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{}, "firstMatch": []any{map[string]any{}}}})
	if err != nil {
		return fmt.Errorf("WebDriverAgent のセッションを作れません：%w", err)
	}
	var s struct {
		SessionID string `json:"sessionId"`
		Value     struct {
			SessionID string `json:"sessionId"`
		} `json:"value"`
	}
	_ = json.Unmarshal(r, &s)
	p.session = s.SessionID
	if p.session == "" {
		p.session = s.Value.SessionID
	}
	if p.session == "" {
		return fmt.Errorf("セッションの番号を取れません")
	}
	return nil
}

func (p *Phone) wdaCallLocked(method, path string, body any) ([]byte, error) {
	if p.http == nil {
		return nil, fmt.Errorf("WebDriverAgent につながっていません")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://wda"+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		s := string(b)
		if len(s) > 400 {
			s = s[:400] + "…"
		}
		return b, fmt.Errorf("WDA %s %s：%d %s", method, path, res.StatusCode, s)
	}
	return b, nil
}

// WDA はセッション付きの呼び出し。path の {s} をセッション番号に置き換える。
func (p *Phone) WDA(ctx context.Context, method, path string, body any) ([]byte, error) {
	if err := p.EnsureWDA(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.wdaCallLocked(method, strings.ReplaceAll(path, "{s}", p.session), body)
}

func (p *Phone) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logMu.Lock()
	n := len(p.logs)
	p.logMu.Unlock()
	return map[string]any{
		"つながっている":        p.hasRSD,
		"WebDriverAgent": p.wdaRunning && p.session != "",
		"WDA の最後のエラー":    p.wdaErr,
		"覚えているログの行数":     n,
		"LiveContainer":  p.lcBundle != "",
	}
}
