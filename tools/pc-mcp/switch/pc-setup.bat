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
for %%f in (PC-OFF.bat PC-VIEW.bat PC-OPERATE.bat PC-ALLOW.bat) do copy /y "%~dp0%%f" "%USERPROFILE%\Desktop\%%f" >nul
echo 置きました：
echo   常駐役 %D%\pc-agent.exe
echo   スイッチ デスクトップの PC-OFF / PC-VIEW / PC-OPERATE / PC-ALLOW
pause
