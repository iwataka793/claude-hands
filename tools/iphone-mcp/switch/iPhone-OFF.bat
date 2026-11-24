@echo off
set D=%USERPROFILE%\devkit\iphone-mcp
if not exist "%D%" mkdir "%D%"
>"%D%\mode.txt" echo(off
echo iPhone の Claude 操作スイッチ：オフ
timeout /t 2 >nul
