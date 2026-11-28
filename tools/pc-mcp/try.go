package main

// 箱で試す（B の道。docs/ASK-DESIGN.md の 2「箱」・docs/PC.md「箱」）。
//
//   run     ：devkit\downloads のものを Defender で検査 → 見つかれば止める → ClaudeBox の中で動かす
//             → 箱の中で増えた・変わった・消えたファイルをまとめる → TRY の窓（LIVE の窓の中）に出す
//   status  ：いまの試しと、カオルの返事
//   discard ：捨てる（箱を空にする）。Claude もカオルも使える（戻せないものは何も無い）
//   apply   ：本物に入れる。TRY の窓（カオル）だけ：窓が合言葉（request.key）で署名して頼む。Claude からは呼べない
//
// 箱の中身の見方（2026-11-28 に記録で確かめた）：
//   箱の根 C:\Sandbox\<ユーザー>\ClaudeBox の下の drive\C\… が C:\…、user\current\… が %USERPROFILE%\…、user\all\… が %ProgramData%\…。
//   本物のファイルを変えると、写しが箱の中にできる。消すと、大きさ 0・作成日時 1986-05-24 の印のファイルになる。
//   根の直下の RegHive などはレジストリ（入れない）。PowerShell や Windows が勝手に作るもの（CatRoot2 など）は数えるだけ。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const tryBox = "ClaudeBox"

var tryMu sync.Mutex

func sbieDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "Sandboxie-Plus")
}

func boxRoot() string {
	if v := os.Getenv("HANDS_BOX_ROOT"); v != "" {
		return v
	}
	return filepath.Join(systemDrive(), "Sandbox", os.Getenv("USERNAME"), tryBox)
}

func tryDir() string       { return filepath.Join(handsUserDir(), "try") }
func tryStateFile() string { return filepath.Join(tryDir(), "current.json") }

type tryItem struct {
	Kind string `json:"kind"` // new・changed・deleted・admin（管理者の場所。入れない）
	Real string `json:"real"`
	Size int64  `json:"size"`
}

type tryState struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Args     []string  `json:"args,omitempty"`
	Started  time.Time `json:"started"`
	Ended    time.Time `json:"ended"`
	Defender string    `json:"defender"`
	Running  bool      `json:"running"` // 待ち時間を過ぎても箱の中で動いている
	Exit     string    `json:"exit,omitempty"`
	Items    []tryItem `json:"items"`
	Noise    int       `json:"noise"`    // Windows・PowerShell が勝手に作るもの（入れない）
	Registry bool      `json:"registry"` // レジストリの変更（箱の中だけ。入れない）
	Status   string    `json:"status"`   // waiting・applied・discarded
	Result   string    `json:"result,omitempty"`
	By       string    `json:"by,omitempty"`
	At       time.Time `json:"at,omitempty"`
}

func loadTry() (*tryState, error) {
	b, err := os.ReadFile(tryStateFile())
	if err != nil {
		return nil, err
	}
	var s tryState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveTry(s *tryState) error {
	_ = os.MkdirAll(tryDir(), 0o755)
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := tryStateFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, tryStateFile())
}

// ---- 署名（TRY の窓だけが「本物に入れる」を頼める） ----

func trySignText(id, action string) string {
	return strings.Join([]string{"claude-hands-try-v1", id, action}, "\n")
}

func trySigOK(id, action, sig string) bool {
	key, err := os.ReadFile(requestKeyFile())
	k := strings.TrimSpace(string(key))
	if err != nil || k == "" || sig == "" {
		return false
	}
	m := hmac.New(sha256.New, []byte(k))
	m.Write([]byte(trySignText(id, action)))
	return hmac.Equal([]byte(hex.EncodeToString(m.Sum(nil))), []byte(sig))
}

// ---- 箱の中身を本物の場所に読み替える ----

