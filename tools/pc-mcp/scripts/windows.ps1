'■ 開いている窓（前から順ではない）'
$fg = [int64][PcNative]::GetForegroundWindow()
foreach ($w in Get-PcWindows) {
  if ($w.Off -and -not $A.all) { continue }
  $mark = ''; if ([int64]$w.Handle -eq $fg) { $mark = ' ← いま前' }
  '  「{0}」 {1}（pid {2}） {3}{4}' -f $w.Name, $w.Proc, $w.Pid, (Fmt-Rect $w.Rect), $mark
}
