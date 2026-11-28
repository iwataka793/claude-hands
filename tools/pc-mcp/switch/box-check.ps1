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
