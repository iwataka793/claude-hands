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
#     {"do": "tapText", "text": "Run", "index": 0},   // 文字で部品を探して押す
#     {"do": "terminate"},                             // アプリを終わらせる（bundle を書けばそのアプリ）
#     {"do": "wait", "sec": 2},
#     {"do": "source"},              // 画面の部品一覧（XML）を保存
#     {"do": "screenshot", "name": "after"}
#   ]
# }
# ファイルの操作（WDA なしでも使える。app は livecontainer か bundle id。開発者署名のアプリの中だけ）:
#     {"do": "tree", "app": "livecontainer", "path": "/Documents/Applications", "grep": "devkit"}  // 一覧は tree.txt、要約は grep に合う行だけ
#     {"do": "pull", "app": "livecontainer", "path": "/Documents/Applications/x.app"}             // runs\...\pulled\ に取り出す
#     {"do": "push", "app": "livecontainer", "local": "files\\a.txt", "remote": "/Documents/a.txt"} // local は devkit-setup からの場所
#     {"do": "pushdir", "app": "livecontainer", "local": "files\\x.app", "remote": "/Documents/Applications/x.app"}  // フォルダごと（上書き）
# 消す操作は入れていない。
$ErrorActionPreference = "Continue"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
# 設定は同じフォルダの config.ps1、無ければ %USERPROFILE%\devkit\config.ps1
$cfgPath = Join-Path $here "config.ps1"
if (-not (Test-Path $cfgPath)) { $cfgPath = Join-Path $env:USERPROFILE "devkit\config.ps1" }
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
        # 日本語などを正しく送るため UTF-8 のバイト列で送る（PowerShell 5.1 は文字列のままだと化ける）
        if ($null -ne $body) { $bytes = [System.Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 6 -Compress)); $r = Invoke-RestMethod -Method $method -Uri $uri -Body $bytes -ContentType "application/json; charset=utf-8" -TimeoutSec 60 }
        else { $r = Invoke-RestMethod -Method $method -Uri $uri -TimeoutSec 60 }
        return $r
    } catch {
        $detail = "$($_.ErrorDetails.Message)"
        if ($detail.Length -gt 400) { $detail = $detail.Substring(0, 400) + "…" }
        S "   WDA $method $path 失敗: $($_.Exception.Message) $detail"
        return $null
    }
}
function Shot([string]$n) {
    $f = Join-Path $runDir "$n.png"
    & $ios screenshot "--output=$f" 2>$null | Out-Null
    if (Test-Path $f) { S "   画面: $n.png" } else { S "   画面を取れず: $n" }
}

