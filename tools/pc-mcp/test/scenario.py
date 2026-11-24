"""PC の道具を通しで試す（GitHub の Windows と、手元の Linux＋pwsh の両方で動く）。

環境変数：
  PCMCP   拡張機能の exe（pc-mcp.exe）
  PCAGENT 常駐役の exe（pc-agent.exe）
  PC_AGENT_DIR / PC_AGENT_ALLOW_DIR  置き場所（試験用の一時フォルダ）
"""
import json, os, subprocess, sys, time, pathlib, tempfile

WIN = os.name == "nt"
MCP = os.environ["PCMCP"]
AGENT = os.environ["PCAGENT"]
SD = pathlib.Path(os.environ["PC_AGENT_DIR"])
AD = pathlib.Path(os.environ["PC_AGENT_ALLOW_DIR"])
SD.mkdir(parents=True, exist_ok=True)
AD.mkdir(parents=True, exist_ok=True)
fails = []


def call(name, args=None, timeout=300):
    msgs = [
        {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "t", "version": "0"}}},
        {"jsonrpc": "2.0", "method": "notifications/initialized"},
        {"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": name, "arguments": args or {}}},
    ]
    p = subprocess.Popen([MCP], stdin=subprocess.PIPE, stdout=subprocess.PIPE)
    for m in msgs:
        p.stdin.write((json.dumps(m, ensure_ascii=False) + "\n").encode("utf-8"))
        p.stdin.flush()
    t0 = time.time()
    res = None
    while time.time() - t0 < timeout:
        l = p.stdout.readline()
        if not l:
            break
        m = json.loads(l)
        if m.get("id") == 2:
            res = m
            break
    dt = time.time() - t0
    p.stdin.close()
    p.kill()
    if res is None:
        return True, "（返事なし）", False, dt
    r = res.get("result", {})
    texts = [c.get("text", "") for c in r.get("content", []) if c.get("type") == "text"]
    img = any(c.get("type") == "image" for c in r.get("content", []))
    return bool(r.get("isError")), "\n".join(texts), img, dt


def check(label, name, args, want_err, contains=None, show=600):
    err, text, img, dt = call(name, args)
    ok = (err == want_err) and (contains is None or contains in text)
    mark = "OK " if ok else "NG "
    print(f"{mark}{label}（{name}・{dt:.1f} 秒・エラー={err}・画像={img}）")
    print("    " + text[:show].replace("\n", "\n    "))
    if not ok:
        fails.append(label)
    return err, text


def look(label, name, args):
    err, text, img, dt = call(name, args)
    print(f"-- {label}（{name}・{dt:.1f} 秒・エラー={err}・画像={img}）")
    print("    " + text[:1500].replace("\n", "\n    "))
    return err, text


def set_mode(m):
    (SD / "mode.txt").write_text(m + "\r\n")
    time.sleep(1.2)


def start_agent():
    flags = 0x08000000 if WIN else 0
    subprocess.Popen([AGENT, "agent"], creationflags=flags)
    for _ in range(50):
        if (SD / "token.txt").exists():
            return
        time.sleep(0.2)
    print("常駐役が起動しない")
    fails.append("常駐役の起動")


tmp = pathlib.Path(tempfile.gettempdir()) / "pc-mcp-test"
tmp.mkdir(exist_ok=True)

print("=== 1. オフ（常駐役なし）")
check("状態", "pc_status", {}, False, "動いていない")
check("オフなら調べられない", "pc_disk", {}, True, "オフ")

print("=== 2. 見るだけ")
set_mode("view")
start_agent()
check("状態", "pc_status", {}, False, "見るだけ")
check("容量（ドライブと膨らみやすい場所）", "pc_disk", {"seconds": 60}, False, "■ ドライブ" if WIN else "■")
check("容量（一時フォルダの中・大きいファイル）", "pc_disk", {"path": str(tmp.parent), "files": True, "top": 5, "seconds": 60}, False, "の中")
check("不具合の記録", "pc_problems", {"hours": 24, "max": 5}, False, "■ イベントログ" if WIN else None)
(check if WIN else look)("プロセス（メモリ）", "pc_processes", {"top": 5}, *([False] if WIN else []))
check("プロセス（CPU）", "pc_processes", {"sort": "cpu", "top": 5}, False)
check("アプリ", "pc_apps", {"max": 5}, False, "■" if WIN else None)
(check if WIN else look)("起動時に動くもの", "pc_apps", {"kind": "startup"}, *([False] if WIN else []))
check("ファイルの一覧", "pc_files", {"action": "list", "path": str(tmp.parent), "max": 5}, False)
check("PowerShell（読む）", "pc_powershell", {"script": "Get-Process | Sort-Object WS -Descending | Select-Object -First 3 -ExpandProperty ProcessName"}, False)
check("PowerShell（速さ：2 回目）", "pc_powershell", {"script": "1+1"}, False, "2")
check("見るだけでは作れない", "pc_powershell", {"script": f"New-Item -ItemType File '{tmp / 'a.txt'}'"}, True, "見るだけ")
check("見るだけではごみ箱に送れない", "pc_trash", {"paths": [str(tmp / "a.txt")]}, True, "見るだけ")