// 勝手にできるもの（入れない・数えるだけ）。箱の根からの場所（小文字・\ 区切り）
var tryNoise = []string{
	`drive\c\windows\system32\catroot2\`,
	`user\current\appdata\roaming\microsoft\windows\recent\`,
	`user\current\appdata\local\microsoft\windows\powershell\`,
	`user\current\appdata\local\microsoft\windows\inetcache\`,
	`user\current\appdata\local\microsoft\windows\webcache\`,
	`user\current\appdata\local\microsoft\windows\explorer\`,
	`user\current\appdata\local\microsoft\windows\caches\`,
	`user\current\appdata\local\temp\`,
	`user\current\appdata\local\packages\`,
	`user\current\appdata\local\connecteddevicesplatform\`,
}

func boxToReal(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return ""
	}
	rest := filepath.Join(parts[min(2, len(parts)):]...)
	switch {
	case strings.EqualFold(parts[0], "drive") && len(parts[1]) == 1 && len(parts) > 2:
		return strings.ToUpper(parts[1]) + `:\` + rest
	case strings.EqualFold(parts[0], "user") && strings.EqualFold(parts[1], "current"):
		return filepath.Join(homeDir(), rest)
	case strings.EqualFold(parts[0], "user") && strings.EqualFold(parts[1], "all"):
		return filepath.Join(programData(), rest)
	}
	return ""
}

func programData() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return pd
	}
	return `C:\ProgramData`
}

// adminPlace：管理者しか書けない場所（本物に入れるには、本物でもう一度入れ直す。UAC）
func adminPlace(real string) bool {
	l := strings.ToLower(matchForm(real))
	return regexpAdmin.MatchString(l)
}

var regexpAdmin = regexp.MustCompile(`^[a-z]:\\(windows|program files|program files \(x86\)|programdata)(\\|$)`)

// deletedMark：箱の中で消したことの印（大きさ 0・作成日時 1986 年）
func deletedMark(fi fs.FileInfo) bool {
	return !fi.IsDir() && fi.Size() == 0 && createdAt(fi).Year() == 1986
}

// scanBox：箱の中身をまとめる（本物と同じものは数えない）
func scanBox() (items []tryItem, noise int, registry bool, err error) {
	root := boxRoot()
	if _, e := os.Stat(root); e != nil {
		return nil, 0, false, nil
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		low := strings.ToLower(strings.ReplaceAll(rel, "/", `\`))
		if !strings.Contains(low, `\`) && !d.IsDir() { // 根の直下（RegHive など）
			if strings.HasPrefix(low, "reghive") {
				registry = true
			}
			return nil
		}
		if d.IsDir() {
			if isReparse(d) {
				return fs.SkipDir
			}
			return nil
		}
		for _, n := range tryNoise {
			if strings.HasPrefix(low+`\`, n) || strings.HasPrefix(low, n) {
				noise++
				return nil
			}
		}
		real := boxToReal(rel)
		if real == "" {
			noise++
			return nil
		}
		fi, e2 := d.Info()
		if e2 != nil {
			return nil
		}
		if deletedMark(fi) {
			if _, e3 := os.Stat(real); e3 == nil {
				items = append(items, tryItem{Kind: "deleted", Real: real})
			}
			return nil
		}
		kind := "new"
		if rf, e3 := os.Stat(real); e3 == nil {
			if rf.Size() == fi.Size() && sameFile(p, real) {
				return nil // 開いただけで同じ
			}
			kind = "changed"
		}
		if adminPlace(real) {
			kind = "admin"
		}
		items = append(items, tryItem{Kind: kind, Real: real, Size: fi.Size()})
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].Real < items[j].Real })
	return
}

func fileHash(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sameFile(a, b string) bool {
	ha := fileHash(a)
	return ha != "" && ha == fileHash(b)
}

// ---- Sandboxie と Defender を呼ぶ（道具の中から。台本の守りは通さない） ----

func sbie(ctx context.Context, d time.Duration, args ...string) error {
	_, err := sbieWait(ctx, d, args...)
	return err
}

// sbieWait：Start.exe を動かす。待ち時間を過ぎたら Start.exe だけ止めて timedOut（箱の中のものは動いたまま）
func sbieWait(ctx context.Context, d time.Duration, args ...string) (timedOut bool, err error) {
	exe := filepath.Join(sbieDir(), "Start.exe")
	if v := os.Getenv("HANDS_SBIE_START"); v != "" {
		exe = v
	}
	c, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(c, exe, args...)
	hideWindow(cmd)
	err = cmd.Run()
	if c.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		return true, nil
	}
	return false, err
}

func emptyBox(ctx context.Context) error {
	_ = sbie(ctx, 30*time.Second, "/box:"+tryBox, "/terminate")
	if err := sbie(ctx, 60*time.Second, "/box:"+tryBox, "delete_sandbox_silent"); err != nil {
		return err
	}
	if os.Getenv("HANDS_BOX_ROOT") != "" { // 試験：箱の代わりのフォルダ
		_ = os.RemoveAll(boxRoot())
	}
	if _, err := os.Stat(boxRoot()); err == nil {
		time.Sleep(2 * time.Second)
		if _, err := os.Stat(boxRoot()); err == nil {
			return errors.New("箱の中身が残った（箱の中で何かが動いている）")
		}
	}
	return nil
}

func mpCmdRun() string {
	if v := os.Getenv("HANDS_MPCMDRUN"); v != "" {
		return v
	}
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	p := filepath.Join(pd, "Microsoft", "Windows Defender", "Platform", "CurrentVersion", "MpCmdRun.exe")
	if fileExists(p) {
		return p
	}
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "Windows Defender", "MpCmdRun.exe")
}

// defenderScan：見つからなければ ""、見つかった・検査できなければ理由
func defenderScan(ctx context.Context, path string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(c, mpCmdRun(), "-Scan", "-ScanType", "3", "-File", path, "-DisableRemediation")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return "", fmt.Errorf("Defender の検査を動かせない：%v", err)
	}
	switch code {
	case 0:
		return "", nil
	case 2:
		return "Defender が見つけた：" + oneLine(lastNonEmpty(string(out)), 200), nil
	}
	return "", fmt.Errorf("Defender の検査が終わりのコード %d で止まった：%s", code, oneLine(lastNonEmpty(string(out)), 200))
}

func lastNonEmpty(s string) string {
	ls := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(ls[i]); t != "" {
			return t
		}
	}
	return ""
}

// inDownloads：devkit\downloads の中か
func inDownloads(p string) bool {
	d := strings.ToLower(matchForm(downloadsDir())) + `\`
	return strings.HasPrefix(strings.ToLower(matchForm(p)), d)
}

// ---- 道具 ----

func (a *Agent) try(ctx context.Context, raw json.RawMessage) Reply {
	var in struct {
		Action  string   `json:"action"`
		Path    string   `json:"path"`
		Args    []string `json:"args"`
		Seconds int      `json:"seconds"`
		ID      string   `json:"id"`
		Sig     string   `json:"sig"`
		By      string   `json:"by"`
	}
	_ = json.Unmarshal(raw, &in)
	tryMu.Lock()
	defer tryMu.Unlock()
	switch strings.ToLower(in.Action) {
	case "", "run":
		if a.st.Mode() != ModeOperate {
			return errText("pc_try（箱で試す）は「操作も可」のときだけ")
		}
		return a.tryRun(ctx, in.Path, in.Args, in.Seconds)
	case "status":
		return tryStatus()
	case "discard":
		if a.st.Mode() != ModeOperate {
			return errText("捨てるのは「操作も可」のときだけ。スイッチの窓で PC を「操作も可」にしてから、もう一度押してください")
		}
		by := "Claude"
		if in.By != "" && trySigOK(in.ID, "discard", in.Sig) {
			by = in.By
		}
		return a.tryDiscard(ctx, in.ID, by)
	case "apply":
		if !trySigOK(in.ID, "apply", in.Sig) {
			return errText("本物に入れるのは TRY の窓（カオルが押す）だけです。Claude からは入れられません。カオルに TRY の窓の［本物に入れる］を押してもらってください")
		}
		if a.st.Mode() != ModeOperate {
			return errText("PC が「操作も可」になっていません。スイッチの窓で PC を「操作も可」にしてから、もう一度押してください")
		}
		return a.tryApply(ctx, in.ID)
	}
	return errText("action は run・status・discard のどれか（本物に入れるのは TRY の窓だけ）")
}

func (a *Agent) tryRun(ctx context.Context, path string, args []string, secs int) Reply {
	if strings.TrimSpace(path) == "" {
		return errText("path（試すファイル。devkit\\downloads の中）を指定してください")
	}
	p := normPath(path)
	if !inDownloads(p) {
		return errText("箱で試せるのは devkit\\downloads（%s）の中のものだけ。ほかの場所のものは、カオルに聞いてからそこへ写す", downloadsDir())
	}
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return errText("ファイルがありません：%s", p)
	}
	if !fileExists(filepath.Join(sbieDir(), "Start.exe")) && os.Getenv("HANDS_SBIE_START") == "" {
		return errText("箱（Sandboxie-Plus）が入っていません")
	}
	if s, err := loadTry(); err == nil && s.Status == "waiting" {
		return errText("前の試し（%s）がカオルの返事待ちです（TRY の窓）。待つか、pc_try action=discard で捨ててから", filepath.Base(s.Path))
	}
	if secs <= 0 {
		secs = 60
	}
	if secs > 110 {
		secs = 110
	}
	// 前の箱の中身は Claude の試しの残りなので空にしてから
	if _, err := os.Stat(boxRoot()); err == nil {
		if err := emptyBox(ctx); err != nil {
			return errText("箱を空にできませんでした：%v", err)
		}
	}
	s := &tryState{ID: time.Now().Format("20060102-150405"), Path: p, Args: args, Started: time.Now(), Status: "waiting"}
	found, err := defenderScan(ctx, p)
	if err != nil {
		return errText("%v。検査できないものは箱でも動かさない", err)
	}
	if found != "" {
		return errText("%s。箱でも動かさずに止めました（ファイルはそのまま。消すかはカオルに聞く）", found)
	}
	s.Defender = "見つからなかった"
	runArgs := append([]string{"/box:" + tryBox, "/wait", p}, args...)
	timedOut, err := sbieWait(ctx, time.Duration(secs)*time.Second, runArgs...)
	var ee *exec.ExitError
	switch {
	case timedOut:
		s.Running = true
	case errors.As(err, &ee):
		s.Exit = fmt.Sprintf("終わりのコード %d", ee.ExitCode())
	case err != nil:
		return errText("箱の中で動かせませんでした：%v", err)
	}
	s.Ended = time.Now()
	s.Items, s.Noise, s.Registry, err = scanBox()
	if err != nil {
		return errText("箱の中身を読めませんでした：%v", err)
	}
	if err := saveTry(s); err != nil {
		return errText("試しの記録を書けませんでした：%v", err)
	}
	return okText(tryReport(s) + "\nTRY の窓（右下）に出しました。カオルが［箱の中で開く］で使ってみて、［本物に入れる］か［捨てる］を押します。返事は pc_try action=status。待つ間はほかの作業を進めてよい")
}

func tryReport(s *tryState) string {
	n := map[string]int{}
	for _, it := range s.Items {
		n[it.Kind]++
	}
	l := []string{
		"箱で試した：" + s.Path,
		"Defender：" + s.Defender,
		fmt.Sprintf("箱の中で：増えた %d・変わった %d・消えた %d・管理者の場所 %d（入れない）", n["new"], n["changed"], n["deleted"], n["admin"]),
		"ネット：切ってある（箱の中から外へはつながらない）",
	}
	if s.Running {
		l = append(l, "まだ箱の中で動いている（待ち時間を過ぎた。カオルが箱の中で使える）")
	} else if s.Exit != "" {
		l = append(l, "終わった："+s.Exit)
	}
	if s.Registry {
		l = append(l, "レジストリの変更あり（箱の中だけ。本物には入れない）")
	}
	if s.Noise > 0 {
		l = append(l, fmt.Sprintf("Windows が勝手に作ったもの %d（入れない）", s.Noise))
	}
	kw := map[string]string{"new": "増えた", "changed": "変わった", "deleted": "消えた", "admin": "管理者の場所"}
	for i, it := range s.Items {
		if i >= 30 {
			l = append(l, fmt.Sprintf("  ほか %d", len(s.Items)-30))
			break
		}
		if it.Kind == "deleted" {
			l = append(l, fmt.Sprintf("  %s：%s", kw[it.Kind], it.Real))
			continue
		}
		l = append(l, fmt.Sprintf("  %s：%s（%s）", kw[it.Kind], it.Real, mb(it.Size)))
	}
	switch s.Status {
	case "applied", "discarded":
		w := map[string]string{"applied": "本物に入れた", "discarded": "捨てた"}[s.Status]
		l = append(l, fmt.Sprintf("返事：%s（%s・%s）", w, s.By, s.At.Format("15:04")))
		if s.Result != "" {
			l = append(l, s.Result)
		}
	case "waiting":
		l = append(l, "返事：まだ（カオルの TRY の窓）")
	}
	return strings.Join(l, "\n")
}

func tryStatus() Reply {
	s, err := loadTry()
	if err != nil {
		return okText("箱で試しているものはありません")
	}
	return okText(tryReport(s))
}

func (a *Agent) tryDiscard(ctx context.Context, id, by string) Reply {
	s, err := loadTry()
	if err == nil && id != "" && s.ID != id {
		return errText("試しの番号が違います（いまは %s）", s.ID)
	}
	if err := emptyBox(ctx); err != nil {
		return errText("箱を空にできませんでした：%v", err)
	}
	if s != nil && s.Status == "waiting" {
		s.Status, s.By, s.At = "discarded", by, time.Now()
		s.Result = "箱を空にした（本物は何も変えていない）"
		_ = saveTry(s)
	}
	return okText("捨てた：箱を空にした（本物は何も変えていない）")
}

func (a *Agent) tryApply(ctx context.Context, id string) Reply {
	s, err := loadTry()
	if err != nil || s.ID != id {
		return errText("この試しはもうありません")
	}
	if s.Status != "waiting" {
		return errText("この試しはもう終わっています（%s）", s.Status)
	}
	_ = sbie(ctx, 30*time.Second, "/box:"+tryBox, "/terminate") // 箱の中で動いているものを止めてから
	time.Sleep(time.Second)
	items, _, _, err := scanBox()
	if err != nil {
		return errText("箱の中身を読めませんでした：%v", err)
	}
	// 窓に出したもの（記録）と、いまの箱の中身が同じかを確かめる（窓の表示と違うものは入れない）
	if !sameItems(s.Items, items) {
		s.Items = items
		_ = saveTry(s)
		return errText("箱の中身が TRY の窓に出していたものから変わっていました（箱の中で使った分など）。窓の一覧を新しくしたので、確かめてからもう一度押してください（まだ何も入れていない）")
	}
	root := boxRoot()
	var done, skipped, del, olds []string
	type put struct{ tmp, dst string }
	var puts []put
	for _, it := range items {
		switch it.Kind {
		case "admin":
			skipped = append(skipped, it.Real+"（管理者の場所。本物でもう一度入れ直す）")
		case "deleted":
			del = append(del, it.Real)
		case "new", "changed":
			if rf, e := os.Stat(it.Real); e == nil && rf.ModTime().After(s.Started) {
				skipped = append(skipped, it.Real+"（試しのあとで本物が変わった）")
				continue
			}
			src := realToBox(root, it.Real)
			if src == "" {
				skipped = append(skipped, it.Real+"（箱の中の場所が分からない）")
				continue
			}
			tmp, err := copyVerified(src, it.Real)
			if err != nil {
				skipped = append(skipped, it.Real+"（"+err.Error()+"）")
				continue
			}
			puts = append(puts, put{tmp, it.Real})
			if it.Kind == "changed" {
				olds = append(olds, it.Real)
			}
		}
	}
	// 変わったものの元の中身と、箱の中で消えたものは、取り置きへ（30 日戻せる）
	var res []string
	if hold := append(append([]string{}, olds...), del...); len(hold) > 0 {
		b, _ := json.Marshal(map[string]any{"paths": hold})
		if r := a.trash(ctx, b); r.IsError {
			for _, p := range puts {
				_ = os.Remove(p.tmp)
			}
			return errText("元のファイルを取り置きへ送れなかったので、何も入れずに止めました：%s", oneLine(r.Text, 200))
		}
		if len(olds) > 0 {
			res = append(res, fmt.Sprintf("変わったものの元 %d を取り置きへ", len(olds)))
		}
		if len(del) > 0 {
			res = append(res, fmt.Sprintf("箱の中で消えたもの %d を取り置きへ", len(del)))
		}
	}
	for _, p := range puts {
		if err := os.Rename(p.tmp, p.dst); err != nil {
			_ = os.Remove(p.tmp)
			skipped = append(skipped, p.dst+"（置き換えられない："+err.Error()+"）")
			continue
		}
		done = append(done, p.dst)
	}
	res = append([]string{fmt.Sprintf("入れた %d", len(done))}, res...)
	if len(skipped) > 0 {
		res = append(res, fmt.Sprintf("入れなかった %d：%s", len(skipped), strings.Join(skipped, "、")))
	}
	if err := emptyBox(ctx); err != nil {
		res = append(res, "箱を空にできなかった："+err.Error())
	}
	s.Status, s.By, s.At, s.Result = "applied", "カオル（TRY の窓）", time.Now(), strings.Join(res, "。")
	_ = saveTry(s)
	return okText("本物に入れた：" + s.Result)
}

func sameItems(a, b []tryItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || !strings.EqualFold(a[i].Real, b[i].Real) || a[i].Size != b[i].Size {
			return false
		}
	}
	return true
}

// realToBox：本物の場所 → 箱の中の場所
func realToBox(root, real string) string {
	under := func(base string) (string, bool) {
		if base == "" {
			return "", false
		}
		r, err := filepath.Rel(base, real)
		if err != nil || r == "." || strings.HasPrefix(r, "..") {
			return "", false
		}
		return r, true
	}
	if r, ok := under(homeDir()); ok {
		return filepath.Join(root, "user", "current", r)
	}
	if r, ok := under(programData()); ok {
		return filepath.Join(root, "user", "all", r)
	}
	if len(real) > 3 && real[1] == ':' && (real[2] == '\\' || real[2] == '/') {
		return filepath.Join(root, "drive", strings.ToUpper(real[:1]), real[3:])
	}
	return ""
}

// copyVerified：入れる先のとなりに一時の名前で写し、中身が同じかを確かめる（置き換えは後で）
func copyVerified(src, dst string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("フォルダを作れない：%v", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("箱の中を読めない：%v", err)
	}
	defer in.Close()
	tmp := dst + ".claude-hands-try"
	out, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("書けない：%v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("写せない：%v", err)
	}
	out.Close()
	if fileHash(tmp) != fileHash(src) {
		_ = os.Remove(tmp)
		return "", errors.New("写した中身が合わない")
	}
	return tmp, nil
}
