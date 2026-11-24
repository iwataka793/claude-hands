@echo off
if not exist "%LOCALAPPDATA%\iphone-mcp" mkdir "%LOCALAPPDATA%\iphone-mcp"
>"%LOCALAPPDATA%\iphone-mcp\mode.txt" echo(off
echo iPhone の Claude 操作スイッチ：オフ
timeout /t 2 >nul
