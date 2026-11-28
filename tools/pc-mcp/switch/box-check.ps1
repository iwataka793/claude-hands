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
  'NG 箱の守りの確かめ：管理者なしで箱の設定を書けてしまった（EditAdminOnly が効いていない）'
} else {
  'OK 箱の守りの確かめ：管理者なしでは箱の設定を書けない'
}
