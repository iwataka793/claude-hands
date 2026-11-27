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
rem スイッチは窓で切り替える。デスクトップには窓のショートカットだけを置く（.bat は zip の中に予備として残す）
wscript.exe "%S%\devkit-switch.vbs" /shortcut
rem 守りの場所（取り置き）と毎日の点検を作る（管理者が要るので UAC が出る）
echo.
echo 守りの場所（取り置き）と毎日の点検を作ります。UAC が出たら「はい」を押してください...
powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process powershell -Verb RunAs -Wait -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File','\"%~dp0hands-protect.ps1\"','-User','%USERDOMAIN%\%USERNAME%'"
powershell -NoProfile -Command "Get-Content -Encoding UTF8 -LiteralPath (Join-Path $env:TEMP 'claude-hands-protect.txt') -ErrorAction SilentlyContinue | Where-Object { $_ -match '^(OK|NG)' }"
echo 置きました：
echo   常駐役 %D%\pc-agent.exe
echo   スイッチの窓 %S%\devkit-switch.vbs（デスクトップの「Claude 操作スイッチ」。Claude が頼むと自動で出ます）
echo   守りの場所 %ProgramData%\claude-hands（取り置き・毎日 12:00 の点検）
pause
