$vs = [System.Windows.Forms.SystemInformation]::VirtualScreen
$x = $vs.X; $y = $vs.Y; $wd = $vs.Width; $ht = $vs.Height
$title = ''
if ($A.window) {
  $w = Find-PcWindow $A.window
  if (-not $w) { 'ERROR: 窓「{0}」が見つかりません' -f $A.window; return }
  if ($A.activate) { [void](Activate-PcWindow $w); Start-Sleep -Milliseconds 200 }
  $r = $w.Rect; $x = [int]$r.X; $y = [int]$r.Y; $wd = [int]$r.Width; $ht = [int]$r.Height; $title = $w.Name
}
$bmp = New-Object System.Drawing.Bitmap $wd, $ht
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($x, $y, 0, 0, $bmp.Size)
$ms = New-Object System.IO.MemoryStream
$bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()
[pscustomobject]@{ left = $x; top = $y; width = $wd; height = $ht; title = $title; png = [Convert]::ToBase64String($ms.ToArray()) } | ConvertTo-Json -Compress
