$top = [int]$A.top; if ($top -le 0) { $top = 20 }
$ps = Get-Process | Where-Object { -not $A.name -or $_.ProcessName -like "*$($A.name)*" }
$cores = [Environment]::ProcessorCount
if ($A.sort -eq 'cpu') {
  # 1 秒あいだを空けて、その間の CPU の使い具合を測る
  $t1 = @{}; foreach ($p in $ps) { try { $t1[$p.Id] = $p.TotalProcessorTime.TotalMilliseconds } catch {} }
  Start-Sleep -Milliseconds 1000
  $rows = foreach ($p in $ps) { try { $p.Refresh(); $d = $p.TotalProcessorTime.TotalMilliseconds - $t1[$p.Id]; [pscustomobject]@{ P = $p; Cpu = [math]::Round($d / 10 / $cores) } } catch {} }
  $rows = $rows | Sort-Object Cpu -Descending | Select-Object -First $top
  '{0,5}  {1,8}  {2,7}  {3}' -f 'CPU%', 'メモリ', 'pid', '名前'
  foreach ($r in $rows) { '{0,5}  {1,6} MB  {2,7}  {3}' -f $r.Cpu, [math]::Round($r.P.WorkingSet64 / 1MB), $r.P.Id, $r.P.ProcessName }
} else {
  # メモリ（同じ名前はまとめる）
  $g = $ps | Group-Object ProcessName | ForEach-Object { [pscustomobject]@{ Name = $_.Name; N = $_.Count; Mem = ($_.Group | Measure-Object WorkingSet64 -Sum).Sum } } | Sort-Object Mem -Descending | Select-Object -First $top
  $os = Get-CimInstance Win32_OperatingSystem
  '全体：使用 {0} GB / {1} GB' -f [math]::Round(($os.TotalVisibleMemorySize - $os.FreePhysicalMemory) / 1MB, 1), [math]::Round($os.TotalVisibleMemorySize / 1MB, 1)
  foreach ($x in $g) { '{0,7} MB  {1}（{2} 個）' -f [math]::Round($x.Mem / 1MB), $x.Name, $x.N }
}
