@echo off
set D=%USERPROFILE%\devkit\pc-agent
if not exist "%D%" mkdir "%D%"
>"%D%\mode.txt" echo(off
echo PC の Claude 操作スイッチ：オフ（常駐役は数秒で終わります）
timeout /t 2 >nul
