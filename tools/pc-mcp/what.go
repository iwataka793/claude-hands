package main

// 言い換えの表：許可の窓の WHAT（しようとしていること）を、道具が呼ばれた中身から作る。
// Claude には書かせない。命令の名前やファイルの場所の文字列はそのまま出さず、
// 「何を（相手）」＋「どうする（読む・変える・消す・送る・開く・押す・打つ）」のふつうの言葉にする。
// 表に無いものは隠さずに「よく分からない操作（道具の名前）」と出す。

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// PowerShell の命令の種類（guard.go の allowCmds の値）→ しようとしていること
var cmdWhat = map[string]string{
	"資格情報": "Windows に保存したパスワード（資格情報）を読む・変える",
	"証明書":  "証明書を出し入れする",
	"ネットワーク・ファイアウォール": "ネットワーク・ファイアウォールの設定を変える",
	"Defender":             "ウイルス対策（Defender）の設定を変える",
	"ファイアウォール":             "ファイアウォールの決まりを変える",
	"実行の決まり":               "PowerShell の実行の決まりを変える",
	"起動の設定":                "Windows の起動の設定を変える",
	"BitLocker":            "ドライブの暗号化（BitLocker）を変える",
	"ユーザー":                 "Windows のユーザーを作る・変える",
	"ユーザー・共有":              "ユーザー・共有の設定を見る・変える",
	"レジストリ":                "Windows の設定（レジストリ）を読む・変える",
	"タスクの予約":               "予約したタスクを作る・変える・消す",
	"サービス":                 "Windows のサービスを作る・変える",
	"中身を後で決める実行":           "中身が先に分からない命令を動かす",
	"別の PowerShell":        "別の PowerShell を起動して命令を動かす",
	"コマンドプロンプト":            "コマンドプロンプトで命令を動かす",
	"スクリプト":                "スクリプトを動かす",
	"外への通信":                "インターネットとやりとりする（落とす・送る）",
	"完全に消す（ごみ箱は pc_trash）": "ファイルを完全に消す（ごみ箱を通さない）",
	"ごみ箱を空にする":             "ごみ箱を空にする",
	"ドライブの初期化":             "ドライブを初期化する",
	"電源":                   "PC を切る・再起動する",
	"イベントログ":               "Windows の記録（イベントログ）を消す・変える",
	"復元ポイント":               "復元ポイントを消す・変える",
	"持ち主の変更":               "ファイルの持ち主を変える",
	"アクセス権":                "ファイルのアクセス権を変える",
}

// .Delete() のような呼び出し（guard.go の allowMemberRe）→ しようとしていること
func memberWhat(m string) string {
	l := strings.ToLower(m)
	switch {
	case strings.HasPrefix(l, "download") || strings.HasPrefix(l, "upload"):
		return "インターネットとやりとりする（落とす・送る）"
	case l == "protect" || l == "unprotect":
		return "暗号化したデータをほどく・かける"
	case strings.HasPrefix(l, "delete"):
		return "ファイル・設定を消す"
	}
	return ""
}

