# claude-hands：毎日 1 回の点検（予定のタスク \claude-hands\RestorePoint。SYSTEM で動く）
$H = Split-Path $PSScriptRoot -Parent
$hold = Join-Path $H 'holding'
$out = New-Object System.Collections.Generic.List[string]
$out.Add("時刻：$(Get-Date -Format 's')")
try {
  Checkpoint-Computer -Description 'claude-hands daily' -RestorePointType MODIFY_SETTINGS -ErrorAction Stop
  $out.Add('復元ポイント：作った')
} catch { $out.Add("復元ポイント：作れなかった（$($_.Exception.Message)）") }
$rp = Get-ComputerRestorePoint -ErrorAction SilentlyContinue | Select-Object -Last 1
if ($rp) { $out.Add("最後の復元ポイント：$($rp.ConvertToDateTime($rp.CreationTime).ToString('yyyy-MM-dd HH:mm')) $($rp.Description)") }
$n = 0
Get-ChildItem -LiteralPath $hold -Directory -Force -ErrorAction SilentlyContinue | Where-Object { $_.CreationTime -lt (Get-Date).AddDays(-30) } | ForEach-Object {
  try { Remove-Item -LiteralPath $_.FullName -Recurse -Force -ErrorAction Stop; $n++ } catch { $out.Add("消せなかった取り置き：$($_.Name)（$($_.Exception.Message)）") }
}
$out.Add("期限で消した取り置き：$n 件")
$free = (Get-PSDrive C -ErrorAction SilentlyContinue).Free
if ($free) { $out.Add("C の空き：$([math]::Floor($free / 1GB)) GB") }
Set-Content -LiteralPath (Join-Path $H 'status\daily.txt') -Value $out -Encoding UTF8