print("=== 3. 操作も可")
set_mode("operate")
f = tmp / "trash-me.txt"
check("ファイルを作る", "pc_powershell", {"script": f"Set-Content -Path '{f}' -Value 'test'; Test-Path '{f}'"}, False, "True")
check("完全に消すのは許可が要る", "pc_powershell", {"script": f"Remove-Item '{f}'"}, True, "許可")
check("資格情報は許可が要る", "pc_powershell", {"script": "cmdkey /list"}, True, "許可")
check("外への通信は許可が要る", "pc_powershell", {"script": "Invoke-WebRequest https://example.com -UseBasicParsing"}, True, "許可")
check("スイッチの置き場所は触れない", "pc_powershell", {"script": f"Get-Content '{SD / 'mode.txt'}'"}, True, "置き場所")
check("許可の置き場所は触れない", "pc_powershell", {"script": f"Set-Content '{AD / 'allow.txt'}' x"}, True, "置き場所")
check("鍵の場所の中身は許可が要る", "pc_files", {"action": "read", "path": "~/.ssh/id_rsa"}, True, "許可")
if WIN:
    check("ごみ箱へ送る", "pc_trash", {"paths": [str(f)]}, False, "ごみ箱へ")
    print("    残っているか：", f.exists())
    if f.exists():
        fails.append("ごみ箱へ送ったのに残っている")
    check("Windows 本体はごみ箱にも送れない", "pc_trash", {"paths": [r"C:\Windows\notepad.exe"]}, True, "許可")

print("=== 4. 許可（ALLOW）")
(AD / "allow.txt").write_text("allow\r\n")
check("状態（許可あり）", "pc_status", {}, False, "あり")
if WIN:
    check("許可があれば資格情報の一覧を見られる", "pc_powershell", {"script": "cmdkey /list | Select-Object -First 3"}, False)
check("許可があれば完全に消せる", "pc_powershell", {"script": f"Set-Content '{tmp / 'b.txt'}' x; Remove-Item '{tmp / 'b.txt'}'; Test-Path '{tmp / 'b.txt'}'"}, False, "False")
check("許可があってもスイッチの置き場所は触れない", "pc_powershell", {"script": f"Set-Content '{SD / 'mode.txt'}' operate"}, True, "置き場所")
set_mode("operate")
check("スイッチを切り替えると許可は取り消し", "pc_status", {}, False, "許可（ALLOW）：なし")

if WIN:
    print("=== 5. 画面と部品（GitHub の Windows に画面があるかは分からないので、結果を見るだけ）")
    look("窓の一覧", "pc_windows", {})
    look("画面", "pc_screenshot", {})
    look("メモ帳を開く", "pc_open", {"target": "notepad.exe"})
    time.sleep(2)
    look("窓の一覧", "pc_windows", {})
    look("メモ帳の部品", "pc_ui", {"window": "notepad", "max": 30})
    look("メモ帳に貼る", "pc_keys", {"window": "notepad", "text": "こんにちは PC の道具", "paste": True})
    look("メモ帳に打つ", "pc_keys", {"window": "notepad", "text": " abc 123", "enter": True})
    look("打ったあとの窓の一覧", "pc_windows", {"all": True})
    look("打ったあとの前の窓の部品（window なし）", "pc_ui", {"max": 8})
    look("Notepad（大文字）で探す", "pc_ui", {"window": "Notepad", "max": 5})
    look("診断：窓を素で数える", "pc_powershell", {"script": "Add-Type -AssemblyName UIAutomationClient; $r=[System.Windows.Automation.AutomationElement]::RootElement; $c=$r.FindAll([System.Windows.Automation.TreeScope]::Children,[System.Windows.Automation.Condition]::TrueCondition); foreach($w in $c){ '{0} | {1} | {2}' -f $w.Current.Name,$w.Current.ProcessId,$w.Current.ClassName }; Get-Process notepad -ErrorAction SilentlyContinue | ForEach-Object { 'proc {0} {1} [{2}]' -f $_.Id,$_.MainWindowHandle,$_.MainWindowTitle }"})
    look("文字を入れる（set_value）", "pc_ui_do", {"window": "Notepad", "target": "Text Editor", "action": "set_value", "value": "set_value で入れた"})
    look("メモ帳の中身", "pc_ui", {"window": "notepad", "filter": "入れた"})
    look("メモ帳の窓", "pc_screenshot", {"window": "notepad"})
    look("メモ帳を閉じる（保存しない）", "pc_powershell", {"script": "Get-Process notepad -ErrorAction SilentlyContinue | Stop-Process -Force; 'closed'"})

print("=== 6. オフにすると常駐役が終わる")
set_mode("off")
time.sleep(4)
check("状態", "pc_status", {}, False, "動いていない")

print()
print("記録（audit.log）の最後：")
try:
    print((SD / "audit.log").read_text(encoding="utf-8").splitlines()[-5:])
except Exception as e:
    print(e)
if fails:
    print("NG：", fails)
    sys.exit(1)
print("全部 OK")
