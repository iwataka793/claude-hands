Add-Type -AssemblyName Microsoft.VisualBasic
foreach ($p in $A.paths) {
  try {
    if (Test-Path -LiteralPath $p -PathType Container) {
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
    } elseif (Test-Path -LiteralPath $p) {
      [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
    } else { 'ERROR: 無い：' + $p; continue }
    if (Test-Path -LiteralPath $p) { 'ERROR: 残っている：' + $p } else { 'ごみ箱へ：' + $p }
  } catch { 'ERROR: ' + $p + '：' + $_.Exception.Message }
}
