@echo off
rem パスワード・セキュリティ・お金にかかわる操作を 10 分だけ許可する。
rem 管理者の確認（UAC）を通ってから、管理者しか書けない場所（%ProgramData%\pc-agent）に書く。
net session >nul 2>&1
if errorlevel 1 (
  powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)
set A=%ProgramData%\pc-agent
if not exist "%A%" mkdir "%A%"
icacls "%A%" /inheritance:r /grant:r "*S-1-5-32-544:(OI)(CI)F" "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-545:(OI)(CI)RX" >nul
>"%A%\allow.txt" echo(allow
echo パスワード・セキュリティ・お金にかかわる操作を、今から 10 分だけ許可しました
echo （スイッチを切り替えると、許可は取り消されます）
timeout /t 4 >nul