// 台本の文字（guard.go の textAllowRe）→ しようとしていること
func textWhat(m string) string {
	l := strings.ToLower(m)
	switch {
	case strings.Contains(l, "runas"):
		return "管理者として動かす（UAC）"
	case strings.Contains(l, "hklm") || strings.Contains(l, "hkey_local_machine") || strings.Contains(l, `\currentversion\run`) || strings.Contains(l, `\policies\`):
		return "Windows 全体の設定（レジストリ）を読む・変える"
	case strings.Contains(l, "encodedcommand"):
		return "中身が先に分からない命令を動かす"
	case strings.Contains(l, "webclient") || strings.Contains(l, "net.http"):
		return "インターネットとやりとりする（落とす・送る）"
	}
	return ""
}

// 守りの場所（guard.go の sensitivePathRe に合った文字）→ 何の場所か
var targetRules = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`(?i)login data|web data|cookies|local state|logins\.json|key4\.db|cookies\.sqlite|cert9\.db`), "ブラウザのパスワード・クッキーのファイル"},
	{regexp.MustCompile(`(?i)microsoft\\(credentials|protect|vault|systemcertificates|crypto)`), "Windows の資格情報・証明書の場所"},
	{regexp.MustCompile(`(?i)\.ssh|\.gnupg|\.aws|\.azure|\.kube|\.docker|git-credentials|\.netrc|\.npmrc|\.pypirc|gcloud|github cli|hosts\.yml`), "開発の鍵・トークンの場所"},
	{regexp.MustCompile(`(?i)go-ios|selfidentity|iloader|lockdown`), "iPhone の鍵（トンネル・Apple ID）の場所"},
	{regexp.MustCompile(`(?i)\.kdbx|\.pfx|\.p12|\.pem|\.key|id_rsa|id_ed25519|\.ovpn|wallet\.dat|exodus|electrum|bitwarden|1password|keepass`), "パスワード管理・鍵・お財布のファイル"},
	{regexp.MustCompile(`(?i)\.env`), "鍵の入った設定ファイル（.env）"},
	{regexp.MustCompile(`(?i)bank|銀行|証券|wallet|ウォレット|crypto|bitcoin|給与|源泉|確定申告|e-?tax`), "お金にかかわる名前のファイル"},
	{regexp.MustCompile(`(?i)マイナンバー|mynumber|passport|パスポート|免許証`), "本人確認にかかわる名前のファイル"},
	{regexp.MustCompile(`(?i)password|パスワード|secret|credential|資格情報`), "パスワードにかかわる名前のファイル"},
}

func targetName(match string) string {
	for _, r := range targetRules {
		if r.re.MatchString(match) {
			return r.name
		}
	}
	return "パスワード・セキュリティ・お金にかかわる場所"
}

// pathWhat：場所の種類と、どうするか
func pathWhat(cls, match string, modify bool) string {
	verb := "読む・開く"
	if modify {
		verb = "変える・消す"
	}
	switch cls {
	case "sensitive":
		return targetName(match) + "を" + verb
	case "system":
		return "Windows 本体の場所（C:\\Windows・Program Files など）を" + verb
	}
	return ""
}

// windowWhat：守りの窓（guard.go の sensitiveWindowRe に合った言葉）と、どうするか
func windowWhat(m, verb string) string {
	if m == "" {
		return "パスワード・セキュリティ・お金にかかわりそうな窓を" + verb
	}
	return "「" + m + "」にかかわりそうな窓を" + verb
}

// joinWhat：重なりを除いて「、」でつなぐ（多いときは 3 つまでと「ほか」）
func joinWhat(ws []string) string {
	ws = uniq(nonEmpty(ws))
	if len(ws) > 3 {
		return strings.Join(ws[:3], "、") + "、ほか " + itoaW(len(ws)-3) + " 件"
	}
	return strings.Join(ws, "、")
}

func nonEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	}
	return out
}

func itoaW(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoaW(n/10) + string(rune('0'+n%10))
}

// ---- Claude の説明（WHY） ----

// whyArg：道具の引数に足す「説明」。窓が出るとき（スイッチが足りない・許可が要る）だけ要る
type whyArg struct {
	Why string `json:"why,omitempty" jsonschema:"カオルの許可の窓（スイッチが足りない・パスワードなどで ALLOW が要る）が出るときだけ要る、Claude の説明。この許可が続く間に何をするかを、ふつうの言葉で 100 字まで（記号の多い文・命令・URL は受け付けない）。窓に紫で出る"`
}

const whyMax = 100

var whySymbols = "$%&*|\\/<>{}[]=;~^`\"#@"

// checkWhy：説明として受け付けるか。だめなら Claude に返す文
func checkWhy(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "この操作にはカオルの許可の窓が出ます。引数 why に、この許可で何をするかを、ふつうの言葉で " + itoaW(whyMax) + " 字までで書いて、呼び直してください（窓に「Claude の説明」として出ます）"
	}
	if n := utf8.RuneCountInString(s); n > whyMax {
		return "why が長すぎます（" + itoaW(n) + " 字）。" + itoaW(whyMax) + " 字までに縮めて呼び直してください"
	}
	if strings.ContainsAny(s, "\r\n\t") {
		return "why は 1 行で書いてください"
	}
	l := strings.ToLower(s)
	if strings.Contains(l, "http") || strings.Contains(l, "://") || strings.Contains(l, "www.") {
		return "why に URL は書けません。ふつうの言葉で書き直してください"
	}
	sym := 0
	for _, r := range s {
		if strings.ContainsRune(whySymbols, r) {
			sym++
		}
	}
	if sym >= 3 {
		return "why に記号が多すぎます（命令のように見える）。ふつうの言葉で書き直してください"
	}
	return ""
}
