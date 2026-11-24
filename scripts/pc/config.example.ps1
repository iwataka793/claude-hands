# PC だけに置く設定（公開 repo には入れない）。
# このファイルを %USERPROFILE%\devkit\config.ps1 にコピーして、値を自分のものにする。
$DevkitConfig = @{
    # go-ios の場所
    IosExe        = Join-Path $env:USERPROFILE "devkit\go-ios\ios.exe"
    # LiveContainer の bundle id（SideStore が付けたチーム ID 入り。例: com.kdt.livecontainer.ABCDE12345）
    LiveContainer = "com.kdt.livecontainer.XXXXXXXXXX"
    # WebDriverAgent のテスト本体の名前（SideStore 向け IPA では .xctest を外してある）
    WdaTestName   = "WebDriverAgentRunner"
}
