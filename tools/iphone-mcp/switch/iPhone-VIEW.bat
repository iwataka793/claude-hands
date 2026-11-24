@echo off
set D=%USERPROFILE%\devkit\iphone-mcp
if not exist "%D%" mkdir "%D%"
>"%D%\mode.txt" echo(view
echo iPhone の Claude 操作スイッチ：見るだけ
timeout /t 2 >nul
