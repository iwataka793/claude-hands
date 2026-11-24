@echo off
set D=%USERPROFILE%\devkit\pc-agent
if not exist "%D%\pc-agent.exe" (
  echo pc-agent.exe がありません。先に pc-setup.bat を実行してください
  pause
  exit /b 1
)
>"%D%\mode.txt" echo(view
start "" "%D%\pc-agent.exe" agent
echo PC の Claude 操作スイッチ：見るだけ
echo （使わない時間が 30 分続くと自動でオフ）
timeout /t 2 >nul
