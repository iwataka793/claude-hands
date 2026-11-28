# claude-hands：箱（Sandboxie）の設定を、カオルの普通の権限（＝Claude の道具と同じ）では変えられないことを確かめる
#   pc-setup.bat が管理者の台本（hands-protect.ps1）のあとに、管理者なしで動かす。試しの名前を 1 つ書こうとして、読み直す
$ini = Join-Path $env:ProgramFiles 'Sandboxie-Plus\SbieIni.exe'
if (-not (Test-Path -LiteralPath $ini)) { 'OK 箱の守りの確かめ：Sandboxie-Plus が入っていないので飛ばした'; return }
$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($admin) { 'NG 箱の守りの確かめ：管理者の権限で動いている（管理者なしで動かす）'; return }
& $ini set ClaudeBox ClaudeHandsEditTest 1 | Out-Null
$v = & $ini query ClaudeBox ClaudeHandsEditTest
if ($v) {
  & $ini set ClaudeBox ClaudeHandsEditTest | Out-Null
  'NG 箱の守りの確かめ：管理者なしで箱の設定を書けてしまった（Sandboxie の設定のパスワードが入っていない）'
} else {
  'OK 箱の守りの確かめ：管理者なしでは箱の設定を書けない'
}

# ネット：箱の中から直接は外へつながらないこと（中継の口 127.0.0.1:60230 だけ通る決まり。常駐役はこのとき止まっているので中継は試さない）
$start = Join-Path $env:ProgramFiles 'Sandboxie-Plus\Start.exe'
$out = Join-Path $env:TEMP 'claude-hands-boxnet.txt'
Remove-Item -LiteralPath $out -ErrorAction SilentlyContinue
$t = Join-Path $env:TEMP 'claude-hands-boxnet.ps1'
@'
param($out)
$r = foreach ($h in 'example.com', '1.1.1.1') {
  try { $c = New-Object Net.Sockets.TcpClient; $a = $c.BeginConnect($h, 443, $null, $null); if ($a.AsyncWaitHandle.WaitOne(5000) -and $c.Connected) { "$h つながった" } else { "$h つながらない" }; $c.Close() } catch { "$h つながらない" }
}
$r | Set-Content -LiteralPath $out -Encoding UTF8
'@ | Set-Content -LiteralPath $t -Encoding UTF8
Start-Process $start -ArgumentList '/box:ClaudeBox', '/wait', 'powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $t, $out -Wait -WindowStyle Hidden
$res = @()
$boxOut = Join-Path ('C:\Sandbox\' + $env:USERNAME + '\ClaudeBox\user\current') ($out.Substring($env:USERPROFILE.Length).TrimStart('\'))
foreach ($f in $out, $boxOut) { if (Test-Path -LiteralPath $f) { $res = Get-Content -LiteralPath $f -Encoding UTF8 } }
Start-Process $start -ArgumentList '/box:ClaudeBox', 'delete_sandbox_silent' -Wait
Remove-Item -LiteralPath $t -ErrorAction SilentlyContinue
if (-not $res) { 'NG 箱のネットの確かめ：箱の中の結果が読めなかった' }
elseif ($res -match 'つながった') { 'NG 箱のネットの確かめ：箱から直接外へつながった（' + ($res -join '・') + '）' }
else { 'OK 箱のネットの確かめ：箱から直接は外へつながらない（' + ($res -join '・') + '）' }

# 見せない場所（box-closed.ps1）：箱の中から読めないこと。中身は読まない・記録しない（開けるかどうかだけ。名前も数だけ）
. (Join-Path $PSScriptRoot 'box-closed.ps1')
$list = @(ClaudeBoxClosed $env:USERPROFILE (Join-Path $env:ProgramData 'claude-hands') | Where-Object { Test-Path -LiteralPath $_[1] })
$realCred = @(cmdkey /list 2>$null | Where-Object { $_ -match '^\s*(Target|ターゲット)\s*:' }).Count
$in2 = Join-Path $env:TEMP 'claude-hands-boxclosed-in.txt'
$out2 = Join-Path $env:TEMP 'claude-hands-boxclosed.txt'
$t2 = Join-Path $env:TEMP 'claude-hands-boxclosed.ps1'
Remove-Item -LiteralPath $out2 -ErrorAction SilentlyContinue
@($list | ForEach-Object { $_[1] }) | Set-Content -LiteralPath $in2 -Encoding UTF8
@'
param($in, $out)
$r = @(foreach ($p in @(Get-Content -LiteralPath $in -Encoding UTF8)) {
  try {
    if (-not (Test-Path -LiteralPath $p)) { 'NO' }
    elseif (Test-Path -LiteralPath $p -PathType Container) { $null = @([IO.Directory]::EnumerateFileSystemEntries($p) | Select-Object -First 1); 'READ' }
    else { $f = [IO.File]::Open($p, 'Open', 'Read', 'ReadWrite'); $f.Dispose(); 'READ' }
  } catch { 'NO' }
})
$r += 'CRED ' + @(cmdkey /list 2>$null | Where-Object { $_ -match '^\s*(Target|ターゲット)\s*:' }).Count
$r | Set-Content -LiteralPath $out -Encoding UTF8
'@ | Set-Content -LiteralPath $t2 -Encoding UTF8
Start-Process $start -ArgumentList '/box:ClaudeBox', '/wait', 'powershell.exe', '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $t2, $in2, $out2 -Wait -WindowStyle Hidden
$res2 = @()
$boxOut2 = Join-Path ('C:\Sandbox\' + $env:USERNAME + '\ClaudeBox\user\current') ($out2.Substring($env:USERPROFILE.Length).TrimStart('\'))
foreach ($f in $out2, $boxOut2) { if (Test-Path -LiteralPath $f) { $res2 = @(Get-Content -LiteralPath $f -Encoding UTF8) } }
Start-Process $start -ArgumentList '/box:ClaudeBox', 'delete_sandbox_silent' -Wait
Remove-Item -LiteralPath $t2, $in2, $out2 -ErrorAction SilentlyContinue
if ($res2.Count -ne $list.Count + 1) { 'NG 箱から見えない場所の確かめ：箱の中の結果が読めなかった' }
else {
  $readable = @(for ($i = 0; $i -lt $list.Count; $i++) { if ($res2[$i] -eq 'READ') { $list[$i][0] } })
  if ($readable.Count) { 'NG 箱から読めてしまう場所：' + ($readable -join '・') }
  else { 'OK 箱から見えない場所：本物にある ' + $list.Count + ' か所は、箱の中からどれも開けない' }
  $boxCred = [int](($res2[-1]) -replace '^CRED ', '')
  if ($realCred -gt 0 -and $boxCred -gt 0) { 'NG 箱の中から Windows の資格情報の一覧が見える（本物 ' + $realCred + ' 件・箱 ' + $boxCred + ' 件。中身は見ていない）' }
  else { 'OK 箱の中から Windows の資格情報の一覧は見えない（本物 ' + $realCred + ' 件・箱 ' + $boxCred + ' 件）' }
}
