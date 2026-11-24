@echo off
if not exist "%LOCALAPPDATA%\iphone-mcp" mkdir "%LOCALAPPDATA%\iphone-mcp"
>"%LOCALAPPDATA%\iphone-mcp\mode.txt" echo(operate
echo iPhone の Claude 操作スイッチ：操作も可
timeout /t 2 >nul
