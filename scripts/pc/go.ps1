# go.ps1 — 「計画ファイル」plan.json のとおりに iPhone を見て・動かして・記録する。
# 使い方：同じフォルダに plan.json を置いて go.bat をダブルクリック。
# 結果：runs\<日時>-<name>\ に一式、同じフォルダの last-result.txt に要約（Claude が読むのはここ）。
# iPhone には何もインストールしない・何も消さない・設定も変えない。最後に起動したものはすべて止める。
#
# plan.json の例:
# {
#   "name": "swipe",
#   "match": "DEVKIT",              // iPhone のログから抜き出す文字（正規表現）。空なら抜き出さない
#   "wda": true,                    // WebDriverAgent を使うか（false ならログと画面だけ）
#   "target": "livecontainer",      // 前に出すアプリ（livecontainer か bundle id）。省略可
#   "seconds": 10,                  // wda=false のとき、ログを取る秒数
#   "actions": [
#     {"do": "activate"},
#     {"do": "screenshot", "name": "before"},
#     {"do": "drag", "from": [195,250], "to": [195,550], "duration": 0.2},
#     {"do": "tap", "at": [195,400]},
#     {"do": "wait", "sec": 2},
#     {"do": "source"},              // 画面の部品一覧（XML）を保存
#     {"do": "screenshot", "name": "after"}
#   ]
# }
$ErrorActionPreference = "Continue"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$cfgPath = Join-Path $env:USERPROFILE "devkit\config.ps1"
$last = Join-Path $here "last-result.txt"
"# go  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" | Set-Content $last -Encoding UTF8
function S([string]$t) { Write-Host $t; $t | Add-Content $last -Encoding UTF8; if ($script:runLog) { $t | Add-Content $script:runLog -Encoding UTF8 } }

