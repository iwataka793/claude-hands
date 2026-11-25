# devkit-switch：iPhone と PC の Claude 操作スイッチを 1 つの窓にまとめたもの（カオル用）。
# - 今の状態（オフ・見るだけ・操作も可・許可）と、自動オフまでの残りが見える
# - ボタンで切り替える（中身は今までの .bat と同じ：mode.txt を書く。PC の許可は UAC を通って書く）
# - Claude が「操作も可にしたい」「許可がほしい」と頼んだとき（devkit\requests に頼みのファイルが置かれたとき）、
#   「許可しますか？」の窓を出す。決めるのはカオル。Claude の道具は mode.txt・allow.txt を書かない
# 通知領域（右下）に常駐する。窓を閉じても常駐は続く（終わるのは通知領域のメニューの「終わる」）。
# Windows PowerShell 5.1（WPF）で動かす。devkit-switch.vbs から、黒い窓を出さずに起動する。

Add-Type -AssemblyName PresentationFramework, PresentationCore, WindowsBase, System.Windows.Forms, System.Drawing

# /shortcut：デスクトップに「Claude 操作スイッチ」のショートカットを作って終わる（pc-setup.bat が使う）
if ($args -contains '/shortcut') {
    $here = Split-Path -Parent $MyInvocation.MyCommand.Path
    $lnk = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Claude 操作スイッチ.lnk'
    $sc = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
    $sc.TargetPath = Join-Path $env:SystemRoot 'System32\wscript.exe'
    $sc.Arguments = '"' + (Join-Path $here 'devkit-switch.vbs') + '"'
    $sc.WorkingDirectory = $here
    $sc.IconLocation = (Join-Path $here 'devkit-switch.ico') + ',0'
    $sc.Description = 'iPhone と PC の Claude 操作スイッチ'
    $sc.Save()
    exit
}

$created = $false
$mutex = New-Object System.Threading.Mutex($true, 'Local\DevKitSwitchApp', [ref]$created)
$showSignal = New-Object System.Threading.EventWaitHandle($false, 'AutoReset', 'Local\DevKitSwitchShow')
if (-not $created) {
    # もう動いている：ダブルクリックで開いたときは、動いている方に「窓を出して」と知らせて終わる
    if ($args -notcontains '/tray') { [void]$showSignal.Set() }
    exit
}
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$IconFile = Join-Path $Here 'devkit-switch.ico'

$U        = $env:USERPROFILE
$Phone    = Join-Path $U 'devkit\iphone-mcp'
$Pc       = Join-Path $U 'devkit\pc-agent'
$AllowDir = Join-Path $env:ProgramData 'pc-agent'
$Req      = Join-Path $U 'devkit\requests'
$Idle     = [TimeSpan]::FromMinutes(30)
$AllowWin = [TimeSpan]::FromMinutes(10)
New-Item -ItemType Directory -Force $Req | Out-Null

$names  = @{ off = 'オフ'; view = '見るだけ'; operate = '操作も可' }
$colors = @{ off = '#6B7280'; view = '#3B82F6'; operate = '#22C55E' }

