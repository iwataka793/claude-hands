@echo off
set D=%USERPROFILE%\devkit\iphone-mcp
if not exist "%D%" mkdir "%D%"
>"%D%\mode.txt" echo(operate
echo iPhone の Claude 操作スイッチ：操作も可
timeout /t 2 >nul
