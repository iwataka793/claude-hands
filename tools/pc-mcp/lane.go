package main

// 作業の分け方 A〜E（ASK-DESIGN.md の 2）。どの道かは道具が決め、TIMELINE と DONE の報告に残す。
//   A 読む・調べる：そのまま
//   B 箱で試せる変更：箱（5 番）ができるまでは C と同じ
//   C 箱で試せない変更（PowerShell で何かを変える）：先に控え（Git）を取る → 本物でやる → 道具が確かめる
//      （エラーが出たか・Windows のエラーの記録が増えたか）→ エラーが出て決まったフォルダが変わっていたら自動で戻す
//   D 小さくて戻せる変更（決めた操作：ごみ箱へ・開く・窓の部品を押す・打つ・ページの操作）：そのまま。取り置きと記録
//   E 戻せないもの（送る・買う・ログイン・投稿）：Claude は最後の一押しの手前で止まる。押すのはカオル

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// finalRe：戻せない操作の最後のボタンの名前（短い名前だけを見る）
var finalRe = regexp.MustCompile(`(?i)(送信|送る|購入|注文を確定|注文する|ご注文|支払う|支払い|お支払い|決済|ログイン|サインイン|投稿|公開する|ツイート|ポストする|\bsend\b|\bsubmit\b|\bbuy\b|\bpurchase\b|place (your )?order|\bpay\b|checkout|check out|\blog ?in\b|\bsign ?in\b|\bpost\b|\bpublish\b|\btweet\b)`)

// finalWord：名前が最後のボタンなら、合った言葉を返す
func finalWord(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 30 {
		return ""
	}
	return finalRe.FindString(name)
}

// finalJS：ブラウザの中で同じ言葉を見るための正規表現（JavaScript の形）
func finalJS() string {
	src := strings.TrimPrefix(finalRe.String(), "(?i)")
	return "new RegExp(" + jsonString(src) + ",'i')"
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// sendAppRe：Enter だけで送ってしまうアプリ（ブラウザ・チャット・メール・Claude）
var sendAppRe = regexp.MustCompile(`(?i)(chrome|msedge|firefox|brave|opera|vivaldi|line|slack|teams|discord|outlook|olk|thunderbird|whatsapp|telegram|messenger|claude|chatgpt|メール|mail)`)

// sendKeys：打つものに Enter（送信に当たるキー）が入っているか
func sendKeys(keys, text string, enter bool) bool {
	if enter {
		return true
	}
	k := strings.ToLower(keys)
	return strings.Contains(k, "{enter}") || strings.Contains(k, "~") || strings.ContainsAny(text, "\r\n")
}

// stopE：E で止めた返事
func stopE(what, word string) Reply {
	r := errText("E（戻せない操作の最後の一押し）：%s は Claude は押しません（「%s」）。ここまで進めた状態をカオルに見せて、押すのはカオル。チャットで「〜を押してください」と伝える", what, word)
	r.Lane = "E"
	return r
}

// eErr：ブラウザの手順の中で E で止めた
type eErr struct{ what, word string }

func (e eErr) Error() string {
	return "E（戻せない操作の最後の一押し）：" + e.what + " は Claude は押しません（「" + e.word + "」）。ここまで進めた状態をカオルに見せて、押すのはカオル"
}

// laneOf：道具ごとの道（常駐役が返事に入れなかったとき）
func laneOf(tool string, args map[string]any) string {
	switch tool {
	case "pc_disk", "pc_problems", "pc_processes", "pc_apps", "pc_files", "pc_screenshot", "pc_windows", "pc_ui",
		"browser_read", "browser_screenshot", "hands":
		return "A"
	case "browser_tabs":
		if a, _ := args["action"].(string); a == "" || strings.EqualFold(a, "list") {
			return "A"
		}
		return "D"
	case "pc_powershell":
		return "C"
	}
	return "D"
}

var laneWord = map[string]string{"A": "読む", "B": "箱で試す", "C": "変更（控えあり）", "D": "小さな変更", "E": "止めた（カオルが押す）"}