if (-not (Test-Path $cfgPath)) { S "!! 設定がありません: $cfgPath（scripts/pc/config.example.ps1 を参照）"; exit 1 }
. $cfgPath
$ios = $DevkitConfig.IosExe
$planPath = Join-Path $here "plan.json"
if (-not (Test-Path $planPath)) { S "!! plan.json がありません"; exit 1 }
try { $plan = Get-Content $planPath -Raw -Encoding UTF8 | ConvertFrom-Json } catch { S "!! plan.json が読めません: $($_.Exception.Message)"; exit 1 }
$name = if ($plan.name) { $plan.name } else { "run" }
$runDir = Join-Path $here ("runs\" + (Get-Date -Format "yyyyMMdd-HHmmss") + "-" + $name)
New-Item -ItemType Directory -Force $runDir | Out-Null
$script:runLog = Join-Path $runDir "result.txt"
Copy-Item $planPath (Join-Path $runDir "plan.json")
S "run: $runDir"

$procs = @()
function Bg([string]$n, [string[]]$a) {
    $p = Start-Process -FilePath $ios -ArgumentList $a -NoNewWindow -PassThru `
        -RedirectStandardOutput (Join-Path $runDir "$n-out.txt") -RedirectStandardError (Join-Path $runDir "$n-err.txt")
    $script:procs += $p
    return $p
}
function Wda([string]$method, [string]$path, $body) {
    $uri = "http://127.0.0.1:8100$path"
    try {
        if ($null -ne $body) { $r = Invoke-RestMethod -Method $method -Uri $uri -Body ($body | ConvertTo-Json -Depth 6 -Compress) -ContentType "application/json" -TimeoutSec 60 }
        else { $r = Invoke-RestMethod -Method $method -Uri $uri -TimeoutSec 60 }
        return $r
    } catch { S "   WDA $method $path 失敗: $($_.Exception.Message)"; return $null }
}
function Shot([string]$n) {
    $f = Join-Path $runDir "$n.png"
    & $ios screenshot "--output=$f" 2>$null | Out-Null
    if (Test-Path $f) { S "   画面: $n.png" } else { S "   画面を取れず: $n" }
}

try {
    S "== 1. ログ取り（トンネルより先に始める）→ トンネル"
    Bg "syslog" @("syslog") | Out-Null
    Start-Sleep -Seconds 3
    Bg "tunnel" @("tunnel","start","--userspace") | Out-Null
    Start-Sleep -Seconds 15

    $target = $plan.target
    if ($target -eq "livecontainer") { $target = $DevkitConfig.LiveContainer }

    if ($plan.wda) {
        S "== 2. WebDriverAgent を起動"
        $line = & $ios apps --list 2>$null | ForEach-Object { "$_" } | Where-Object { $_ -match "WebDriverAgent" } | Select-Object -First 1
        $bid = ($line -split " ")[0]
        if (-not $bid) { throw "WebDriverAgent が iPhone に見つかりません（SideStore で WebDriverAgentRunner-ss.ipa を入れる）" }
        S "   WDA: $bid / テスト本体: $($DevkitConfig.WdaTestName)"
        $w = Bg "runwda" @("runwda","--bundleid=$bid","--testrunnerbundleid=$bid","--xctestconfig=$($DevkitConfig.WdaTestName)","--log-output=$(Join-Path $runDir 'wda-testlog.txt')")
        Bg "forward" @("forward","8100","8100") | Out-Null
        $ok = $false
        for ($i = 0; $i -lt 18; $i++) {
            Start-Sleep -Seconds 5
            if ($w.HasExited) { break }
            try { Invoke-RestMethod -Uri "http://127.0.0.1:8100/status" -TimeoutSec 4 | Out-Null; $ok = $true; break } catch { }
        }
        if (-not $ok) {
            Get-Content (Join-Path $runDir "runwda-err.txt") -ErrorAction SilentlyContinue | Where-Object { $_ -match '"level":"ERROR"' } | Select-Object -Last 5 | ForEach-Object { S "   $_" }
            throw "WebDriverAgent が返事をしません"
        }
        S "   返事あり（$($i*5+5) 秒）"
        $s = Wda POST "/session" @{ capabilities = @{ alwaysMatch = @{}; firstMatch = @(@{}) } }
        $sid = $null; if ($s) { $sid = $s.sessionId; if (-not $sid) { $sid = $s.value.sessionId } }
        if (-not $sid) { throw "セッションを作れません" }
        $size = Wda GET "/session/$sid/window/size" $null
        if ($size) { S "   画面の大きさ: $($size.value.width) x $($size.value.height)（座標はこの単位）" }

        S "== 3. 操作"
        $k = 0
        foreach ($a in $plan.actions) {
            $k++
            $t = Get-Date -Format "HH:mm:ss.fff"
            switch ($a.do) {
                "activate"   { $b = if ($a.bundle) { $a.bundle } else { $target }; $r = Wda POST "/session/$sid/wda/apps/activate" @{ bundleId = $b }; S "   [$k $t] activate $b"; Start-Sleep -Seconds 2 }
                "drag"       { $d = if ($a.duration) { $a.duration } else { 0.2 }; $r = Wda POST "/session/$sid/wda/dragfromtoforduration" @{ fromX = $a.from[0]; fromY = $a.from[1]; toX = $a.to[0]; toY = $a.to[1]; duration = $d }; S "   [$k $t] drag $($a.from -join ',') -> $($a.to -join ',')" }
                "tap"        { $r = Wda POST "/session/$sid/wda/tap" @{ x = $a.at[0]; y = $a.at[1] }; S "   [$k $t] tap $($a.at -join ',')" }
                "wait"       { Start-Sleep -Seconds $a.sec; S "   [$k $t] wait $($a.sec)" }
                "screenshot" { S "   [$k $t] screenshot"; Shot $(if ($a.name) { $a.name } else { "shot$k" }) }
                "source"     { $r = Wda GET "/session/$sid/source" $null; if ($r) { $r.value | Set-Content (Join-Path $runDir "source$k.xml") -Encoding UTF8; S "   [$k $t] source -> source$k.xml" } }
                default      { S "   [$k $t] 知らない操作: $($a.do)" }
            }
        }
    } else {
        $sec = if ($plan.seconds) { [int]$plan.seconds } else { 10 }
        S "== 2. ログと画面だけ（$sec 秒）"
        Shot "before"
        Start-Sleep -Seconds $sec
        Shot "after"
    }
}
catch { S "!! エラー: $($_.Exception.Message)" }
finally {
    Start-Sleep -Seconds 2
    foreach ($p in $procs) { if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } }
    S "== 起動したもの（ログ取り・トンネル・WDA・転送）を止めました"
    $sys = Join-Path $runDir "syslog-out.txt"
    if ($plan.match) {
        $m = Select-String -Path $sys -Pattern $plan.match -ErrorAction SilentlyContinue | ForEach-Object { $_.Line }
        $m | Set-Content (Join-Path $runDir "syslog-match.txt") -Encoding UTF8
        S "-- iPhone のログで「$($plan.match)」に合う行（$(@($m).Count) 行、最後の 60 行）"
        $m | Select-Object -Last 60 | ForEach-Object { S $_ }
    }
    S "== 完了"
}
