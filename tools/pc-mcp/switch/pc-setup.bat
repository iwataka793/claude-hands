@echo off
rem PC の道具の常駐役とスイッチを置く（最初と、新しい版にするとき）
set D=%USERPROFILE%\devkit\pc-agent
if not exist "%D%" mkdir "%D%"
if exist "%D%\pc-agent.exe" (
  >"%D%\mode.txt" echo(off
  echo 動いている常駐役を止めています...
  timeout /t 4 >nul
)
copy /y "%~dp0pc-agent.exe" "%D%\pc-agent.exe" >nul || (echo pc-agent.exe を置けませんでした & pause & exit /b 1)
set S=%USERPROFILE%\devkit\switch-app
if not exist "%S%" mkdir "%S%"
copy /y "%~dp0devkit-switch.ps1" "%S%\devkit-switch.ps1" >nul
copy /y "%~dp0devkit-switch.vbs" "%S%\devkit-switch.vbs" >nul
copy /y "%~dp0devkit-switch.ico" "%S%\devkit-switch.ico" >nul
rem スイッチは窓で切り替える。デスクトップには窓のショートカットだけを置く（.bat は zip の中に予備として残す）
wscript.exe "%S%\devkit-switch.vbs" /shortcut
echo 置きました：
echo   常駐役 %D%\pc-agent.exe
echo   スイッチの窓 %S%\devkit-switch.vbs（デスクトップの「Claude 操作スイッチ」。Claude が頼むと自動で出ます）
pause
