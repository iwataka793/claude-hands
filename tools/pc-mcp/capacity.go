package main

// 容量の決まり（docs/ASK-DESIGN.md「容量の決まり」）：claude-hands が自分で作ったものに上限を決め、
// 上限に来たら道具が自分で片付ける。カオルのファイルは消さない（知らせるだけ）。
//   常駐役が動き出したとき・動いている間は 1 日 1 回、片付けて結果を devkit\hands\status\capacity.txt に書く。
//   pc_status はその「知らせ」をいちばん上に出す。
//   管理者の権限が要るもの（復元ポイント・取り置きの期限）は SYSTEM の点検（hands-daily.ps1）のまま。

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	mib           = int64(1) << 20
	recordDays    = 90        // TIMELINE・DONE の報告・頼みの記録
	recordCap     = 200 * mib // 合わせて
	chromeCap     = 1 * gib   // Claude 用の Chrome
	webviewCap    = 200 * mib // 窓の中身（WebView2）1 つあたり
	downloadDays  = 14        // Claude が落としたもの（devkit\downloads）
	tempHours     = 24        // 道具の一時ファイル（%TEMP%\claude-hands*）
	gitCap        = 500 * mib // Git の控え
	gitGCDays     = 7
	handsTotalCap = 20 * gib // claude-hands が C で使う分の合計の目安
	capEvery      = 20 * time.Hour
)

func capacityFile() string { return filepath.Join(handsUserDir(), "status", "capacity.txt") }
func gitGCStamp() string   { return filepath.Join(handsUserDir(), "status", "git-gc.txt") }
func downloadsDir() string { return filepath.Join(homeDir(), "devkit", "downloads") }
func localHandsDir() string {
	return env("PC_HANDS_LOCALDIR", filepath.Join(os.Getenv("LOCALAPPDATA"), "claude-hands"))
}

// capacityDue：前の点検から capEvery たったか（無ければ今やる）
func capacityDue() bool {
	st, err := os.Stat(capacityFile())
	return err != nil || time.Since(st.ModTime()) > capEvery
}

