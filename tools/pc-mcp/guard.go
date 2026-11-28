package main

// 守り：パスワード・セキュリティ・お金にかかわる場所と命令は、カオルの許可（PC-ALLOW.bat、10 分）が要る。
// スイッチの置き場所（pc-agent・iphone-mcp の mode.txt / allow.txt）は、許可があっても触らせない。
//
// これは「うっかり」を止めるための守り。命令の中身を全部見抜けるわけではないので、
// 記録（audit.log）と、Claude 側の決まり（CLAUDE.md・docs/PC.md）と組み合わせて使う。

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 許可が要る場所（読むのも変えるのも）
var sensitivePathRe = regexp.MustCompile(`(?i)(` + strings.Join([]string{
	// ブラウザのパスワード・クッキー・カード
	`\\Login Data`, `\\Web Data\b`, `\\Cookies\b`, `\\Local State\b`, `logins\.json`, `key4\.db`, `cookies\.sqlite`, `cert9\.db`,
	// Windows の資格情報・証明書
	`\\Microsoft\\(Credentials|Protect|Vault|SystemCertificates|Crypto)\b`,
	// 開発の鍵・トークン
	`\\\.ssh\\`, `\\\.ssh$`, `\\\.gnupg\b`, `\\\.aws\b`, `\\\.azure\b`, `\\\.kube\b`, `\\\.docker\\config\.json`, `\.git-credentials`,
	`\\\.netrc`, `\\\.npmrc`, `\\\.pypirc`, `\\gcloud\b`, `\\GitHub CLI\b`, `\\gh\\hosts\.yml`,
	// iPhone の鍵（トンネルの鍵・Apple ID のサインイン）
	`\\devkit\\go-ios\b`, `selfIdentity\.plist`, `me\.nabdev\.iloader`, `\\Lockdown\b`,
	// パスワード管理・鍵ファイル・お財布
	`\.kdbx\b`, `\.pfx\b`, `\.p12\b`, `\.pem\b`, `\.key\b`, `id_rsa`, `id_ed25519`, `\.ovpn\b`, `wallet\.dat`,
	`\\Exodus\b`, `\\Electrum\b`, `\\Bitwarden\b`, `\\1Password\b`, `\\KeePass`,
	`(^|\\)\.env(\.|$|\\)`,
	// 名前で分かるお金・本人確認・パスワード
	`bank`, `銀行`, `証券`, `wallet`, `ウォレット`, `crypto`, `bitcoin`, `給与`, `源泉`, `確定申告`, `e-?tax`, `マイナンバー`, `mynumber`,
	`passport`, `パスポート`, `免許証`, `password`, `パスワード`, `secret`, `credential`, `資格情報`,
}, "|") + `)`)

// スイッチの置き場所：許可があっても触らせない
var switchPathRe = regexp.MustCompile(`(?i)(\\devkit\\pc-agent\b|\\devkit\\browser\\(mode|last-used)\.txt|\\ProgramData\\pc-agent\b|\\devkit\\iphone-mcp\\(mode|allow|last-used)\.txt|pc-agent\.exe|\bmode\.txt\b|\ballow\.txt\b)`)

// 箱（Sandboxie）の設定：許可があっても触らせない（2026-11-28）。設定は pc-setup.bat（UAC）が入れ、
// Sandboxie の EditAdminOnly=y でカオルの普通の権限からは変えられない。ここはその手前の「うっかり」止め。
// Start.exe は箱でものを動かすのに使うので止めない。設定の窓を開く使い方（run_sbie_ctrl・open_agent）だけ止める。
var boxConfRe = regexp.MustCompile(`(?i)(\bsbieini(\.exe)?\b|\bsandman(\.exe)?\b|\bsbiectrl(\.exe)?\b|\bsandboxie\.ini\b|\brun_sbie_ctrl\b|\bopen_agent\b)`)

// 箱の設定の窓（SandMan・SbieCtrl）の題名。ブラウザの窓（タブの題名）は除く
var boxWindowRe = regexp.MustCompile(`(?i)^[\s「]*sandboxie(-plus| control)\b`)
var browserWindowRe = regexp.MustCompile(`(?i)(google chrome|microsoft ?edge|firefox|chrome（pid|msedge（pid)`)

const boxConfWhy = "箱（Sandboxie）の設定（変えるのは pc-setup.bat だけ）"

