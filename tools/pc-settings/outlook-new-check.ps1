# 新しい Outlook（olk）の設定の点検と戻し
#   そのまま実行      … 点検だけ（何も変えない）
#   -Fix を付けて実行 … Windows 側の設定（自動起動・Windows の通知の許可）を戻す
# Outlook の中の設定（音・優先受信トレイ・お気に入りなど）はプライマリ アカウントに付いて保存される。
# Windows からは変えられないので、最後に「手で確かめる一覧」を出す。
param([switch]$Fix)

$ErrorActionPreference = 'Stop'
$pfn     = 'Microsoft.OutlookForWindows_8wekyb3d8bbwe'
$appId   = "$pfn!Microsoft.OutlookforWindows"
$startup = [Environment]::GetFolderPath('Startup')
$newLnk  = Join-Path $startup '新しい Outlook（最小化で起動）.lnk'
$oldLnk  = Join-Path $startup 'Outlook（最小化で起動）.lnk'
$backup  = Join-Path $env:USERPROFILE 'devkit\setup\backup'
$alias   = Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\olk.exe'
$notifKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Notifications\Settings\$appId"
$ng = 0

function Say($ok, $text) {
    if ($ok) { Write-Host "  OK  $text" -ForegroundColor Green }
    else     { Write-Host "  NG  $text" -ForegroundColor Yellow; $script:ng++ }
}

Write-Host '■ 新しい Outlook の点検'

# 1. 入っているか
$pkg = Get-AppxPackage | Where-Object Name -eq 'Microsoft.OutlookForWindows'
Say ([bool]$pkg) ("新しい Outlook が入っている" + $(if ($pkg) { "（$($pkg.Version)）" } else { '' }))

# 2. 自動起動（新しい Outlook を最小化で）
Say (Test-Path $newLnk) 'Windows 起動時に新しい Outlook を最小化で開く'
if ($Fix -and -not (Test-Path $newLnk) -and (Test-Path $alias)) {
    $w = New-Object -ComObject WScript.Shell
    $s = $w.CreateShortcut($newLnk)
    $s.TargetPath  = $alias
    $s.WindowStyle = 7
    $s.Description = '新しい Outlook を最小化で起動（通知を受けるため）'
    $s.Save()
    Write-Host '      → 作った'
}

# 3. 昔の Outlook の自動起動が残っていないか（両方動くと通知が二重になる）
Say (-not (Test-Path $oldLnk)) '昔の Outlook（classic）は自動起動しない'
if ($Fix -and (Test-Path $oldLnk)) {
    New-Item -ItemType Directory -Force -Path $backup | Out-Null
    Move-Item -LiteralPath $oldLnk -Destination $backup -Force
    Write-Host "      → $backup へ移した（消してはいない）"
}

# 4. Windows の通知の許可（値が無い＝オン、Enabled=0＝オフ）
$en = $null
if (Test-Path $notifKey) { $en = (Get-ItemProperty $notifKey -ErrorAction SilentlyContinue).Enabled }
Say ($en -ne 0) 'Windows の設定で Outlook の通知がオン'
if ($Fix -and $en -eq 0) {
    Set-ItemProperty -Path $notifKey -Name Enabled -Value 1 -Type DWord
    Write-Host '      → オンに戻した'
}

# 5. Windows 全体の通知と「応答不可」
$push = (Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\PushNotifications' -ErrorAction SilentlyContinue).ToastEnabled
Say ($push -ne 0) 'Windows 全体の通知がオン'

# 6. いま動いているか
Say ([bool](Get-Process olk -ErrorAction SilentlyContinue)) '新しい Outlook がいま動いている（閉じると通知は来ない）'

Write-Host ''
Write-Host '■ Outlook の中で手で確かめる（… → 設定）'
Write-Host '  ・アカウント：3 つ（Gmail・Yahoo・iCloud）がある。Gmail が一番上＝プライマリ'
Write-Host '      ※ プライマリを変える・消すと、下の設定が初期に戻る'
Write-Host '  ・全般 → 通知：Outlook の通知 オン／メール オン／スタイル デスクトップ／サウンド デフォルト'
Write-Host '  ・メール → メッセージ一覧 → 優先受信トレイ：3 つのアドレスとも「分類する」のチェックなし'
Write-Host '  ・メール → フォルダー ウィンドウ：お気に入りを表示・上部にピン留め'
Write-Host '  ・左の「お気に入り」に 3 つの受信トレイ（フォルダーの ☆ で足せる）'
Write-Host '  ・Yahoo：IMAP imap.mail.yahoo.co.jp 993 SSL ／ SMTP smtp.mail.yahoo.co.jp 465 SSL'
Write-Host ''
if ($ng -eq 0) { Write-Host '結果：問題なし' -ForegroundColor Green }
elseif ($Fix)  { Write-Host "結果：$ng 件 直した（もう一度そのまま実行して確かめる）" }
else           { Write-Host "結果：$ng 件 要確認。直すときは -Fix を付けて実行" -ForegroundColor Yellow }