// sizeOf：フォルダかファイルの大きさ（つなぎ目の先は数えない。クラウドだけのファイルは数えない）
func sizeOf(p string) int64 {
	st, err := os.Lstat(p)
	if err != nil {
		return 0
	}
	if !st.IsDir() {
		if isCloudOnly(st) {
			return 0
		}
		return st.Size()
	}
	var n int64
	_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && q != p && isReparse(d) {
			return fs.SkipDir
		}
		if !d.IsDir() {
			if fi, e := d.Info(); e == nil && !isCloudOnly(fi) {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func mb(n int64) string {
	if n >= gib {
		return gb(n)
	}
	if n > 0 && n < mib {
		return "1 MB 未満"
	}
	return fmt.Sprintf("%d MB", (n+mib/2)/mib)
}

type capResult struct {
	done    []string // 自動で片付けたもの
	notes   []string // カオルに知らせること
	sizes   []string // 置き場所ごとの大きさ
	errs    []string
	removed int64
}

func (r *capResult) did(f string, a ...any)  { r.done = append(r.done, fmt.Sprintf(f, a...)) }
func (r *capResult) note(f string, a ...any) { r.notes = append(r.notes, fmt.Sprintf(f, a...)) }

// ---- 記録（TIMELINE・DONE・asks.jsonl）：90 日・合わせて 200 MB ----

type ageFile struct {
	path string
	when time.Time
	size int64
}

func pruneRecords(r *capResult, now time.Time) {
	var files []ageFile
	cut := now.AddDate(0, 0, -recordDays)
	for _, dir := range []string{timelineDir(), doneDir()} {
		es, _ := os.ReadDir(dir)
		for _, e := range es {
			if e.IsDir() {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			when := fi.ModTime()
			if t, err := time.ParseInLocation("2006-01-02", strings.TrimSuffix(e.Name(), ".jsonl"), time.Local); err == nil {
				when = t.Add(24 * time.Hour) // その日の分は、その日が終わってから数える
			}
			files = append(files, ageFile{filepath.Join(dir, e.Name()), when, fi.Size()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].when.Before(files[j].when) })
	n := 0
	var keep []ageFile
	for _, f := range files {
		if f.when.Before(cut) {
			if os.Remove(f.path) == nil {
				n++
				r.removed += f.size
				continue
			}
		}
		keep = append(keep, f)
	}
	if n > 0 {
		r.did("%d 日より古い記録（TIMELINE・DONE）を %d 件消した", recordDays, n)
	}
	// 頼みの記録：90 日より古い行を落とす
	asks := filepath.Join(handsUserDir(), "asks.jsonl")
	if dropped := pruneJSONL(asks, cut); dropped > 0 {
		r.did("頼みの記録（asks.jsonl）の %d 日より古い %d 行を消した", recordDays, dropped)
	}
	var total int64
	for _, f := range keep {
		total += f.size
	}
	total += sizeOf(asks)
	m := 0
	for i := 0; total > recordCap && i < len(keep); i++ {
		if strings.HasPrefix(filepath.Base(keep[i].path), now.Format("2006-01-02")) {
			continue // 今日の分は残す
		}
		if os.Remove(keep[i].path) == nil {
			total -= keep[i].size
			r.removed += keep[i].size
			m++
		}
	}
	if m > 0 {
		r.did("記録が %s を超えたので、古いものから %d 件消した", mb(recordCap), m)
	}
	if total > recordCap {
		r.note("記録（TIMELINE・DONE・頼み）が %s あり、上限 %s を超えている（今日の分だけで超えている）", mb(total), mb(recordCap))
	}
	r.sizes = append(r.sizes, fmt.Sprintf("記録（TIMELINE・DONE・頼み）：%s／%s・%d 日", mb(total), mb(recordCap), recordDays))
}

// pruneJSONL："at"（か "t"）が cut より古い行を落として書き直す。落とした行の数
func pruneJSONL(path string, cut time.Time) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	var keep []string
	drop := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		l := sc.Text()
		var m struct {
			At time.Time `json:"at"`
			T  time.Time `json:"t"`
		}
		if json.Unmarshal([]byte(l), &m) == nil {
			at := m.At
			if at.IsZero() {
				at = m.T
			}
			if !at.IsZero() && at.Before(cut) {
				drop++
				continue
			}
		}
		keep = append(keep, l)
	}
	f.Close()
	if drop == 0 {
		return 0
	}
	tmp := path + ".tmp"
	body := strings.Join(keep, "\r\n")
	if body != "" {
		body += "\r\n"
	}
	if os.WriteFile(tmp, []byte(body), 0o644) != nil || os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
		return 0
	}
	return drop
}

// ---- Chromium の中身（Claude 用の Chrome・窓の WebView2） ----

var chromeCacheDirs = []string{
	"Cache", "Code Cache", "GPUCache", "DawnGraphiteCache", "DawnWebGPUCache", "GrShaderCache", "GraphiteDawnCache", "ShaderCache",
	filepath.Join("Default", "Cache"), filepath.Join("Default", "Code Cache"), filepath.Join("Default", "GPUCache"),
	filepath.Join("Default", "DawnGraphiteCache"), filepath.Join("Default", "DawnWebGPUCache"),
	filepath.Join("Default", "Service Worker", "CacheStorage"), filepath.Join("Default", "Service Worker", "ScriptCache"),
}

// chromiumIdle：その中身を使う Chrome が動いていないか（Windows の Chrome は lockfile を開いたままにする。消せれば動いていない）
func chromiumIdle(dir string) bool {
	lf := filepath.Join(dir, "lockfile")
	if _, err := os.Stat(lf); err != nil {
		return true
	}
	return os.Remove(lf) == nil
}

// clearChromeCache：キャッシュだけ消す（ログイン・設定は残る）。消した大きさ
func clearChromeCache(dir string) int64 {
	var n int64
	for _, c := range chromeCacheDirs {
		p := filepath.Join(dir, c)
		s := sizeOf(p)
		if s > 0 && os.RemoveAll(p) == nil {
			n += s
		}
	}
	return n
}

func capChrome(r *capResult) {
	dir := claudeChromeDir()
	if _, err := os.Stat(dir); err != nil {
		r.sizes = append(r.sizes, "Claude 用の Chrome：なし／"+mb(chromeCap))
		return
	}
	if chromiumIdle(dir) {
		if n := clearChromeCache(dir); n > 0 {
			r.removed += n
			r.did("Claude 用の Chrome のキャッシュを消した（%s）", mb(n))
		}
		if s := sizeOf(dir); s > chromeCap {
			// ログインは入っていないので、作り直しても失うものは無い（見た目の設定は次に起動するときに入れ直す）
			if os.RemoveAll(dir) == nil {
				r.removed += s
				r.did("Claude 用の Chrome が %s あり上限 %s を超えたので、中身を作り直した", mb(s), mb(chromeCap))
			} else {
				r.note("Claude 用の Chrome が %s あり、上限 %s を超えているが消せなかった", mb(s), mb(chromeCap))
			}
		}
	}
	r.sizes = append(r.sizes, fmt.Sprintf("Claude 用の Chrome：%s／%s", mb(sizeOf(dir)), mb(chromeCap)))
}

func capWebviews(r *capResult) {
	for _, w := range []struct{ name, dir string }{{"SWITCH の窓", "switch-webview"}, {"LIVE の窓", "live-webview"}} {
		dir := filepath.Join(localHandsDir(), w.dir)
		s := sizeOf(dir)
		if s > webviewCap {
			// 窓は動いたままのことが多い。使っている最中のファイルは消せずに残るだけ
			if n := clearChromeCache(filepath.Join(dir, "EBWebView")); n > 0 {
				r.removed += n
				r.did("%sの中身（WebView2）のキャッシュを消した（%s）", w.name, mb(n))
				s = sizeOf(dir)
			}
			if s > webviewCap {
				r.note("%sの中身（WebView2）が %s あり、上限 %s を超えている", w.name, mb(s), mb(webviewCap))
			}
		}
		r.sizes = append(r.sizes, fmt.Sprintf("%sの中身：%s／%s", w.name, mb(s), mb(webviewCap)))
	}
}

// ---- 一時ファイル（%TEMP% の claude-hands の印つき）：1 日 ----

func capTemp(r *capResult, now time.Time) {
	tmp := os.TempDir()
	es, _ := os.ReadDir(tmp)
	n := 0
	for _, e := range es {
		if !strings.Contains(strings.ToLower(e.Name()), "claude-hands") {
			continue
		}
		fi, err := e.Info()
		if err != nil || now.Sub(fi.ModTime()) < tempHours*time.Hour {
			continue
		}
		p := filepath.Join(tmp, e.Name())
		s := sizeOf(p)
		if os.RemoveAll(p) == nil {
			n++
			r.removed += s
		}
	}
	if n > 0 {
		r.did("道具の一時ファイル（1 日より古い）を %d 件消した", n)
	}
}

// ---- Claude が落としたもの（devkit\downloads）：14 日たったら取り置きへ ----

func (a *Agent) capDownloads(ctx context.Context, r *capResult, now time.Time) {
	dir := downloadsDir()
	_ = os.MkdirAll(dir, 0o755)
	es, _ := os.ReadDir(dir)
	var old []string
	for _, e := range es {
		fi, err := e.Info()
		if err == nil && now.Sub(fi.ModTime()) > downloadDays*24*time.Hour {
			old = append(old, filepath.Join(dir, e.Name()))
		}
	}
	if len(old) > 0 && a != nil && a.ps != nil {
		rep := a.trash(ctx, mustJSON(map[string]any{"paths": old}))
		ok := strings.Count(rep.Text, "消した（取り置き")
		if ok > 0 {
			r.did("Claude が落としたもの（%d 日より古い）を %d 件、取り置きへ送った", downloadDays, ok)
		}
		if strings.Contains(rep.Text, "ERROR") {
			r.note("Claude が落としたもののうち、取り置きへ送れなかったものがある（%s）", oneLine(rep.Text, 120))
		}
	}
	r.sizes = append(r.sizes, fmt.Sprintf("Claude が落としたもの（devkit\\downloads）：%s・%d 日", mb(sizeOf(dir)), downloadDays))
}

// ---- Git の控え：週 1 回まとめる・500 MB ----

func capGit(ctx context.Context, r *capResult, now time.Time) {
	if _, err := os.Stat(gitDir()); err != nil {
		return
	}
	last := time.Time{}
	if st, err := os.Stat(gitGCStamp()); err == nil {
		last = st.ModTime()
	}
	if now.Sub(last) > gitGCDays*24*time.Hour && gitExe() != "" {
		before := sizeOf(gitDir())
		gctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		_, err := gitRaw(gctx, "gc", "--quiet")
		cancel()
		if err != nil {
			r.errs = append(r.errs, "Git をまとめられなかった："+oneLine(err.Error(), 120))
		} else {
			_ = os.WriteFile(gitGCStamp(), []byte(now.Format(time.RFC3339)), 0o644)
			if after := sizeOf(gitDir()); before > after {
				r.removed += before - after
				r.did("Git の控えをまとめた（%s → %s）", mb(before), mb(after))
			}
		}
	}
	s := sizeOf(gitDir())
	if s > gitCap {
		r.note("Git の控えが %s あり、上限 %s を超えている（古い記録を 1 つにまとめる）", mb(s), mb(gitCap))
	}
	r.sizes = append(r.sizes, fmt.Sprintf("Git の控え：%s／%s", mb(s), mb(gitCap)))
}

// ---- 知らせること（C の空き・バックアップの古さ・合計） ----

func capChecks(r *capResult, now time.Time) {
	free := cFree()
	switch {
	case free < 50*gib:
		r.note("C の空きが %s（50 GB を切った）。新しい取り置きを止めている。消すものはカオルが決める", gb(free))
	case free < 100*gib:
		r.note("C の空きが %s（100 GB を切った）。取り置きの上限を 5 GB にした", gb(free))
	}
	if l := lastLine(filepath.Join(homeDir(), "devkit", "backup", "copy-onedrive.log"), "done "); l != "" {
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.TrimSpace(strings.TrimPrefix(l, "done ")), time.Local); err == nil && now.Sub(t) > 24*time.Hour {
			r.note("OneDrive への写しが止まっている（最後 %s）", t.Format("01-02 15:04"))
		}
	} else if _, err := os.Stat(filepath.Join(homeDir(), "devkit", "backup")); err == nil {
		r.note("OneDrive への写しの記録が無い")
	}
	if b, err := os.ReadFile(filepath.Join(handsDir(), "status", "daily.txt")); err == nil {
		for _, l := range strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), "\ufeff"), "\r\n", "\n"), "\n") {
			if v, ok := strings.CutPrefix(l, "最後の復元ポイント："); ok && len(v) >= 16 {
				if t, err := time.ParseInLocation("2006-01-02 15:04", v[:16], time.Local); err == nil && now.Sub(t) > 72*time.Hour {
					r.note("復元ポイントが古い（最後 %s）", t.Format("01-02 15:04"))
				}
			}
		}
	}
	// claude-hands が C で使う分の合計（復元ポイントと iPhone のバックアップを除く）
	var total int64
	for _, p := range []string{handsUserDir(), localHandsDir(), handsDir(), filepath.Join(homeDir(), "devkit", "pc-agent"),
		filepath.Join(homeDir(), "devkit", "iphone-mcp", "logs"), downloadsDir(), filepath.Join(homeDir(), "devkit", "installers")} {
		total += sizeOf(p)
	}
	if total > handsTotalCap {
		r.note("claude-hands が C で使う分が %s（目安 %s を超えた）", gb(total), gb(handsTotalCap))
	}
	r.sizes = append([]string{fmt.Sprintf("claude-hands の合計：%s／目安 %s", mb(total), gb(handsTotalCap))}, r.sizes...)
	// OneDrive：PC に落ちている分（クラウドだけのものは数えない）
	if od := os.Getenv("OneDrive"); od != "" {
		r.sizes = append(r.sizes, "OneDrive の PC に落ちている分："+mb(sizeOf(od)))
		capOneDrive(r, now, od)
	}
}

