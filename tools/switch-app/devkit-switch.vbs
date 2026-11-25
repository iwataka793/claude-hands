' devkit-switch.ps1 を、黒い窓を出さずに起動する（ダブルクリック用）。
' スタートアップに置くときは、引数に /tray を付けたショートカットにする（窓を出さずに常駐だけ）。
Set sh = CreateObject("WScript.Shell")
dir = CreateObject("Scripting.FileSystemObject").GetParentFolderName(WScript.ScriptFullName)
arg = ""
If WScript.Arguments.Count > 0 Then arg = " " & WScript.Arguments(0)
sh.Run "powershell.exe -NoProfile -ExecutionPolicy Bypass -STA -WindowStyle Hidden -File """ & dir & "\devkit-switch.ps1""" & arg, 0, False
