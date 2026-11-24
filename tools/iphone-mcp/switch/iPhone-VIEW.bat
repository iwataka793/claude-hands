@echo off
if not exist "%LOCALAPPDATA%\iphone-mcp" mkdir "%LOCALAPPDATA%\iphone-mcp"
>"%LOCALAPPDATA%\iphone-mcp\mode.txt" echo(view
echo iPhone の Claude 操作スイッチ：見るだけ
timeout /t 2 >nul