// capOneDrive：OneDrive の契約。使用量は PC の中に無い（取るには設定の窓を開いて読むしかない）ので、
// 有料（1.1 TB）の間は 80% に届かない前提で、契約が切れて無料の 5 GB に落ちることを見る（2026-11-28 カオルに伝えて決めた）
func capOneDrive(r *capResult, now time.Time, od string) {
	v := oneDriveAccount()
	if len(v) == 0 {
		return
	}
	backup := logicalSize(filepath.Join(od, "PC-Backup"))
	paid := regNum(v["LastKnownIsPaidUser"]) == 1
	state := v["LastKnownSubscriptionState"]
	exp := time.Time{}
	if n := regNum(v["LastKnownSubscriptionExpiry"]); n > 0 {
		exp = time.Unix(n, 0)
	}
	auto := regNum(v["LastKnownSubscriptionAutoRenew"]) == 1
	line := "OneDrive の契約："
	switch {
	case !paid:
		line += "無料（5 GB）"
		if backup > 4*gib {
			r.note("OneDrive が無料（5 GB）で、PC-Backup の写しが %s ある（あふれるおそれ）", gb(backup))
		}
	case state != "" && state != "Active":
		line += "有料・" + state
		r.note("OneDrive の契約が「%s」になっている（有効でない）", state)
	default:
		line += "有料・有効"
	}
	if !exp.IsZero() {
		line += "・次の期限 " + exp.Format("2006-01-02")
		if auto {
			line += "（自動更新）"
		} else {
			line += "（自動更新なし）"
			if paid && exp.Sub(now) < 14*24*time.Hour {
				r.note("OneDrive の契約が %s で切れる（自動更新なし。切れると無料の 5 GB に落ちる）", exp.Format("01-02"))
			}
		}
	}
	r.sizes = append(r.sizes, line+"・PC-Backup の写し "+mb(backup))
}

