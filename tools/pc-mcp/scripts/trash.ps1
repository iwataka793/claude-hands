# 消す：取り置き（$A.holding。30 日。Claude には消せない・取り出せない）に写して中身が合うのを確かめてから、元を消す。
# ごみ箱は通さない（取り置きとごみ箱で C を 2 回使わないため。2026-11-28。ごみ箱はカオルが自分で消したものだけ）。
# 元を消せなかったときだけごみ箱へ送る。
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
# ファイルの中身だけを写す（[IO.File]::Copy は「ネットから来た」印などの付属データも写そうとして、取り置きの権限で拒否される）
function CopyOne($s, $d) {
  $in = [IO.File]::OpenRead($s)
  try {
    $out = [IO.File]::Open($d, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
    try { $in.CopyTo($out) } finally { $out.Dispose() }
  } finally { $in.Dispose() }
  # 時刻は付けられない（取り置きの権限）。元の更新日時は「元の場所.txt」に書く
}
function CopyTree($src, $dst) {
  [void][IO.Directory]::CreateDirectory($dst)
  foreach ($e in @(Get-ChildItem -LiteralPath $src -Force)) {
    if ($e.PSIsContainer) {
      if ($e.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
      CopyTree $e.FullName (Join-Path $dst $e.Name)
    } else { CopyOne $e.FullName (Join-Path $dst $e.Name) }
  }
}
# 取り置きに写せた元を消す。つなぎ目（ジャンクション）の先は消さない（.NET の Directory.Delete は中に入らない）
function DropHeld($p) {
  if (Test-Path -LiteralPath $p -PathType Container) {
    foreach ($f in @(Get-ChildItem -LiteralPath $p -Recurse -Force -File -ErrorAction SilentlyContinue)) {
      if ($f.Attributes -band [IO.FileAttributes]::ReadOnly) { $f.Attributes = $f.Attributes -band (-bnot [IO.FileAttributes]::ReadOnly) }
    }
    [IO.Directory]::Delete($p, $true)
  } else {
    $i = Get-Item -LiteralPath $p -Force
    if ($i.Attributes -band [IO.FileAttributes]::ReadOnly) { $i.Attributes = $i.Attributes -band (-bnot [IO.FileAttributes]::ReadOnly) }
    [IO.File]::Delete($p)
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
        "戻すとき：この中のものを、元の場所へコピーしてください（取り置きの中からは消せません。30 日たつと毎日の点検が消します）",
        "元の更新日時：$((Get-Item -LiteralPath $p -Force).LastWriteTime.ToString('s'))",
        $(try {
          $z = Get-Content -LiteralPath $p -Stream Zone.Identifier -ErrorAction Stop
          $zone = ($z | Where-Object { $_ -match '^ZoneId=' }) -join ''
          $host1 = ($z | Where-Object { $_ -match '^HostUrl=' } | ForEach-Object { try { ([uri]($_ -replace '^HostUrl=', '')).Host } catch { '' } }) -join ''
          'ネットから来た印（写しには付いていない）：' + $zone + $(if ($host1) { ' ' + $host1 } else { '' })
        } catch { '' }))
      $dst = Join-Path $item (Split-Path -Leaf $p)
      if (Test-Path -LiteralPath $p -PathType Container) {
        CopyTree $p $dst
      } else {
        CopyOne $p $dst
      }
      $got = SizeOf $dst
      if ($got -ne $size) { throw "取り置きの写しの大きさが合いません（元 $size／写し $got）。元は消していません" }
      $held = $item
    }
    $via = ''
    if ($held) {
      try { DropHeld $p } catch { $via = '（元を消せず、ごみ箱へ送った：' + $_.Exception.Message + '）'; if (Test-Path -LiteralPath $p) { ToRecycle $p } }
    } else { ToRecycle $p }
    if (Test-Path -LiteralPath $p) { 'ERROR: 残っている：' + $p; continue }
    $used += $size
    if ($held) { '消した（取り置き 30 日・ごみ箱は通さない）：' + $p + ' → ' + $held + $via } else { 'ごみ箱へ（取り置きなし）：' + $p }
  } catch { 'ERROR: ' + $p + '：' + $_.Exception.Message }
}
