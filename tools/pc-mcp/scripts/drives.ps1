Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' | ForEach-Object {
  $used = $_.Size - $_.FreeSpace
  '  {0}  使用 {1} GB / {2} GB（空き {3} GB・{4}%）' -f $_.DeviceID, [math]::Round($used / 1GB), [math]::Round($_.Size / 1GB), [math]::Round($_.FreeSpace / 1GB), [math]::Round($_.FreeSpace * 100 / $_.Size)
}
