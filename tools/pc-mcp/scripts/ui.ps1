$w = Find-PcWindow $A.window
if (-not $w) { 'ERROR: 窓「{0}」が見つかりません（pc_windows で確かめる）' -f $A.window; return }
$depth = [int]$A.depth; if ($depth -le 0) { $depth = 8 }
$max = [int]$A.max; if ($max -le 0) { $max = 300 }
$f = [string]$A.filter
$walker = [System.Windows.Automation.TreeWalker]::ControlViewWalker
$cnt = @{ n = 0 }
'■ 窓「{0}」 {1}（pid {2}） {3}' -f $w.Name, $w.Proc, $w.Pid, (Fmt-Rect $w.Rect)
function Walk($el, $d) {
  if ($d -gt $depth -or $cnt.n -ge $max) { return }
  $c = $walker.GetFirstChild($el)
  while ($c -and $cnt.n -lt $max) {
    try {
      $cur = $c.Current
      $t = $cur.ControlType.ProgrammaticName -replace '^ControlType\.', ''
      $val = ''
      try { $vp = $c.GetCurrentPattern([System.Windows.Automation.ValuePattern]::Pattern); $val = $vp.Current.Value } catch {}
      if (-not $val) { try { $tx = $c.GetCurrentPattern([System.Windows.Automation.TextPattern]::Pattern); $val = $tx.DocumentRange.GetText(300) } catch {} }
      $tog = ''
      try { $tp = $c.GetCurrentPattern([System.Windows.Automation.TogglePattern]::Pattern); $tog = ' [' + $tp.Current.ToggleState + ']' } catch {}
      $show = ($cur.Name -or $cur.AutomationId -or $val) -and (-not $cur.IsOffscreen -or $A.all)
      if ($show -and (-not $f -or $cur.Name -like "*$f*" -or $cur.AutomationId -like "*$f*" -or $val -like "*$f*")) {
        $line = ('  ' * $d) + $t + ' 「' + $cur.Name + '」'
        if ($cur.AutomationId) { $line += ' id=' + $cur.AutomationId }
        if ($val) { if ($val.Length -gt 80) { $val = $val.Substring(0, 80) + '…' }; $line += ' 値=「' + $val + '」' }
        $line += $tog
        if (-not $cur.IsEnabled) { $line += ' （無効）' }
        $line += ' ' + (Fmt-Rect $cur.BoundingRectangle)
        $line
        $cnt.n++
      }
      Walk $c ($d + 1)
    } catch {}
    $c = $walker.GetNextSibling($c)
  }
}
Walk $w.El 1
if ($cnt.n -ge $max) { '  （{0} 個で打ち切り。filter で絞る）' -f $max }