Set-Location (Split-Path -Parent $ios)   # go-ios の鍵（selfIdentity.plist）をいつもの場所で使う
function AppId([string]$x) { if ($x -eq "livecontainer") { return $DevkitConfig.LiveContainer } else { return $x } }
function Local([string]$x) { if ([System.IO.Path]::IsPathRooted($x)) { return $x } else { return (Join-Path $here $x) } }
function RunAction($a, [int]$k) {
    $t = Get-Date -Format "HH:mm:ss.fff"
    $needWda = @("activate","terminate","drag","tap","tapText","source")
    if (($needWda -contains $a.do) -and -not $script:sid) { S "   [$k $t] $($a.do)：WDA を使っていないのでできません（wda: true にする）"; return }
    $sid = $script:sid
    switch ($a.do) {
        "activate"   { $b = if ($a.bundle) { AppId $a.bundle } else { $script:target }; $r = Wda POST "/session/$sid/wda/apps/activate" @{ bundleId = $b }; S "   [$k $t] activate $b"; Start-Sleep -Seconds 2 }
        "terminate"  { $b = if ($a.bundle) { AppId $a.bundle } else { $script:target }; $r = Wda POST "/session/$sid/wda/apps/terminate" @{ bundleId = $b }; S "   [$k $t] terminate $b -> $($r.value)"; Start-Sleep -Seconds 1 }
        "tapText"    {
            # 画面の部品を文字（label / name / value）で探して押す。index は同じ文字が複数あるときの番号（0 から）
            $q = $a.text.Replace("'", "\'")
            $r = Wda POST "/session/$sid/elements" @{ using = "predicate string"; value = "label == '$q' OR name == '$q' OR value == '$q'" }
            $ids = @($r.value | ForEach-Object { if ($_.ELEMENT) { $_.ELEMENT } else { $_.'element-6066-11e4-a52e-4f735466cecf' } })
            $i = if ($a.index) { [int]$a.index } else { 0 }
            if ($ids.Count -le $i) { S "   [$k $t] tapText '$($a.text)'：見つからず（$($ids.Count) 個）"; return }
            $null = Wda POST "/session/$sid/element/$($ids[$i])/click" @{}
            S "   [$k $t] tapText '$($a.text)'（$($ids.Count) 個中 $i 番）"
        }
        "drag"       { $d = if ($a.duration) { $a.duration } else { 0.2 }; $r = Wda POST "/session/$sid/wda/dragfromtoforduration" @{ fromX = $a.from[0]; fromY = $a.from[1]; toX = $a.to[0]; toY = $a.to[1]; duration = $d }; S "   [$k $t] drag $($a.from -join ',') -> $($a.to -join ',')" }
        "tap"        { $r = Wda POST "/session/$sid/wda/tap" @{ x = $a.at[0]; y = $a.at[1] }; S "   [$k $t] tap $($a.at -join ',')" }
        "wait"       { Start-Sleep -Seconds $a.sec; S "   [$k $t] wait $($a.sec)" }
        "screenshot" { S "   [$k $t] screenshot"; Shot $(if ($a.name) { $a.name } else { "shot$k" }) }
        "source"     { $r = Wda GET "/session/$sid/source" $null; if ($r) { $r.value | Set-Content (Join-Path $runDir "source$k.xml") -Encoding UTF8; S "   [$k $t] source -> source$k.xml" } }
        "tree"       {
            $out = & $ios fsync "--app=$(AppId $a.app)" tree "--path=$($a.path)" 2>$null | ForEach-Object { "$_" }
            $out | Set-Content (Join-Path $runDir "tree$k.txt") -Encoding UTF8
            S "   [$k $t] tree $($a.path)：$(@($out).Count) 行 -> tree$k.txt"
            if ($a.grep) { $out | Where-Object { $_ -match $a.grep } | Select-Object -First 40 | ForEach-Object { S "      $_" } }
        }
        "pull"       {
            $dst = Join-Path $runDir "pulled"
            New-Item -ItemType Directory -Force $dst | Out-Null
            $o = & $ios fsync "--app=$(AppId $a.app)" pull "--srcPath=$($a.path)" "--dstPath=$dst" 2>&1 | Out-String
            S "   [$k $t] pull $($a.path)：終了コード $LASTEXITCODE"
            if ($LASTEXITCODE -ne 0) { ($o -split "`n") | Where-Object { $_ -match 'ERROR|error' } | Select-Object -Last 3 | ForEach-Object { S "      $_" } }
        }
        "push"       {
            $src = Local $a.local
            $o = & $ios fsync "--app=$(AppId $a.app)" push "--srcPath=$src" "--dstPath=$($a.remote)" 2>&1 | Out-String
            S "   [$k $t] push $($a.local) -> $($a.remote)：終了コード $LASTEXITCODE"
        }
        "pushdir"    {
            $src = (Resolve-Path (Local $a.local)).Path
            $app = AppId $a.app
            $null = & $ios fsync "--app=$app" mkdir "--path=$($a.remote)" 2>&1
            $bad = 0; $n = 0
            Get-ChildItem $src -Recurse | Sort-Object FullName | ForEach-Object {
                $rel = $_.FullName.Substring($src.Length).TrimStart('\').Replace('\','/')
                $remote = "$($a.remote)/$rel"
                if ($_.PSIsContainer) { $null = & $ios fsync "--app=$app" mkdir "--path=$remote" 2>&1 }
                else {
                    $null = & $ios fsync "--app=$app" push "--srcPath=$($_.FullName)" "--dstPath=$remote" 2>&1
                    $n++; if ($LASTEXITCODE -ne 0) { $bad++; S "      失敗: $rel" }
                }
            }
            S "   [$k $t] pushdir $($a.local) -> $($a.remote)：ファイル $n 個、失敗 $bad 個"
        }
        default      { S "   [$k $t] 知らない操作: $($a.do)" }
    }
}

try {
    S "== 1. ログ取り（トンネルより先に始める）→ トンネル"
    Bg "syslog" @("syslog") | Out-Null
    Start-Sleep -Seconds 3
    Bg "tunnel" @("tunnel","start","--userspace") | Out-Null
    Start-Sleep -Seconds 15

    $script:target = AppId $plan.target
    $script:sid = $null

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
        $script:sid = $sid
        $size = Wda GET "/session/$sid/window/size" $null
        if ($size) { S "   画面の大きさ: $($size.value.width) x $($size.value.height)（座標はこの単位）" }

    }
    S "== 3. 操作"
    if (-not $plan.actions) {
        $sec = if ($plan.seconds) { [int]$plan.seconds } else { 10 }
        S "   操作なし：ログを $sec 秒取って前後の画面を撮る"
        Shot "before"; Start-Sleep -Seconds $sec; Shot "after"
    }
    $k = 0
    foreach ($a in $plan.actions) { $k++; RunAction $a $k }
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