# ---------------- 状態を読む・書く ----------------
function Get-Mode($dir) {
    $f = Join-Path $dir 'mode.txt'
    if (-not (Test-Path $f)) { return 'off' }
    $v = ((Get-Content $f -Raw -ErrorAction SilentlyContinue) + '').Trim().ToLower()
    if ($names.ContainsKey($v)) { return $v } else { return 'off' }
}
function Get-Left($dir) {
    # 最後の動き（スイッチを切り替えた時刻か、道具を使った時刻の新しい方）から 30 分
    $last = [datetime]::MinValue
    foreach ($n in 'mode.txt', 'last-used.txt') {
        $f = Join-Path $dir $n
        if (Test-Path $f) { $t = (Get-Item $f).LastWriteTime; if ($t -gt $last) { $last = $t } }
    }
    if ($last -eq [datetime]::MinValue) { return [TimeSpan]::Zero }
    return $Idle - ((Get-Date) - $last)
}
function Get-AllowLeft {
    $a = Join-Path $AllowDir 'allow.txt'
    $m = Join-Path $Pc 'mode.txt'
    if (-not (Test-Path $a)) { return $null }
    $at = (Get-Item $a).LastWriteTime
    if ((Test-Path $m) -and (Get-Item $m).LastWriteTime -gt $at) { return $null }  # 切り替えたら取り消し
    $left = $AllowWin - ((Get-Date) - $at)
    if ($left -le [TimeSpan]::Zero) { return $null }
    return $left
}
function Set-Mode($dir, $mode) {
    New-Item -ItemType Directory -Force $dir | Out-Null
    [IO.File]::WriteAllText((Join-Path $dir 'mode.txt'), "$mode`r`n")
}
function Set-PhoneMode($mode) { Set-Mode $Phone $mode }
function Set-PcMode($mode) {
    $exe = Join-Path $Pc 'pc-agent.exe'
    if ($mode -ne 'off' -and -not (Test-Path $exe)) {
        [System.Windows.MessageBox]::Show('pc-agent.exe がありません。先に pc-setup.bat を実行してください', 'devkit') | Out-Null
        return
    }
    Set-Mode $Pc $mode
    if ($mode -ne 'off' -and -not (Get-Process -Name 'pc-agent' -ErrorAction SilentlyContinue)) {
        Start-Process -FilePath $exe -ArgumentList 'agent' -WindowStyle Hidden
    }
}
function Grant-PcAllow {
    # 今までの PC-ALLOW.bat と同じことを、UAC（管理者の確認）を通ってから行う
    $cmd = @"
`$A = Join-Path `$env:ProgramData 'pc-agent'
New-Item -ItemType Directory -Force `$A | Out-Null
icacls `$A /inheritance:r /grant:r '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-545:(OI)(CI)RX' | Out-Null
[IO.File]::WriteAllText((Join-Path `$A 'allow.txt'), "allow``r``n")
"@
    $enc = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($cmd))
    try { Start-Process powershell.exe -Verb RunAs -WindowStyle Hidden -ArgumentList "-NoProfile -EncodedCommand $enc" -Wait } catch { }  # UAC で「いいえ」
}

# ---------------- 見た目（WPF） ----------------
[xml]$xaml = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        Title="Claude 操作スイッチ" Width="470" SizeToContent="Height"
        WindowStyle="None" AllowsTransparency="True" Background="Transparent"
        ResizeMode="NoResize" WindowStartupLocation="CenterScreen"
        FontFamily="Yu Gothic UI" Foreground="#E5E7EB">
  <Window.Resources>
    <Style x:Key="Seg" TargetType="RadioButton">
      <Setter Property="Foreground" Value="#9CA3AF"/>
      <Setter Property="FontSize" Value="14"/>
      <Setter Property="Cursor" Value="Hand"/>
      <Setter Property="Template">
        <Setter.Value>
          <ControlTemplate TargetType="RadioButton">
            <Border x:Name="b" CornerRadius="9" Background="Transparent" Padding="0,9" Margin="3">
              <TextBlock Text="{TemplateBinding Content}" HorizontalAlignment="Center" FontWeight="SemiBold"
                         Foreground="{TemplateBinding Foreground}"/>
            </Border>
            <ControlTemplate.Triggers>
              <Trigger Property="IsMouseOver" Value="True">
                <Setter TargetName="b" Property="Background" Value="#273244"/>
              </Trigger>
              <Trigger Property="IsChecked" Value="True">
                <Setter TargetName="b" Property="Background" Value="{Binding Tag, RelativeSource={RelativeSource TemplatedParent}}"/>
                <Setter Property="Foreground" Value="White"/>
              </Trigger>
            </ControlTemplate.Triggers>
          </ControlTemplate>
        </Setter.Value>
      </Setter>
    </Style>
    <Style x:Key="Btn" TargetType="Button">
      <Setter Property="Foreground" Value="White"/>
      <Setter Property="Padding" Value="16,9"/>
      <Setter Property="FontSize" Value="14"/>
      <Setter Property="Cursor" Value="Hand"/>
      <Setter Property="Template">
        <Setter.Value>
          <ControlTemplate TargetType="Button">
            <Border x:Name="b" CornerRadius="10" Background="{TemplateBinding Background}" Padding="{TemplateBinding Padding}">
              <ContentPresenter HorizontalAlignment="Center" VerticalAlignment="Center"/>
            </Border>
            <ControlTemplate.Triggers>
              <Trigger Property="IsMouseOver" Value="True"><Setter TargetName="b" Property="Opacity" Value="0.85"/></Trigger>
              <Trigger Property="IsPressed" Value="True"><Setter TargetName="b" Property="Opacity" Value="0.7"/></Trigger>
            </ControlTemplate.Triggers>
          </ControlTemplate>
        </Setter.Value>
      </Setter>
    </Style>
    <Style x:Key="Bar" TargetType="ProgressBar">
      <Setter Property="Height" Value="4"/>
      <Setter Property="Template">
        <Setter.Value>
          <ControlTemplate TargetType="ProgressBar">
            <Grid>
              <Border CornerRadius="2" Background="#1F2937"/>
              <Border x:Name="PART_Track"/>
              <Border x:Name="PART_Indicator" CornerRadius="2" HorizontalAlignment="Left" Background="{TemplateBinding Foreground}"/>
            </Grid>
          </ControlTemplate>
        </Setter.Value>
      </Setter>
    </Style>
  </Window.Resources>

  <Border CornerRadius="18" Background="#0F172A" BorderBrush="#1E293B" BorderThickness="1" Margin="14">
    <Border.Effect><DropShadowEffect BlurRadius="24" ShadowDepth="0" Opacity="0.55"/></Border.Effect>
    <StackPanel Margin="22,16,22,20">

      <Grid x:Name="Header" Background="Transparent" Margin="0,0,0,14">
        <StackPanel>
          <TextBlock Text="Claude 操作スイッチ" FontSize="19" FontWeight="Bold" Foreground="White"/>
          <TextBlock Text="iPhone と PC を、Claude にどこまで任せるか" FontSize="12" Foreground="#64748B" Margin="0,2,0,0"/>
        </StackPanel>
        <Button x:Name="BtnClose" HorizontalAlignment="Right" VerticalAlignment="Top" Width="34" Height="30"
                Style="{StaticResource Btn}" Background="#1E293B" Padding="0" ToolTip="隠す（右下に常駐したまま）">
          <TextBlock Text="&#xE8BB;" FontFamily="Segoe Fluent Icons, Segoe MDL2 Assets" FontSize="11" Foreground="#CBD5E1"/>
        </Button>
      </Grid>

      <!-- iPhone -->
      <Border CornerRadius="14" Background="#111C2E" Padding="16,14" Margin="0,0,0,12">
        <StackPanel>
          <Grid>
            <StackPanel Orientation="Horizontal" VerticalAlignment="Center">
              <TextBlock Text="📱" FontSize="15" FontFamily="Segoe UI Emoji" VerticalAlignment="Center" Margin="0,0,8,0"/>
              <TextBlock Text="iPhone" FontSize="16" FontWeight="SemiBold" Foreground="White" VerticalAlignment="Center"/>
            </StackPanel>
            <Border x:Name="PhonePill" HorizontalAlignment="Right" CornerRadius="10" Padding="10,2" Background="#6B7280">
              <TextBlock x:Name="PhoneState" Text="オフ" FontSize="12" Foreground="White" FontWeight="SemiBold"/>
            </Border>
          </Grid>
          <Border CornerRadius="12" Background="#0B1220" Margin="0,12,0,0">
            <UniformGrid Columns="3">
              <RadioButton x:Name="PhOff" GroupName="ph" Content="オフ" Style="{StaticResource Seg}" Tag="#4B5563"/>
              <RadioButton x:Name="PhView" GroupName="ph" Content="見るだけ" Style="{StaticResource Seg}" Tag="#2563EB"/>
              <RadioButton x:Name="PhOp" GroupName="ph" Content="操作も可" Style="{StaticResource Seg}" Tag="#16A34A"/>
            </UniformGrid>
          </Border>
          <Grid Margin="0,10,0,0">
            <TextBlock x:Name="PhoneLeft" FontSize="12" Foreground="#94A3B8" Text=" "/>
          </Grid>
          <ProgressBar x:Name="PhoneBar" Style="{StaticResource Bar}" Maximum="30" Value="0" Foreground="#3B82F6" Margin="0,6,0,0"/>
        </StackPanel>
      </Border>

      <!-- PC -->
      <Border CornerRadius="14" Background="#111C2E" Padding="16,14">
        <StackPanel>
          <Grid>
            <StackPanel Orientation="Horizontal" VerticalAlignment="Center">
              <TextBlock Text="💻" FontSize="15" FontFamily="Segoe UI Emoji" VerticalAlignment="Center" Margin="0,0,8,0"/>
              <TextBlock Text="PC" FontSize="16" FontWeight="SemiBold" Foreground="White" VerticalAlignment="Center"/>
            </StackPanel>
            <Border x:Name="PcPill" HorizontalAlignment="Right" CornerRadius="10" Padding="10,2" Background="#6B7280">
              <TextBlock x:Name="PcState" Text="オフ" FontSize="12" Foreground="White" FontWeight="SemiBold"/>
            </Border>
          </Grid>
          <Border CornerRadius="12" Background="#0B1220" Margin="0,12,0,0">
            <UniformGrid Columns="3">
              <RadioButton x:Name="PcOff" GroupName="pc" Content="オフ" Style="{StaticResource Seg}" Tag="#4B5563"/>
              <RadioButton x:Name="PcView" GroupName="pc" Content="見るだけ" Style="{StaticResource Seg}" Tag="#2563EB"/>
              <RadioButton x:Name="PcOp" GroupName="pc" Content="操作も可" Style="{StaticResource Seg}" Tag="#16A34A"/>
            </UniformGrid>
          </Border>
          <TextBlock x:Name="PcLeft" FontSize="12" Foreground="#94A3B8" Margin="0,10,0,0" Text=" "/>
          <ProgressBar x:Name="PcBar" Style="{StaticResource Bar}" Maximum="30" Value="0" Foreground="#3B82F6" Margin="0,6,0,0"/>

          <Border CornerRadius="12" Background="#1A1625" BorderBrush="#3B2A1A" BorderThickness="1" Padding="14,12" Margin="0,14,0,0">
            <Grid>
              <StackPanel VerticalAlignment="Center" Margin="0,0,130,0">
                <TextBlock Text="パスワード・セキュリティ・お金" FontSize="13" FontWeight="SemiBold" Foreground="#FBBF24"/>
                <TextBlock x:Name="AllowText" Text="許可していません" FontSize="12" Foreground="#A8A29E" Margin="0,3,0,0"/>
              </StackPanel>
              <Button x:Name="BtnAllow" HorizontalAlignment="Right" VerticalAlignment="Center" Style="{StaticResource Btn}">
                <Button.Background>
                  <LinearGradientBrush StartPoint="0,0" EndPoint="1,1">
                    <GradientStop Color="#F59E0B" Offset="0"/><GradientStop Color="#DC2626" Offset="1"/>
                  </LinearGradientBrush>
                </Button.Background>
                <TextBlock Text="🔐 10 分 許可" FontFamily="Segoe UI Emoji, Yu Gothic UI" FontWeight="SemiBold"/>
              </Button>
            </Grid>
          </Border>
        </StackPanel>
      </Border>

      <Grid Margin="0,14,0,0">
        <TextBlock Text="Claude からの頼みは「許可しますか？」で出ます" FontSize="11" Foreground="#475569" VerticalAlignment="Center"/>
        <Button x:Name="BtnAllOff" Content="すべてオフ" HorizontalAlignment="Right" Style="{StaticResource Btn}" Background="#334155" FontSize="13"/>
      </Grid>
    </StackPanel>
  </Border>
</Window>
'@
$win = [Windows.Markup.XamlReader]::Load((New-Object System.Xml.XmlNodeReader $xaml))
$c = @{}
foreach ($n in 'Header','BtnClose','PhonePill','PhoneState','PhOff','PhView','PhOp','PhoneLeft','PhoneBar',
               'PcPill','PcState','PcOff','PcView','PcOp','PcLeft','PcBar','AllowText','BtnAllow','BtnAllOff') { $c[$n] = $win.FindName($n) }

function Brush($hex) { return (New-Object System.Windows.Media.BrushConverter).ConvertFromString($hex) }

function Show-Side($mode, $dir, $pill, $state, $rOff, $rView, $rOp, $leftText, $bar) {
    $state.Text = $names[$mode]
    $pill.Background = Brush $colors[$mode]
    $rOff.IsChecked = ($mode -eq 'off'); $rView.IsChecked = ($mode -eq 'view'); $rOp.IsChecked = ($mode -eq 'operate')
    if ($mode -eq 'off') {
        $leftText.Text = 'Claude はこの機器を使えません'; $bar.Value = 0
    } else {
        $l = Get-Left $dir; $min = [Math]::Max(0, [int][Math]::Ceiling($l.TotalMinutes))
        $leftText.Text = "使わないまま あと $min 分で自動オフ"
        $bar.Value = $min; $bar.Foreground = Brush $colors[$mode]
    }
}

function Update-View {
    $m1 = Get-Mode $Phone; $m2 = Get-Mode $Pc
    Show-Side $m1 $Phone $c.PhonePill $c.PhoneState $c.PhOff $c.PhView $c.PhOp $c.PhoneLeft $c.PhoneBar
    Show-Side $m2 $Pc $c.PcPill $c.PcState $c.PcOff $c.PcView $c.PcOp $c.PcLeft $c.PcBar
    $al = Get-AllowLeft
    if ($al) { $c.AllowText.Text = "許可中　あと $([int][Math]::Ceiling($al.TotalMinutes)) 分"; $c.AllowText.Foreground = Brush '#FCA5A5' }
    else     { $c.AllowText.Text = '許可していません（Claude は触れない）'; $c.AllowText.Foreground = Brush '#A8A29E' }
    $tray.Text = ("Claude 操作スイッチ  iPhone：{0} / PC：{1}{2}" -f $names[$m1], $names[$m2], $(if ($al) { '・許可中' } else { '' }))
}

$c.PhOff.Add_Click({ Set-PhoneMode 'off'; Update-View })
$c.PhView.Add_Click({ Set-PhoneMode 'view'; Update-View })
$c.PhOp.Add_Click({ Set-PhoneMode 'operate'; Update-View })
$c.PcOff.Add_Click({ Set-PcMode 'off'; Update-View })
$c.PcView.Add_Click({ Set-PcMode 'view'; Update-View })
$c.PcOp.Add_Click({ Set-PcMode 'operate'; Update-View })
$c.BtnAllow.Add_Click({ Grant-PcAllow; Update-View })
$c.BtnAllOff.Add_Click({ Set-PhoneMode 'off'; Set-PcMode 'off'; Update-View })
$c.BtnClose.Add_Click({ $win.Hide() })
$c.Header.Add_MouseLeftButtonDown({ $win.DragMove() })
$win.Add_Closing({ param($s, $e) if (-not $script:quit) { $e.Cancel = $true; $win.Hide() } })

# ---------------- 通知領域 ----------------
$tray = New-Object System.Windows.Forms.NotifyIcon
if (Test-Path $IconFile) { $tray.Icon = New-Object System.Drawing.Icon($IconFile) } else { $tray.Icon = [System.Drawing.SystemIcons]::Shield }
$tray.Visible = $true
$menu = New-Object System.Windows.Forms.ContextMenuStrip
$menu.Items.Add('開く', $null, { $win.Show(); $win.Activate() }) | Out-Null
$menu.Items.Add('すべてオフ', $null, { Set-PhoneMode 'off'; Set-PcMode 'off'; Update-View }) | Out-Null
$menu.Items.Add('終わる', $null, { $script:quit = $true; $tray.Visible = $false; $win.Close(); $app.Shutdown() }) | Out-Null
$tray.ContextMenuStrip = $menu
$tray.Add_MouseClick({ param($s, $e) if ($e.Button -eq 'Left') { $win.Show(); $win.Activate() } })

# ---------------- Claude からの頼み（devkit\requests\*.txt） ----------------
# ファイル名：iphone-view / iphone-operate / pc-view / pc-operate / pc-allow（.txt）。中身は理由（1 行）
[xml]$askXaml = @'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation"
        xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"
        Width="420" SizeToContent="Height" WindowStyle="None" AllowsTransparency="True" Background="Transparent"
        ResizeMode="NoResize" WindowStartupLocation="CenterScreen" Topmost="True" FontFamily="Yu Gothic UI" ShowInTaskbar="True"
        Title="Claude からの頼み">
  <Border CornerRadius="18" Background="#0F172A" BorderBrush="#334155" BorderThickness="1" Margin="14">
    <Border.Effect><DropShadowEffect BlurRadius="24" ShadowDepth="0" Opacity="0.6"/></Border.Effect>
    <StackPanel Margin="24,20">
      <TextBlock Text="Claude からの頼み" FontSize="13" Foreground="#64748B"/>
      <TextBlock x:Name="What" FontSize="18" FontWeight="Bold" Foreground="White" TextWrapping="Wrap" Margin="0,6,0,0"/>
      <TextBlock x:Name="Why" FontSize="13" Foreground="#94A3B8" TextWrapping="Wrap" Margin="0,8,0,0"/>
      <TextBlock x:Name="Count" FontSize="11" Foreground="#475569" Margin="0,12,0,0"/>
      <Grid Margin="0,16,0,0">
        <Grid.ColumnDefinitions><ColumnDefinition/><ColumnDefinition Width="12"/><ColumnDefinition/></Grid.ColumnDefinitions>
        <Button x:Name="No" Grid.Column="0" IsDefault="True" IsCancel="True" Cursor="Hand">
          <Button.Template><ControlTemplate TargetType="Button"><Border CornerRadius="10" Background="#334155" Padding="0,11">
            <TextBlock Text="断る" Foreground="White" FontSize="14" HorizontalAlignment="Center" FontWeight="SemiBold"/></Border></ControlTemplate></Button.Template>
        </Button>
        <Button x:Name="Yes" Grid.Column="2" Cursor="Hand">
          <Button.Template><ControlTemplate TargetType="Button"><Border CornerRadius="10" Padding="0,11">
            <Border.Background><LinearGradientBrush StartPoint="0,0" EndPoint="1,1"><GradientStop Color="#22C55E" Offset="0"/><GradientStop Color="#15803D" Offset="1"/></LinearGradientBrush></Border.Background>
            <TextBlock Text="許可する" Foreground="White" FontSize="14" HorizontalAlignment="Center" FontWeight="SemiBold"/></Border></ControlTemplate></Button.Template>
        </Button>
      </Grid>
    </StackPanel>
  </Border>
</Window>
'@

function Ask($what, $why, $seconds) {
    $w = [Windows.Markup.XamlReader]::Load((New-Object System.Xml.XmlNodeReader $askXaml))
    $w.FindName('What').Text = $what
    $w.FindName('Why').Text = $(if ($why) { "理由：$why" } else { '' })
    $script:answer = $false
    $w.FindName('Yes').Add_Click({ $script:answer = $true; $w.Close() })
    $w.FindName('No').Add_Click({ $w.Close() })
    $w.Add_MouseLeftButtonDown({ $w.DragMove() })
    $end = (Get-Date).AddSeconds($seconds)
    $cnt = $w.FindName('Count')
    $t = New-Object System.Windows.Threading.DispatcherTimer
    $t.Interval = [TimeSpan]::FromSeconds(1)
    $t.Add_Tick({ $s = [int]($end - (Get-Date)).TotalSeconds; if ($s -le 0) { $w.Close() } else { $cnt.Text = "あと $s 秒で自動的に断ります" } })
    $cnt.Text = "あと $seconds 秒で自動的に断ります"
    $t.Start()
    [void]$w.ShowDialog()
    $t.Stop()
    return $script:answer
}

$script:asking = $false
function Check-Requests {
    if ($script:asking) { return }
    $f = Get-ChildItem $Req -Filter '*.txt' -ErrorAction SilentlyContinue | Sort-Object LastWriteTime | Select-Object -First 1
    if (-not $f) { return }
    $why = ((Get-Content $f.FullName -Raw -Encoding UTF8 -ErrorAction SilentlyContinue) + '').Trim()
    $kind = $f.BaseName.ToLower()
    $age = (Get-Date) - $f.LastWriteTime
    Remove-Item $f.FullName -ErrorAction SilentlyContinue
    if ($age -gt [TimeSpan]::FromMinutes(2)) { return }   # 古い頼み（Claude がもう待っていない）は捨てる
    $what = switch ($kind) {
        'iphone-view'    { 'iPhone を「見るだけ」にしますか？' }
        'iphone-operate' { 'iPhone を「操作も可」にしますか？' }
        'pc-view'        { 'PC を「見るだけ」にしますか？' }
        'pc-operate'     { 'PC を「操作も可」にしますか？' }
        'pc-allow'       { 'パスワード・セキュリティ・お金にかかわる操作を 10 分だけ許可しますか？' }
        default          { $null }
    }
    if (-not $what) { return }
    $script:asking = $true
    $sec = $(if ($kind -eq 'pc-allow') { 80 } else { 55 }) - [int]$age.TotalSeconds
    if ($sec -gt 5 -and (Ask $what $why $sec)) {
        switch ($kind) {
            'iphone-view'    { Set-PhoneMode 'view' }
            'iphone-operate' { Set-PhoneMode 'operate' }
            'pc-view'        { Set-PcMode 'view' }
            'pc-operate'     { Set-PcMode 'operate' }
            'pc-allow'       { Grant-PcAllow }
        }
    }
    $script:asking = $false
    Update-View
}

if (Test-Path $IconFile) { $win.Icon = [System.Windows.Media.Imaging.BitmapFrame]::Create([Uri]$IconFile) }

# 2 つ目を開こうとしたら（ショートカットのダブルクリックなど）、この窓を前に出す
$showTimer = New-Object System.Windows.Threading.DispatcherTimer
$showTimer.Interval = [TimeSpan]::FromMilliseconds(400)
$showTimer.Add_Tick({ if ($showSignal.WaitOne(0)) { $win.Show(); if ($win.WindowState -eq 'Minimized') { $win.WindowState = 'Normal' }; $win.Activate(); $win.Topmost = $true; $win.Topmost = $false } })
$showTimer.Start()

$timer = New-Object System.Windows.Threading.DispatcherTimer
$timer.Interval = [TimeSpan]::FromSeconds(2)
$timer.Add_Tick({ Update-View; Check-Requests })
$timer.Start()

$app = New-Object System.Windows.Application
$app.ShutdownMode = 'OnExplicitShutdown'
Update-View
if ($args -notcontains '/tray') { $win.Show() }
[void]$app.Run()
$tray.Dispose()
$mutex.ReleaseMutex()