// isBoxConf：箱の設定にかかわる名前に触れているか。
func isBoxConf(s string) bool { return boxConfRe.MatchString(s) }

// isBoxWindow：箱の設定の窓か（プログラムの名前か、窓の題名で見る）。
func isBoxWindow(s string) bool {
	return isBoxConf(s) || (boxWindowRe.MatchString(s) && !browserWindowRe.MatchString(s))
}

// 箱のすり抜け止め（2026-11-28。ASK-DESIGN.md の 6「箱のすり抜け」）：
// Claude が落としたもの（devkit\downloads）の中の、動くもの・マクロ入りの文書は、箱の外では動かさない（pc_try を使う）。
// 箱の中身（C:\Sandbox）は台本から触らない（本物に写すのは TRY の窓だけ。読むのは pc_files でよい）。
var riskyExt = `(exe|msi|msix|msixbundle|appx|appxbundle|bat|cmd|ps1|psm1|vbs|vbe|js|jse|wsf|wsh|hta|scr|com|pif|lnk|url|docm|dotm|xlsm|xltm|xlam|pptm|potm|ppam|jar|reg|cpl|msc|inf|application|appref-ms)`
var riskyDownloadRe = regexp.MustCompile(`(?i)\\devkit\\downloads\\[^'"\r\n;|]*\.` + riskyExt + `\b`)
var boxDataRe = regexp.MustCompile(`(?i)(^|[^a-z])[a-z]:\\sandbox(\\|$|['"\s])`)

// 台本の中で「動かす」に当たる命令（落としたものを箱の外で動かす）
var runCmds = map[string]bool{"start-process": true, "invoke-item": true, "msiexec": true, "msiexec.exe": true, "rundll32": true, "rundll32.exe": true,
	"regsvr32": true, "regsvr32.exe": true, "cmd": true, "cmd.exe": true, "powershell": true, "powershell.exe": true, "pwsh": true, "pwsh.exe": true,
	"wscript": true, "wscript.exe": true, "cscript": true, "cscript.exe": true, "mshta": true, "mshta.exe": true, "explorer": true, "explorer.exe": true,
	"java": true, "java.exe": true, "javaw": true, "javaw.exe": true, "reg": true, "reg.exe": true, "regedit": true, "regedit.exe": true, "import-module": true}

const tryMsg = "devkit\\downloads の中の動くもの（ネットから来たもの）は、箱の外では動かしません。pc_try で箱の中で試してください（本物に入れるかはカオルが TRY の窓で決める）"

