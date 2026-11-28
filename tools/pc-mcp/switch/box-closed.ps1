# claude-hands：ClaudeBox から見せない場所（ClosedFilePath）の一覧。hands-protect.ps1（書く）と box-check.ps1（確かめる）が同じものを使う
#   名前（ラベル）と場所の組。場所は無くてもよい（Sandboxie は無い場所の決まりも受け付ける）
#   限界：知っている場所を並べるやり方なので、あとで入れたソフトの鍵の置き場所は漏れる。新しく鍵を置くソフトを入れたらここに足す（PC.md の「箱から見えない場所」）
function ClaudeBoxClosed([string]$uh, [string]$H) {
  $la = Join-Path $uh 'AppData\Local'
  $ra = Join-Path $uh 'AppData\Roaming'
  @(
    # 前から（2026-11-28 pc-52〜）：守りの場所・道具の置き場所・鍵・ブラウザのデータ・OneDrive
    @('守りの場所', $H), @('常駐役の場所', (Join-Path $env:ProgramData 'pc-agent')),
    @('常駐役の記録', "$uh\devkit\pc-agent"), @('iPhone の道具', "$uh\devkit\iphone-mcp"), @('作業の記録', "$uh\devkit\hands"),
    @('バックアップ', "$uh\devkit\backup"), @('ブラウザのスイッチ', "$uh\devkit\browser"), @('iPhone の鍵（go-ios）', "$uh\devkit\go-ios"),
    @('SSH の鍵', "$uh\.ssh"), @('Chrome のデータ', "$la\Google\Chrome\User Data"), @('Edge のデータ', "$la\Microsoft\Edge\User Data"), @('OneDrive', "$uh\OneDrive"),
    # 足した（2026-11-28 夜。道具の「鍵などの場所」の一覧に合わせる）：Windows の資格情報・証明書の鍵
    @('Windows の資格情報（Roaming）', "$ra\Microsoft\Credentials"), @('Windows の資格情報（Local）', "$la\Microsoft\Credentials"),
    @('Windows の資格情報の保管庫（Roaming）', "$ra\Microsoft\Vault"), @('Windows の資格情報の保管庫（Local）', "$la\Microsoft\Vault"),
    @('DPAPI の鍵', "$ra\Microsoft\Protect"), @('証明書の鍵', "$ra\Microsoft\Crypto"), @('個人の証明書', "$ra\Microsoft\SystemCertificates\My"),
    # 開発の鍵・トークン
    @('GitHub CLI のトークン', "$ra\GitHub CLI"), @('gcloud の鍵', "$ra\gcloud"), @('AWS の鍵', "$uh\.aws"), @('Azure の鍵', "$uh\.azure"),
    @('Kubernetes の鍵', "$uh\.kube"), @('GnuPG の鍵', "$uh\.gnupg"), @('Docker の鍵', "$uh\.docker"),
    @('Git の資格情報', "$uh\.git-credentials"), @('netrc', "$uh\.netrc"), @('netrc（_netrc）', "$uh\_netrc"), @('npm のトークン', "$uh\.npmrc"), @('PyPI のトークン', "$uh\.pypirc"),
    # ほかのブラウザ・パスワード管理
    @('Firefox のデータ', "$ra\Mozilla\Firefox\Profiles"), @('Brave のデータ', "$la\BraveSoftware\Brave-Browser\User Data"),
    @('Bitwarden', "$ra\Bitwarden"), @('1Password', "$la\1Password")
  )
}
