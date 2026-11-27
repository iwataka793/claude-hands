# 消す：取り置き（$A.holding。30 日。Claude には消せない・取り出せない）に写してから、元をごみ箱へ。
# $A.recycle が true のときだけ、取り置きに写さずにごみ箱へ（取り置きに入らない大きいもの。カオルに聞いてから）。
Add-Type -AssemblyName Microsoft.VisualBasic
# つなぎ目（ジャンクション・シンボリックリンク）の先は数えない・写さない
function SizeOf($p) {
  $i = Get-Item -LiteralPath $p -Force
  if (-not $i.PSIsContainer) { return [int64]$i.Length }
  $s = [int64]0
  foreach ($e in @(Get-ChildItem -LiteralPath $p -Force -ErrorAction SilentlyContinue)) {
    if ($e.PSIsContainer) { if (-not ($e.Attributes -band [IO.FileAttributes]::ReparsePoint)) { $s += SizeOf $e.FullName } }
    else { $s += $e.Length }
  }
  return $s
}
# 取り置きの中は「作る」だけの権限なので、robocopy（作ったあとにフォルダの属性を書こうとして拒否される）は使わない。
# フォルダを作ってファイルを 1 つずつ写す（2026-11-27 にカオルの PC で確かめた）
function CopyTree($src, $dst) {
  [void][IO.Directory]::CreateDirectory($dst)
  foreach ($e in @(Get-ChildItem -LiteralPath $src -Force)) {
    if ($e.PSIsContainer) {
      if ($e.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
      CopyTree $e.FullName (Join-Path $dst $e.Name)
    } else { [IO.File]::Copy($e.FullName, (Join-Path $dst $e.Name)) }
  }
}
function ToRecycle($p) {
  if (Test-Path -LiteralPath $p -PathType Container) {
    [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
  } else {
    [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
  }
}
$used = [int64]$A.used
$cap = [int64]$A.cap
foreach ($p in $A.paths) {
  try {
    if (-not (Test-Path -LiteralPath $p)) { 'ERROR: 無い：' + $p; continue }
    $size = SizeOf $p
    $held = $null
    if (-not $A.recycle) {
      if (-not $A.ready) { 'ERROR: 取り置きの場所がまだありません。カオルに pc-setup.bat を動かしてもらってください（UAC あり）。急ぐなら、カオルに聞いてから recycle: true でごみ箱だけに送る：' + $p; continue }
      if ($used + $size -gt $cap) {
        $why = if ($A.capWhy) { '。' + $A.capWhy } else { '' }
        'ERROR: 取り置きに入りません（{0:N1} GB。使っている {1:N1} GB／上限 {2:N1} GB{3}）。元は消していません。カオルに聞いてから recycle: true でごみ箱だけに送る：{4}' -f ($size / 1GB), ($used / 1GB), ($cap / 1GB), $why, $p
        continue
      }
      $item = Join-Path $A.holding (Get-Date -Format 'yyyyMMdd-HHmmss-fff')
      New-Item -ItemType Directory -Path $item -ErrorAction Stop | Out-Null
      Set-Content -LiteralPath (Join-Path $item '元の場所.txt') -Encoding UTF8 -ErrorAction Stop -Value @(
        "元の場所：$p", "取り置いた時刻：$(Get-Date -Format s)", "大きさ：$size バイト",
        "戻すとき：この中のものを、元の場所へコピーしてください（取り置きの中からは消せません。30 日たつと毎日の点検が消します）")
      $dst = Join-Path $item (Split-Path -Leaf $p)
      if (Test-Path -LiteralPath $p -PathType Container) {
        CopyTree $p $dst
      } else {
        [IO.File]::Copy($p, $dst)
      }
      $got = SizeOf $dst
      if ($got -ne $size) { throw "取り置きの写しの大きさが合いません（元 $size／写し $got）。元は消していません" }
      $held = $item
    }
    ToRecycle $p
    if (Test-Path -LiteralPath $p) { 'ERROR: 残っている：' + $p; continue }
    $used += $size
    if ($held) { 'ごみ箱へ（取り置き 30 日）：' + $p + ' → ' + $held } else { 'ごみ箱へ（取り置きなし）：' + $p }
  } catch { 'ERROR: ' + $p + '：' + $_.Exception.Message }
}
