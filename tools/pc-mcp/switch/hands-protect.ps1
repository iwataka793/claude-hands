# claude-hands：守りの場所を作る（pc-setup.bat から管理者で 1 回。UAC はカオル）
#   %ProgramData%\claude-hands\holding  取り置き。カオルの普通の権限（＝常駐役・Claude）は「入れる」だけ。消す・書き換える・取り出すはできない
#   %ProgramData%\claude-hands\status   毎日の点検の結果（読むだけ）
#   %ProgramData%\claude-hands\bin      毎日の点検の台本（読むだけ）
#   %ProgramData%\claude-hands\git      作業の前後の Git の記録（常駐役が書く）
#   %ProgramData%\claude-hands\keys     頼みの署名の合言葉（request.key。管理者だけが作れる・カオルの権限では読むだけ）
#   予定のタスク \claude-hands\RestorePoint（SYSTEM・毎日 12:00）：復元ポイント・30 日たった取り置きを消す・結果を status に書く
param([Parameter(Mandatory)][string]$User)
$ErrorActionPreference = 'Stop'
$log = Join-Path $env:TEMP 'claude-hands-protect.txt'
Start-Transcript -Path $log -Force | Out-Null
try {
  $H = Join-Path $env:ProgramData 'claude-hands'
  foreach ($d in $H, "$H\holding", "$H\status", "$H\bin", "$H\git", "$H\keys") { New-Item -ItemType Directory -Force -Path $d | Out-Null }
  $sys = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-18')
  $adm = New-Object System.Security.Principal.SecurityIdentifier('S-1-5-32-544')
  $own = New-Object System.Security.Principal.SecurityIdentifier('S-1-3-4')   # OWNER RIGHTS：作った人（持ち主）に自動で付く権限（権限の書き換え）を消す
  $usr = (New-Object System.Security.Principal.NTAccount($User)).Translate([System.Security.Principal.SecurityIdentifier])
  function Rule($id, $rights, $inh, $prop) { New-Object System.Security.AccessControl.FileSystemAccessRule($id, $rights, $inh, $prop, 'Allow') }
  function Protect($path, $userRules) {
    $acl = New-Object System.Security.AccessControl.DirectorySecurity
    $acl.SetAccessRuleProtection($true, $false)
    $acl.SetOwner($adm)
    $acl.AddAccessRule((Rule $sys 'FullControl' 'ContainerInherit,ObjectInherit' 'None'))
    $acl.AddAccessRule((Rule $adm 'FullControl' 'ContainerInherit,ObjectInherit' 'None'))
    foreach ($r in $userRules) { $acl.AddAccessRule($r) }
    Set-Acl -LiteralPath $path -AclObject $acl
  }
  $read = @((Rule $usr 'ReadAndExecute' 'ContainerInherit,ObjectInherit' 'None'))
  Protect $H $read
  Protect "$H\status" $read
  Protect "$H\bin" $read
  # 頼みの署名の合言葉：道具（PC・iPhone）が頼みに署名し、スイッチの窓が確かめる。無いときだけ作る（作り直すと古い頼みは通らない）
  Protect "$H\keys" $read
  $key = "$H\keys\request.key"
  if (-not (Test-Path -LiteralPath $key)) {
    $b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
    [IO.File]::WriteAllText($key, (($b | ForEach-Object { $_.ToString('x2') }) -join ''))
  }
  # Git の記録（作業の前後）：常駐役が書くので、カオルの権限で書き換えられる。文字の守りで Claude の台本からは触らせない
  Protect "$H\git" @((Rule $usr 'Modify' 'ContainerInherit,ObjectInherit' 'None'))
  Protect "$H\holding" @(
    (Rule $usr 'ReadAndExecute, CreateFiles, CreateDirectories' 'ContainerInherit' 'None'),   # フォルダ：中に作れる
    (Rule $usr 'Read, WriteAttributes' 'ObjectInherit' 'InheritOnly'),                         # ファイル：読むだけ
    (Rule $own 'ReadAndExecute' 'ContainerInherit,ObjectInherit' 'None'))
  Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'hands-daily.ps1') -Destination "$H\bin\hands-daily.ps1" -Force
  # 毎日の復元ポイントが「24 時間以内」で飛ばされないように
  New-ItemProperty -Path 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore' -Name SystemRestorePointCreationFrequency -PropertyType DWord -Value 0 -Force | Out-Null
  $act = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$H\bin\hands-daily.ps1`""
  $trg = New-ScheduledTaskTrigger -Daily -At 12:00
  $set = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Minutes 30)
  $pri = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
  Register-ScheduledTask -TaskPath '\claude-hands\' -TaskName 'RestorePoint' -Action $act -Trigger $trg -Settings $set -Principal $pri -Description 'claude-hands：毎日 1 回、復元ポイント・30 日たった取り置きを消す・結果を status\daily.txt に' -Force | Out-Null
  Start-ScheduledTask -TaskPath '\claude-hands\' -TaskName 'RestorePoint'
  "OK 守りの場所：$H"
} catch {
  "NG：$($_.Exception.Message)"
} finally {
  Stop-Transcript | Out-Null
}
