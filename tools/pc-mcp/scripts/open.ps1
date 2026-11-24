$t = [string]$A.target
$p = @{ FilePath = $t; PassThru = $true; ErrorAction = 'Stop' }
if ($A.args) { $p.ArgumentList = [string]$A.args }
try { $pr = Start-Process @p; Start-Sleep -Milliseconds 800; if ($pr) { '開いた：{0}（pid {1}）' -f $t, $pr.Id } else { '開いた：{0}' -f $t } }
catch { 'ERROR: 開けません：' + $_.Exception.Message }
