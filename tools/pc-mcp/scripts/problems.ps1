# 不具合の記録：イベントログ（アプリ・システム）、エラー報告（WER）、クラッシュのダンプ、信頼性モニター
$hours = [int]$A.hours; if ($hours -le 0) { $hours = 24 }
$max = [int]$A.max; if ($max -le 0) { $max = 40 }
$app = [string]$A.app
$since = (Get-Date).AddHours(-$hours)
$levels = @(1, 2); if ($A.warnings) { $levels += 3 }
"■ イベントログ（$hours 時間以内・エラー" + $(if ($A.warnings) { '・警告' } else { '' }) + $(if ($app) { "・「$app」を含む" } else { '' }) + '）'
$ev = @()
foreach ($log in 'Application', 'System') {
  try { $ev += Get-WinEvent -FilterHashtable @{ LogName = $log; Level = $levels; StartTime = $since } -MaxEvents 2000 -ErrorAction Stop } catch {}
}
if ($app) { $ev = @($ev | Where-Object { $_.ProviderName -like "*$app*" -or $_.Message -like "*$app*" }) }
$ev = @($ev | Sort-Object TimeCreated -Descending)
if (-not $ev.Count) { '  なし' }
$lv = @{ 1 = '重大'; 2 = 'エラー'; 3 = '警告' }
foreach ($e in ($ev | Select-Object -First $max)) {
  $msg = ($e.Message -split "`r?`n" | Where-Object { $_.Trim() } | Select-Object -First 2) -join ' / '
  if ($msg.Length -gt 220) { $msg = $msg.Substring(0, 220) + '…' }
  '  {0:MM-dd HH:mm} {1} {2} {3} id={4}：{5}' -f $e.TimeCreated, $e.LogName, $lv[[int]$e.Level], $e.ProviderName, $e.Id, $msg
}
if ($ev.Count -gt $max) { '  ほか {0} 件' -f ($ev.Count - $max) }
# まとめ：どのアプリが何回落ちたか（Application Error 1000 / Application Hang 1002）
$crash = @($ev | Where-Object { $_.Id -in 1000, 1002 -and $_.ProviderName -in 'Application Error', 'Application Hang' })
if ($crash.Count) {
  '■ 落ちた・固まったアプリ（回数）'
  $crash | ForEach-Object { if ($_.Properties.Count) { [string]$_.Properties[0].Value } else { '?' } } | Group-Object | Sort-Object Count -Descending | ForEach-Object { '  {0} 回  {1}' -f $_.Count, $_.Name }
}
'■ エラー報告（WER）の控え（新しい順）'
$wer = @()
foreach ($d in "$env:LOCALAPPDATA\Microsoft\Windows\WER\ReportArchive", "$env:LOCALAPPDATA\Microsoft\Windows\WER\ReportQueue", "$env:ProgramData\Microsoft\Windows\WER\ReportArchive", "$env:ProgramData\Microsoft\Windows\WER\ReportQueue") {
  if (Test-Path $d) { $wer += Get-ChildItem $d -Directory -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -ge $since } }
}
if ($app) { $wer = @($wer | Where-Object { $_.Name -like "*$app*" }) }
if (-not $wer.Count) { '  なし' }
foreach ($w in ($wer | Sort-Object LastWriteTime -Descending | Select-Object -First 15)) { '  {0:MM-dd HH:mm}  {1}  {2}' -f $w.LastWriteTime, $w.Name, $w.FullName }
'■ クラッシュのダンプ（%LOCALAPPDATA%\CrashDumps）'
$cd = "$env:LOCALAPPDATA\CrashDumps"
$dumps = @(); if (Test-Path $cd) { $dumps = @(Get-ChildItem $cd -File -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTime -ge $since }) }
if ($app) { $dumps = @($dumps | Where-Object { $_.Name -like "*$app*" }) }
if (-not $dumps.Count) { '  なし' }
foreach ($f in ($dumps | Sort-Object LastWriteTime -Descending | Select-Object -First 10)) { '  {0:MM-dd HH:mm}  {1} MB  {2}' -f $f.LastWriteTime, [math]::Round($f.Length / 1MB), $f.Name }
if ($A.reliability) {
  '■ 信頼性モニター'
  try {
    $t = [Management.ManagementDateTimeConverter]::ToDmtfDateTime($since)
    $rr = @(Get-CimInstance Win32_ReliabilityRecords -Filter "TimeGenerated > '$t'" -ErrorAction Stop)
    if ($app) { $rr = @($rr | Where-Object { $_.ProductName -like "*$app*" -or $_.Message -like "*$app*" }) }
    if (-not $rr.Count) { '  なし' }
    foreach ($r in ($rr | Sort-Object TimeGenerated -Descending | Select-Object -First $max)) {
      $m = ($r.Message -split "`r?`n")[0]; if ($m.Length -gt 200) { $m = $m.Substring(0, 200) + '…' }
      '  {0:MM-dd HH:mm}  {1}  {2}' -f $r.TimeGenerated, $r.ProductName, $m
    }
  } catch { '  読めない：' + $_.Exception.Message }
}
