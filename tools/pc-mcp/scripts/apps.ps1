$f = [string]$A.filter
if ($A.kind -eq 'startup') {
  '■ PC を起動したときに動くもの'
  Get-CimInstance Win32_StartupCommand | Where-Object { -not $f -or $_.Name -like "*$f*" -or $_.Command -like "*$f*" } | Sort-Object Name | ForEach-Object { '  {0}  [{1}]  {2}' -f $_.Name, $_.Location, $_.Command }
  return
}
$keys = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*', 'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*', 'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*'
$apps = Get-ItemProperty $keys -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -and -not $_.SystemComponent -and (-not $f -or $_.DisplayName -like "*$f*" -or $_.Publisher -like "*$f*") } |
  Sort-Object DisplayName -Unique
switch ($A.sort) {
  'size' { $apps = $apps | Sort-Object { [int64]$_.EstimatedSize } -Descending }
  'date' { $apps = $apps | Sort-Object InstallDate -Descending }
}
$max = [int]$A.max; if ($max -le 0) { $max = 300 }
'■ 入っているアプリ（{0} 個）' -f @($apps).Count
foreach ($x in ($apps | Select-Object -First $max)) {
  $sz = ''; if ($x.EstimatedSize) { $sz = '{0} MB' -f [math]::Round([int64]$x.EstimatedSize / 1024) }
  '  {0}  {1}  {2}  {3}  {4}' -f $x.DisplayName, $x.DisplayVersion, $x.Publisher, $x.InstallDate, $sz
}
if ($A.store) {
  '■ ストアのアプリ'
  Get-AppxPackage | Where-Object { -not $_.IsFramework -and $_.SignatureKind -ne 'System' -and (-not $f -or $_.Name -like "*$f*") } | Sort-Object Name | ForEach-Object { '  {0}  {1}' -f $_.Name, $_.Version }
}
