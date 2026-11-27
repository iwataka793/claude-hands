package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// callUndo：PC の常駐役（127.0.0.1:60210・合言葉 token.txt）に「まとめて戻す」を頼む。
// 常駐役が動いていない・「操作も可」でないときは、その旨を返す（スイッチを入れるのはカオル）
func callUndo(id string) string {
	tok, err := os.ReadFile(filepath.Join(home(), "devkit", "pc-agent", "token.txt"))
	if err != nil {
		return "PC の常駐役が動いていません。スイッチの窓で PC を「操作も可」にしてから、もう一度押してください"
	}
	body, _ := json.Marshal(map[string]any{"tool": "hands", "args": map[string]any{"action": "undo", "task": id, "by": "カオル（DONE の窓）"}})
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:60210/call", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return "PC の常駐役に頼めませんでした：" + err.Error()
	}
	defer res.Body.Close()
	var rep struct {
		Text    string `json:"text"`
		IsError bool   `json:"isError"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rep); err != nil {
		return "常駐役の返事が読めませんでした"
	}
	if rep.IsError && strings.Contains(rep.Text, "操作も可") {
		return "PC が「操作も可」になっていません。スイッチの窓で「操作も可」にしてから、もう一度押してください"
	}
	return rep.Text
}
