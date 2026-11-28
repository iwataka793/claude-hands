@echo off
rem PC の道具の常駐役とスイッチを置く（最初と、新しい版にするとき）
set D=%USERPROFILE%\devkit\pc-agent
if not exist "%D%" mkdir "%D%"
if exist "%D%\pc-agent.exe" (
  >"%D%\mode.txt" echo(off 2>nul
  rem ブラウザのスイッチが入っていると常駐役は止まらないので、ブラウザもオフにする
  if exist "%USERPROFILE%\devkit\browser" >"%USERPROFILE%\devkit\browser\mode.txt" echo(off 2>nul
  echo 動いている常駐役を止めています...
  for /l %%i in (1,1,20) do (
    tasklist /fi "imagename eq pc-agent.exe" 2>nul | find /i "pc-agent.exe" >nul || goto stopped
    timeout /t 1 >nul
  )
)
:stopped
copy /y "%~dp0pc-agent.exe" "%D%\pc-agent.exe" >nul || (echo pc-agent.exe を置けませんでした & pause & exit /b 1)
set S=%USERPROFILE%\devkit\switch-app
if not exist "%S%" mkdir "%S%"
copy /y "%~dp0devkit-switch.ps1" "%S%\devkit-switch.ps1" >nul
copy /y "%~dp0devkit-switch.vbs" "%S%\devkit-switch.vbs" >nul
copy /y "%~dp0devkit-switch.ico" "%S%\devkit-switch.ico" >nul
rem 窓（スイッチ・LIVE）の置き場所
set H=%USERPROFILE%\devkit\hands
if not exist "%H%" mkdir "%H%"
rem スイッチと「許可しますか？」の窓（新しい窓。今までの PowerShell の窓は予備：devkit-switch.vbs /old）
powershell -NoProfile -Command "Get-CimInstance Win32_Process -Filter \"Name='powershell.exe'\" | Where-Object { $_.CommandLine -like '*devkit-switch.ps1*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }"
taskkill /im claude-hands-switch.exe /f >nul 2>&1
timeout /t 1 >nul
copy /y "%~dp0claude-hands-switch.exe" "%H%\claude-hands-switch.exe" >nul || echo スイッチの窓を置けませんでした
rem デスクトップには窓のショートカットだけを置く（.bat は zip の中に予備として残す）
powershell -NoProfile -Command "$s=(New-Object -ComObject WScript.Shell).CreateShortcut([Environment]::GetFolderPath('Desktop')+'\Claude 操作スイッチ.lnk'); $s.TargetPath=$env:USERPROFILE+'\devkit\hands\claude-hands-switch.exe'; $s.Arguments=''; $s.WorkingDirectory=$env:USERPROFILE+'\devkit\hands'; $s.IconLocation=$env:USERPROFILE+'\devkit\switch-app\devkit-switch.ico,0'; $s.Description='iPhone・PC・ブラウザの Claude 操作スイッチ'; $s.Save()"
rem LIVE の窓（右下の「Claude が動いています」。Windows の起動のたびに動かす）
taskkill /im claude-hands-live.exe /f >nul 2>&1
timeout /t 1 >nul
copy /y "%~dp0claude-hands-live.exe" "%H%\claude-hands-live.exe" >nul || echo LIVE の窓を置けませんでした
powershell -NoProfile -Command "$s=(New-Object -ComObject WScript.Shell).CreateShortcut([Environment]::GetFolderPath('Startup')+'\claude-hands LIVE.lnk'); $s.TargetPath=$env:USERPROFILE+'\devkit\hands\claude-hands-live.exe'; $s.Save()"
start "" /D "%H%" "%H%\claude-hands-live.exe"
rem 守りの場所（取り置き）と毎日の点検を作る（管理者が要るので UAC が出る）
echo.
echo 守りの場所（取り置き）と毎日の点検を作ります。UAC が出たら「はい」を押してください...
powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','\"%~dp0hands-protect.ps1\"','-User','%USERDOMAIN%\%USERNAME%'"
powershell -NoProfile -Command "Get-Content -Encoding UTF8 -LiteralPath (Join-Path $env:TEMP 'claude-hands-protect.txt') -ErrorAction SilentlyContinue | Where-Object { $_ -match '^(OK|NG)' }"
rem 箱（Sandboxie）の設定を、管理者なし（Claude の道具と同じ権限）では変えられないことを確かめる
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0box-check.ps1"
echo 置きました：
echo   常駐役 %D%\pc-agent.exe
echo   スイッチの窓 %H%\claude-hands-switch.exe（デスクトップの「Claude 操作スイッチ」。Claude が頼むと真ん中に出ます。右下の通知領域にも丸）
echo   予備の窓 %S%\devkit-switch.vbs /old（今までの PowerShell の窓）
echo   守りの場所 %ProgramData%\claude-hands（取り置き・毎日 12:00 の点検）
echo   LIVE の窓 %USERPROFILE%\devkit\hands\claude-hands-live.exe（Claude が動いている間だけ右下に出る。スタートアップにも置いた）
pause
