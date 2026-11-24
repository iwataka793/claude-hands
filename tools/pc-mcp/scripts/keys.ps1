if ($A.window) { $w = Find-PcWindow $A.window; if (-not $w) { 'ERROR: 窓「{0}」が見つかりません' -f $A.window; return }; Activate-PcWindow $w }
$fg = [PcNative]::GetForegroundWindow(); $fpid = 0; [void][PcNative]::GetWindowThreadProcessId($fg, [ref]$fpid)
$pn = ''; try { $pn = (Get-Process -Id $fpid -ErrorAction Stop).ProcessName } catch {}
if ($A.text -and $A.paste) {
  # 日本語など：クリップボード経由で貼る（前のクリップボードは戻す）
  $old = $null; try { $old = Get-Clipboard -Raw -ErrorAction Stop } catch {}
  Set-Clipboard -Value ([string]$A.text); Start-Sleep -Milliseconds 100
  [System.Windows.Forms.SendKeys]::SendWait('^v'); Start-Sleep -Milliseconds 200
  if ($null -ne $old) { Set-Clipboard -Value $old }
} elseif ($A.text) {
  # 文字はそのまま打つ（SendKeys の記号を逃がす）
  $t = [regex]::Replace([string]$A.text, '[+^%~(){}\[\]]', '{$0}')
  $t = $t -replace "`r?`n", '{ENTER}'
  [System.Windows.Forms.SendKeys]::SendWait($t)
}
if ($A.keys) { [System.Windows.Forms.SendKeys]::SendWait([string]$A.keys) }
if ($A.enter) { [System.Windows.Forms.SendKeys]::SendWait('{ENTER}') }
'打った（前の窓：{0}）' -f $pn
