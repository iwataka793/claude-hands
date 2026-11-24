$act = [string]$A.action
function Click-At($x, $y, $kind) {
  [void][PcNative]::SetCursorPos([int]$x, [int]$y); Start-Sleep -Milliseconds 50
  switch ($kind) {
    'right' { [PcNative]::mouse_event(0x0008, 0, 0, 0, [UIntPtr]::Zero); [PcNative]::mouse_event(0x0010, 0, 0, 0, [UIntPtr]::Zero) }
    'double' { 1..2 | ForEach-Object { [PcNative]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero); [PcNative]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero); Start-Sleep -Milliseconds 60 } }
    default { [PcNative]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero); [PcNative]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero) }
  }
}
$w = $null
if ($A.window) { $w = Find-PcWindow $A.window; if (-not $w) { 'ERROR: 窓「{0}」が見つかりません' -f $A.window; return }; Activate-PcWindow $w }
if ($act -in 'click_xy', 'double_xy', 'right_xy') {
  $k = 'left'; if ($act -eq 'double_xy') { $k = 'double' } elseif ($act -eq 'right_xy') { $k = 'right' }
  Click-At $A.x $A.y $k
  '画面の ({0},{1}) を押した（{2}）' -f [int]$A.x, [int]$A.y, $act
  return
}
if (-not $w) { 'ERROR: window（窓の題名の一部・プログラム名・pid）を指定してください'; return }
$target = [string]$A.target
$all = $w.El.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
$m = @()
foreach ($e in $all) {
  try {
    $c = $e.Current
    $t = $c.ControlType.ProgrammaticName -replace '^ControlType\.', ''
    if ($A.type -and $t -ne $A.type) { continue }
    if ($c.Name -eq $target -or $c.AutomationId -eq $target -or ($A.partial -and $c.Name -like "*$target*")) { $m += $e }
  } catch {}
}
if (-not $m.Count) { 'ERROR: 窓「{0}」の中に「{1}」が見つかりません（pc_ui で確かめる）' -f $w.Name, $target; return }
$i = [int]$A.index; if ($i -ge $m.Count) { 'ERROR: 「{0}」は {1} 個しかありません' -f $target, $m.Count; return }
$e = $m[$i]; $c = $e.Current
$desc = '「{0}」（{1} 個中 {2} 番・{3}）' -f $c.Name, $m.Count, $i, ($c.ControlType.ProgrammaticName -replace '^ControlType\.', '')
function Pat($p) { try { return $e.GetCurrentPattern($p) } catch { return $null } }
$r = $c.BoundingRectangle
$cx = $r.X + $r.Width / 2; $cy = $r.Y + $r.Height / 2
switch ($act) {
  'click' {
    $ip = Pat ([System.Windows.Automation.InvokePattern]::Pattern)
    if ($ip) { $ip.Invoke(); "押した（Invoke）：$desc"; break }
    $sp = Pat ([System.Windows.Automation.SelectionItemPattern]::Pattern)
    if ($sp) { $sp.Select(); "選んだ：$desc"; break }
    $tp = Pat ([System.Windows.Automation.TogglePattern]::Pattern)
    if ($tp) { $tp.Toggle(); "切り替えた：$desc → " + $tp.Current.ToggleState; break }
    $ep = Pat ([System.Windows.Automation.ExpandCollapsePattern]::Pattern)
    if ($ep) { $ep.Expand(); "開いた：$desc"; break }
    if ($r.IsEmpty) { "ERROR: 押し方が分かりません（位置も無い）：$desc"; break }
    Click-At $cx $cy 'left'; "位置で押した ({0},{1})：{2}" -f [int]$cx, [int]$cy, $desc
  }
  'double' { Click-At $cx $cy 'double'; "ダブルクリックした ({0},{1})：{2}" -f [int]$cx, [int]$cy, $desc }
  'right' { Click-At $cx $cy 'right'; "右クリックした ({0},{1})：{2}" -f [int]$cx, [int]$cy, $desc }
  'set_value' {
    $vp = Pat ([System.Windows.Automation.ValuePattern]::Pattern)
    if (-not $vp) { "ERROR: 文字を入れられない部品です：$desc"; break }
    $vp.SetValue([string]$A.value); "入れた「{0}」：{1}" -f $A.value, $desc
  }
  'focus' { $e.SetFocus(); "選んだ（フォーカス）：$desc" }
  'toggle' { $tp = Pat ([System.Windows.Automation.TogglePattern]::Pattern); if ($tp) { $tp.Toggle(); "切り替えた：$desc → " + $tp.Current.ToggleState } else { "ERROR: 切り替えられない部品：$desc" } }
  'expand' { $ep = Pat ([System.Windows.Automation.ExpandCollapsePattern]::Pattern); if ($ep) { $ep.Expand(); "開いた：$desc" } else { "ERROR: 開けない部品：$desc" } }
  'collapse' { $ep = Pat ([System.Windows.Automation.ExpandCollapsePattern]::Pattern); if ($ep) { $ep.Collapse(); "閉じた：$desc" } else { "ERROR: 閉じられない部品：$desc" } }
  'select' { $sp = Pat ([System.Windows.Automation.SelectionItemPattern]::Pattern); if ($sp) { $sp.Select(); "選んだ：$desc" } else { "ERROR: 選べない部品：$desc" } }
  'scroll_into_view' { $si = Pat ([System.Windows.Automation.ScrollItemPattern]::Pattern); if ($si) { $si.ScrollIntoView(); "見える所まで動かした：$desc" } else { "ERROR: 動かせない部品：$desc" } }
  default { 'ERROR: action は click / double / right / set_value / focus / toggle / expand / collapse / select / scroll_into_view / click_xy / double_xy / right_xy' }
}
