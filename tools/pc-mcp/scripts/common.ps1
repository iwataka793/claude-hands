# 画面・入力のための Windows の部品（1 回だけ読み込む）。座標は画面の本当の画素にそろえる（DPI）。
if (-not ('PcNative' -as [type])) {
  Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class PcNative {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, UIntPtr e);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int c);
  [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint f, UIntPtr e);
}
'@
}
[void][PcNative]::SetProcessDPIAware()
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes, System.Windows.Forms, System.Drawing

function Get-PcWindows {
  $root = [System.Windows.Automation.AutomationElement]::RootElement
  $cond = [System.Windows.Automation.Condition]::TrueCondition
  foreach ($w in $root.FindAll([System.Windows.Automation.TreeScope]::Children, $cond)) {
    try {
      $c = $w.Current
      if (-not $c.Name -and -not $c.ClassName) { continue }
      $pn = ''
      try { $pn = (Get-Process -Id $c.ProcessId -ErrorAction Stop).ProcessName } catch {}
      [pscustomobject]@{ El = $w; Name = $c.Name; Class = $c.ClassName; Pid = $c.ProcessId; Proc = $pn; Rect = $c.BoundingRectangle; Off = $c.IsOffscreen; Handle = $c.NativeWindowHandle }
    } catch {}
  }
}

# 窓を探す：題名の一部・プログラム名・pid のどれか
function Find-PcWindow($spec) {
  $ws = @(Get-PcWindows)
  if (-not $spec) { $h = [PcNative]::GetForegroundWindow(); return ($ws | Where-Object { $_.Handle -eq [int64]$h } | Select-Object -First 1) }
  if ($spec -match '^\d+$') { $m = @($ws | Where-Object { $_.Pid -eq [int]$spec }); if ($m.Count) { return $m[0] } }
  $m = @($ws | Where-Object { $_.Name -like "*$spec*" })
  if (-not $m.Count) { $m = @($ws | Where-Object { $_.Proc -like "*$spec*" }) }
  if ($m.Count) { return $m[0] }
  return $null
}

function Activate-PcWindow($w) {
  $h = [IntPtr]$w.Handle
  if ([PcNative]::IsIconic($h)) { [void][PcNative]::ShowWindow($h, 9) }
  # Alt を一度押すと、前に出すのを Windows が許しやすい
  [PcNative]::keybd_event(0x12, 0, 0, [UIntPtr]::Zero); [PcNative]::keybd_event(0x12, 0, 2, [UIntPtr]::Zero)
  [void][PcNative]::SetForegroundWindow($h)
  try { $w.El.SetFocus() } catch {}
  Start-Sleep -Milliseconds 150
}

function Fmt-Rect($r) { if ($r.IsEmpty) { return '' } ; '位置({0},{1}) 大きさ{2}x{3}' -f [int]$r.X, [int]$r.Y, [int]$r.Width, [int]$r.Height }