// runsDownload：台本が devkit\downloads の動くものを、箱の外で動かそうとしているか
func runsDownload(info scriptInfo, text string) bool {
	if !riskyDownloadRe.MatchString(text) {
		return false
	}
	// 箱の中で動かす（Start.exe /box:ClaudeBox …）のはよい
	if l := strings.ToLower(text); strings.Contains(l, `sandboxie-plus\start.exe`) && strings.Contains(l, "box:"+strings.ToLower(tryBox)) {
		return false
	}
	for _, c := range info.Cmds {
		lc := strings.ToLower(c)
		if runCmds[lc] || riskyDownloadRe.MatchString(strings.ReplaceAll(c, "/", `\`)) {
			return true
		}
	}
	return false
}

// openRuns：pc_open の target・args に、devkit\downloads の動くもの・箱の中のものがあるか
func openRuns(target, args string) string {
	t := matchForm(normPath(target)) + " " + args
	if riskyDownloadRe.MatchString(t) || (inDownloads(normPath(target)) && regexp.MustCompile(`(?i)\.`+riskyExt+`$`).MatchString(target)) {
		return tryMsg
	}
	if boxDataRe.MatchString(matchForm(target)) || boxDataRe.MatchString(args) {
		return "箱の中のもの（C:\\Sandbox）は、箱の外では開きません（中を見るのは pc_files で。動かすのは pc_try）"
	}
	return ""
}

// Windows 本体：読むのはよいが、変えるには許可が要る
var systemPathRe = regexp.MustCompile(`(?i)^([a-z]:\\?$|[a-z]:\\windows\b|[a-z]:\\program files|[a-z]:\\programdata\b|[a-z]:\\\$recycle\.bin|[a-z]:\\system volume information|[a-z]:\\users\\[^\\]+\\?$|.*\\start menu\\programs\\startup\b)`)

// normPath は、パスを絶対パス・バックスラッシュにそろえる（%USERPROFILE% なども広げる）。
func normPath(p string) string {
	p = os.ExpandEnv(expandWinEnv(strings.TrimSpace(p)))
	if strings.HasPrefix(p, "~") {
		if h, err := os.UserHomeDir(); err == nil {
			p = h + p[1:]
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.FromSlash(p)
}

// matchForm は、決まりの正規表現（バックスラッシュで書いてある）と比べるための形。
func matchForm(p string) string { return strings.ReplaceAll(p, "/", `\`) }

var winEnvRe = regexp.MustCompile(`%([A-Za-z0-9_()]+)%`)

func expandWinEnv(s string) string {
	return winEnvRe.ReplaceAllStringFunc(s, func(m string) string {
		if v := os.Getenv(m[1 : len(m)-1]); v != "" {
			return v
		}
		return m
	})
}

// isSwitchPlace：スイッチ・許可の置き場所に触れているか（決まった名前と、実際の置き場所の両方で見る）。
func isSwitchPlace(s string) bool {
	if switchPathRe.MatchString(s) {
		return true
	}
	low := strings.ToLower(matchForm(s))
	for _, d := range []string{stateDir(), allowDir(), browserDir(), handsDir()} {
		if d != "" && strings.Contains(low, strings.ToLower(matchForm(d))) {
			return true
		}
	}
	return false
}

// pathClass は "switch"（触らせない）・"sensitive"（許可が要る）・"system"（変えるには許可）・""（自由）。
func pathClass(p string) (string, string) {
	n := matchForm(normPath(p))
	switch {
	case isSwitchPlace(n):
		return "switch", "スイッチ・許可・守り（取り置き）の置き場所"
	case isBoxConf(n):
		return "switch", boxConfWhy
	case sensitivePathRe.MatchString(n):
		return "sensitive", "パスワード・セキュリティ・お金にかかわる場所（" + sensitivePathRe.FindString(n) + "）"
	case systemPathRe.MatchString(n):
		return "system", "Windows 本体の場所"
	}
	return "", ""
}

// ---- PowerShell の台本を調べる ----

// scriptInfo は、PowerShell 自身に台本を読ませて（AST）分かったこと。
type scriptInfo struct {
	Cmds    []string `json:"cmds"`    // 呼ぶ命令（別名は元の名前に直したもの）
	Members []string `json:"members"` // .Delete() のような呼び出し
	Redir   int      `json:"redir"`   // > や >> でファイルに書く数
	Dynamic int      `json:"dynamic"` // & $x のように中身が決まっていない呼び出しの数
	Errors  []string `json:"errors"`  // 文法の誤り
}

// 見るだけのときに使ってよい命令（動詞）
var viewVerbs = map[string]bool{"get": true, "test": true, "measure": true, "select": true, "sort": true, "where": true,
	"foreach": true, "format": true, "convertto": true, "convertfrom": true, "group": true, "compare": true, "resolve": true,
	"find": true, "join": true, "split": true}

var viewExact = map[string]bool{"out-string": true, "out-null": true, "write-output": true, "write-host": true,
	"whoami": true, "hostname": true, "ipconfig": true, "systeminfo": true, "tasklist": true, "where": true, "where.exe": true,
	"whoami.exe": true, "hostname.exe": true, "ipconfig.exe": true, "systeminfo.exe": true, "tasklist.exe": true, "findstr": true, "findstr.exe": true}

var badMemberRe = regexp.MustCompile(`(?i)^(delete|move|moveto|copy|copyto|create|createsubdirectory|write|writeall\w*|append\w*|set\w*|remove\w*|kill|start|stop|open\w*|replace|encrypt|decrypt|save\w*|clear|add\w*|invoke\w*|execute\w*|run\w*|upload\w*|download\w*|send\w*|unprotect|protect|import\w*|export\w*|install\w*|uninstall\w*|change\w*|rename|delete\w*|post\w*|put\w*)$`)

// 操作のときでも許可が要る命令
var allowCmds = map[string]string{
	"cmdkey": "資格情報", "cmdkey.exe": "資格情報", "vaultcmd": "資格情報", "vaultcmd.exe": "資格情報",
	"get-storedcredential": "資格情報", "get-credential": "資格情報", "convertfrom-securestring": "資格情報", "convertto-securestring": "資格情報",
	"export-pfxcertificate": "証明書", "import-pfxcertificate": "証明書", "certutil": "証明書", "certutil.exe": "証明書",
	"netsh": "ネットワーク・ファイアウォール", "netsh.exe": "ネットワーク・ファイアウォール",
	"set-mppreference": "Defender", "add-mppreference": "Defender", "remove-mppreference": "Defender",
	"new-netfirewallrule": "ファイアウォール", "set-netfirewallrule": "ファイアウォール", "remove-netfirewallrule": "ファイアウォール",
	"disable-netfirewallrule": "ファイアウォール", "enable-netfirewallrule": "ファイアウォール", "set-netfirewallprofile": "ファイアウォール",
	"set-executionpolicy": "実行の決まり", "bcdedit": "起動の設定", "bcdedit.exe": "起動の設定", "manage-bde": "BitLocker", "manage-bde.exe": "BitLocker",
	"disable-bitlocker": "BitLocker", "enable-bitlocker": "BitLocker", "suspend-bitlocker": "BitLocker",
	"new-localuser": "ユーザー", "set-localuser": "ユーザー", "remove-localuser": "ユーザー", "add-localgroupmember": "ユーザー", "remove-localgroupmember": "ユーザー",
	"net": "ユーザー・共有", "net.exe": "ユーザー・共有", "net1.exe": "ユーザー・共有",
	"reg": "レジストリ", "reg.exe": "レジストリ", "regedit": "レジストリ", "regedit.exe": "レジストリ",
	"register-scheduledtask": "タスクの予約", "set-scheduledtask": "タスクの予約", "unregister-scheduledtask": "タスクの予約", "schtasks": "タスクの予約", "schtasks.exe": "タスクの予約",
	"new-service": "サービス", "set-service": "サービス", "remove-service": "サービス", "sc": "サービス", "sc.exe": "サービス",
	"invoke-expression": "中身を後で決める実行", "invoke-command": "中身を後で決める実行",
	"powershell": "別の PowerShell", "powershell.exe": "別の PowerShell", "pwsh": "別の PowerShell", "pwsh.exe": "別の PowerShell",
	"cmd": "コマンドプロンプト", "cmd.exe": "コマンドプロンプト", "wscript.exe": "スクリプト", "cscript.exe": "スクリプト", "mshta.exe": "スクリプト",
	"invoke-webrequest": "外への通信", "invoke-restmethod": "外への通信", "curl.exe": "外への通信", "start-bitstransfer": "外への通信", "send-mailmessage": "外への通信",
	"remove-item": "完全に消す（ごみ箱は pc_trash）", "clear-recyclebin": "ごみ箱を空にする", "format-volume": "ドライブの初期化", "clear-disk": "ドライブの初期化",
	"remove-itemproperty": "レジストリ", "set-itemproperty": "レジストリ", "new-itemproperty": "レジストリ",
	"stop-computer": "電源", "restart-computer": "電源", "shutdown": "電源", "shutdown.exe": "電源",
	"wevtutil": "イベントログ", "wevtutil.exe": "イベントログ", "vssadmin": "復元ポイント", "vssadmin.exe": "復元ポイント",
	"takeown": "持ち主の変更", "takeown.exe": "持ち主の変更", "icacls": "アクセス権", "icacls.exe": "アクセス権", "set-acl": "アクセス権",
}

var allowMemberRe = regexp.MustCompile(`(?i)^(unprotect|protect|downloadstring|downloadfile|downloaddata|uploadstring|uploadfile|uploaddata|delete|deletesubkey\w*|deletevalue)$`)

var textAllowRe = regexp.MustCompile(`(?i)(-verb\s+['"]?runas|hklm:|hkey_local_machine|\\currentversion\\run\b|\\policies\\|-encodedcommand|\bnet\.webclient\b|\bsystem\.net\.http\b)`)

// checkScript は、台本を実行してよいかを決める。
// 返り値：block（許可があってもだめ）・needAllow（許可が要る理由）・viewNG（見るだけでは使えない理由）
func checkScript(info scriptInfo, src string) (block, needAllow, viewNG string) {
	block, needAllow, viewNG, _ = checkScriptWhat(info, src)
	return
}

// checkScriptWhat は checkScript と同じで、許可の窓の WHAT（ふつうの言葉。what.go の表）も返す。
func checkScriptWhat(info scriptInfo, src string) (block, needAllow, viewNG, what string) {
	text := strings.ReplaceAll(src, "/", `\`)
	if isSwitchPlace(text) {
		return "スイッチ・許可・守り（取り置き）の置き場所は触れません", "", "", ""
	}
	if m := boxConfRe.FindString(text); m != "" {
		return boxConfWhy + "には触れません（" + m + "）。Claude からは変えない決まり。変えるときはカオルに頼む", "", "", ""
	}
	if boxDataRe.MatchString(text) {
		return "箱の中身（C:\\Sandbox）は台本から触りません。中を見るのは pc_files、試すのは pc_try、本物に入れるのはカオルの TRY の窓だけ", "", "", ""
	}
	if runsDownload(info, text) {
		return tryMsg, "", "", ""
	}
	if len(info.Errors) > 0 {
		return "台本の書き方に誤りがあります：" + strings.Join(info.Errors, " / "), "", "", ""
	}
	var allowWhy, whats []string
	for _, c := range info.Cmds {
		lc := strings.ToLower(c)
		if why, ok := allowCmds[lc]; ok {
			allowWhy = append(allowWhy, c+"（"+why+"）")
			whats = append(whats, cmdWhat[why])
		}
		if viewNG == "" && !viewExact[lc] {
			verb := lc
			if i := strings.Index(lc, "-"); i > 0 {
				verb = lc[:i]
			} else {
				verb = "" // 動詞の無い外の exe は、決めたものだけ
			}
			if !viewVerbs[verb] {
				viewNG = "命令 " + c
			}
		}
	}
	for _, m := range info.Members {
		if allowMemberRe.MatchString(m) {
			allowWhy = append(allowWhy, "."+m+"()")
			whats = append(whats, memberWhat(m))
		}
		if viewNG == "" && badMemberRe.MatchString(m) {
			viewNG = "呼び出し ." + m + "()"
		}
	}
	if info.Dynamic > 0 {
		allowWhy = append(allowWhy, "中身が決まっていない呼び出し（& $x など）")
		whats = append(whats, "中身が先に分からない命令を動かす")
		if viewNG == "" {
			viewNG = "中身が決まっていない呼び出し"
		}
	}
	if info.Redir > 0 && viewNG == "" {
		viewNG = "ファイルへの書き出し（>）"
	}
	if m := textAllowRe.FindString(text); m != "" {
		allowWhy = append(allowWhy, m)
		whats = append(whats, textWhat(m))
	}
	if m := sensitivePathRe.FindString(text); m != "" {
		allowWhy = append(allowWhy, "場所 "+m)
		whats = append(whats, targetName(m)+"を読む・変える")
	}
	if len(allowWhy) > 0 {
		needAllow = strings.Join(uniq(allowWhy), "、")
		what = joinWhat(whats)
		if what == "" {
			what = "よく分からない操作（pc_powershell）"
		}
	}
	return
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func allowMsg(why string) string {
	return fmt.Sprintf("カオルの許可が要ります（%s）。引数 why（この許可で何をするか）を付けて呼ぶと、カオルの PC に ALLOW の窓が出て、許可されれば 10 分間使えます。Claude からは許可できません", why)
}

// カオルの窓（スイッチ・許可の頼み・LIVE・DONE・前の PowerShell のスイッチ）：Claude の道具は押さない・打たない（許可があっても）
var handsWindowRe = regexp.MustCompile(`(?i)(claude-hands (switch|ask|live)|claude 操作スイッチ|claude からの頼み)`)

// 画面の操作で許可が要る窓（題名・プログラム名で判断）
var sensitiveWindowRe = regexp.MustCompile(`(?i)(bank|銀行|証券|paypay|楽天ペイ|ペイディ|クレジット|カード|wallet|ウォレット|crypto|bitcoin|password|パスワード|sign ?in|サインイン|ログイン|log ?in|credential|資格情報|1password|bitwarden|keepass|windows セキュリティ|windows security|defender|ファイアウォール|firewall|user account control|ユーザー アカウント制御|bitlocker|マイナ|e-?tax|確定申告|アカウント|account)`)