// oneDriveAccount：レジストリの OneDrive の個人用アカウントの値（reg query の出力から。値は REG_SZ はそのまま、REG_DWORD は 0x..）
func oneDriveAccount() map[string]string {
	cmd := exec.Command("reg", "query", `HKCU\Software\Microsoft\OneDrive\Accounts\Personal`)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseRegQuery(string(out))
}

func parseRegQuery(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && strings.HasPrefix(f[1], "REG_") {
			m[f[0]] = strings.Join(f[2:], " ")
		}
	}
	return m
}

// regNum：reg query の値を数に（REG_DWORD・REG_QWORD は 0x..、REG_SZ は 10 進）。読めなければ -1
func regNum(v string) int64 {
	v = strings.TrimSpace(v)
	base := 10
	if strings.HasPrefix(v, "0x") {
		v, base = v[2:], 16
	}
	n, err := strconv.ParseInt(v, base, 64)
	if err != nil {
		return -1
	}
	return n
}

// logicalSize：クラウドだけのファイルも含めた大きさ（OneDrive の側で使う大きさ）
func logicalSize(p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && q != p && isReparse(d) {
			return fs.SkipDir
		}
		if !d.IsDir() {
			if fi, e := d.Info(); e == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// capacityRun：全部の置き場所を測って、上限を超えたものを片付け、結果を status に書く
func (a *Agent) capacityRun(ctx context.Context) []string {
	now := time.Now()
	r := &capResult{}
	pruneRecords(r, now)
	capTemp(r, now)
	capChrome(r)
	capWebviews(r)
	a.capDownloads(ctx, r, now)
	capGit(ctx, r, now)
	capChecks(r, now)
	out := []string{"時刻：" + now.Format("2006-01-02 15:04")}
	if len(r.notes) == 0 {
		out = append(out, "知らせ：なし")
	} else {
		for _, n := range r.notes {
			out = append(out, "知らせ："+n)
		}
	}
	if len(r.done) == 0 {
		out = append(out, "片付け：なし")
	} else {
		for _, d := range r.done {
			out = append(out, "片付け："+d)
		}
	}
	for _, s := range r.sizes {
		out = append(out, "大きさ："+s)
	}
	for _, e := range r.errs {
		out = append(out, "うまくいかなかった："+e)
	}
	_ = os.MkdirAll(filepath.Dir(capacityFile()), 0o755)
	_ = os.WriteFile(capacityFile(), []byte(strings.Join(out, "\r\n")+"\r\n"), 0o644)
	return out
}

// capacityLines：pc_status に出す行（知らせは前に、ほかは後ろに）
func capacityLines() (notes, rest []string) {
	b, err := os.ReadFile(capacityFile())
	if err != nil {
		return nil, []string{"容量の点検：まだ（常駐役が動いている間に 1 日 1 回）"}
	}
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "時刻："):
			rest = append(rest, "容量の点検："+strings.TrimPrefix(l, "時刻："))
		case strings.HasPrefix(l, "知らせ："):
			if v := strings.TrimPrefix(l, "知らせ："); v != "なし" {
				notes = append(notes, "【知らせ】"+v)
			}
		case strings.HasPrefix(l, "片付け："):
			if v := strings.TrimPrefix(l, "片付け："); v != "なし" {
				rest = append(rest, "  片付けた："+v)
			}
		case strings.HasPrefix(l, "大きさ："):
			rest = append(rest, "  "+strings.TrimPrefix(l, "大きさ："))
		case strings.HasPrefix(l, "うまくいかなかった："):
			rest = append(rest, "  "+l)
		}
	}
	return notes, rest
}
