# 台本を実行せずに読んで（AST）、呼ぶ命令・メソッド・書き出しを調べる
$tokens = $null; $errs = $null
$ast = [System.Management.Automation.Language.Parser]::ParseInput([string]$A.src, [ref]$tokens, [ref]$errs)
$cmds = New-Object System.Collections.Generic.List[string]
$dyn = 0
foreach ($c in $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.CommandAst] }, $true)) {
  $n = $c.GetCommandName()
  if (-not $n) { $dyn++; continue }
  $gc = Get-Command $n -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($gc -and $gc.CommandType -eq 'Alias') { $n = $gc.ResolvedCommandName }
  elseif ($gc -and $gc.CommandType -eq 'Application') { $n = $gc.Name }
  $cmds.Add($n)
}
$members = @($ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.InvokeMemberExpressionAst] }, $true) | ForEach-Object { $_.Member.ToString().Trim("'`"") })
$redir = @($ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.FileRedirectionAst] }, $true)).Count
[pscustomobject]@{ cmds = @($cmds); members = $members; redir = $redir; dynamic = $dyn; errors = @($errs | ForEach-Object { $_.Message }) } | ConvertTo-Json -Compress -Depth 3
