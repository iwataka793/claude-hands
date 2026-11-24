// wda-refresh：WebDriverAgent の IPA を署名して iPhone に入れ直す小さな道具（Windows 用）。
// iloader と同じ部品（isideload）を同じ設定で使う：
//   - パスワード：iloader が Windows の資格情報マネージャーに保存したもの（"iloader" / メールアドレス）
//   - 署名用の証明書・Anisette の状態：iloader と同じ保存場所（KeyringStorage "iloader"）を使い回す
//   - 証明書が上限に達していたら、勝手に取り消さずにエラーで止める（SideStore / LiveContainer の証明書を守るため）
//   - 2 段階認証を求められたら、自動実行では止める（手で実行したときだけ入力できる）
// 使い方：wda-refresh.exe <AppleIDのメール> <IPAのパス> [Anisetteサーバー] [UDID]
use idevice::usbmuxd::{UsbmuxdAddr, UsbmuxdConnection};
use isideload::{
    anisette::{AnisetteClientInfo, remote_v3::RemoteV3AnisetteProvider},
    auth::apple_account::{AppleAccount, TwoFactorCallbackParams, TwoFactorCallbackResponse},
    dev::developer_session::DeveloperSession,
    sideload::{
        SideloaderBuilder,
        builder::{ExtensionsBehavior, ExtensionsBehaviorSelection, MaxCertsBehavior},
    },
    util::{
        callbacks::{ExtensionsBehaviorCallbackBox, MaxCertsCallbackBox},
        keyring_storage::KeyringStorage,
    },
};
use std::{env, io::IsTerminal, path::PathBuf, process::exit};
use tracing::Level;
use tracing_subscriber::FmtSubscriber;

fn fail(code: i32, msg: &str) -> ! {
    eprintln!("RESULT=FAIL code={code} {msg}");
    exit(code)
}

#[tokio::main]
async fn main() {
    rustls::crypto::ring::default_provider()
        .install_default()
        .expect("Failed to install rustls crypto provider");
    let _ = isideload::init();
    let subscriber = FmtSubscriber::builder().with_max_level(Level::INFO).finish();
    let _ = tracing::subscriber::set_global_default(subscriber);

    let args: Vec<String> = env::args().collect();
    if args.len() < 3 {
        fail(2, "usage: wda-refresh <apple-id-email> <ipa-path> [anisette-host] [udid]");
    }
    let email = args[1].to_lowercase();
    let ipa = PathBuf::from(&args[2]);
    let anisette = args.get(3).cloned().unwrap_or_else(|| "mac-ani.jkcoxson.com".to_string());
    let anisette_url = if anisette.starts_with("http") { anisette } else { format!("https://{anisette}") };
    let want_udid = args.get(4).cloned();

    if !ipa.exists() {
        fail(2, &format!("IPA not found: {}", ipa.display()));
    }

    // 1. パスワード（iloader が保存したもの）
    let password = match keyring::Entry::new("iloader", &email).and_then(|e| e.get_password()) {
        Ok(p) => p,
        Err(e) => fail(3, &format!("password not found in Windows Credential Manager (iloader/{email}): {e}")),
    };

    // 2. サインイン（2 段階認証は、手で実行しているときだけ受け付ける）
    let interactive = std::io::stdin().is_terminal();
    let tfa = move |params: TwoFactorCallbackParams| async move {
        if !interactive {
            fail(4, "Apple asked for a 2FA code; run wda-refresh by hand once to enter it");
        }
        println!("2FA code (sent to {}):", if params.sms { "SMS" } else { "your devices" });
        let mut code = String::new();
        std::io::stdin().read_line(&mut code).ok();
        Ok(TwoFactorCallbackResponse::SubmitCode(code.trim().to_string()))
    };

    let account = AppleAccount::builder(&email)
        .anisette_provider(
            RemoteV3AnisetteProvider::default()
                .unwrap_or_else(|e| fail(5, &format!("anisette provider: {e}")))
                .set_serial_number("0".to_string())
                .set_storage(Box::new(KeyringStorage::new("iloader".to_string())))
                .set_client_info_override(Some(AnisetteClientInfo {
                    client_info: "<MacBookPro13,3> <Mac OS X;10.12.6;16G29> <com.apple.AuthKit/1 (com.apple.akd/1.0)>"
                        .to_string(),
                    user_agent: "akd/1.0 CFNetwork/811.5.4 Darwin/16.7.0".to_string(),
                }))
                .set_url(&anisette_url),
        )
        .login(&password, tfa)
        .await;
    let mut account = account.unwrap_or_else(|e| fail(6, &format!("login failed: {e}")));
    println!("logged in");

    let dev_session = DeveloperSession::from_account(&mut account)
        .await
        .unwrap_or_else(|e| fail(7, &format!("developer session: {e}")));

    // 3. iPhone（Apple Devices の usbmuxd 経由。USB でも同じ Wi-Fi でも可）
    let mut usbmuxd = UsbmuxdConnection::default()
        .await
        .unwrap_or_else(|e| fail(8, &format!("usbmuxd: {e:?}")));
    let devs = usbmuxd.get_devices().await.unwrap_or_else(|e| fail(8, &format!("get_devices: {e:?}")));
    let dev = match &want_udid {
        Some(u) => devs.iter().find(|d| &d.udid == u),
        None => devs.first(),
    };
    let dev = dev.unwrap_or_else(|| fail(9, "iPhone not found (is it on the same Wi-Fi or connected by USB?)"));
    println!("device {}", dev.udid);
    let provider = dev.to_provider(UsbmuxdAddr::from_env_var().unwrap(), "iloader");

    // 4. 署名して入れる（iloader と同じ設定。証明書の上限ではエラーで止める）
    let mut sideloader = SideloaderBuilder::<MaxCertsCallbackBox, ExtensionsBehaviorCallbackBox>::new(dev_session, email.clone())
        .machine_name("iloader".into())
        .storage(Box::new(KeyringStorage::new("iloader".to_string())))
        .extensions_behavior(ExtensionsBehaviorSelection::Fixed(ExtensionsBehavior::ReuseMain))
        .max_certs_behavior(MaxCertsBehavior::Error)
        .build();

    match sideloader
        .install_app(&provider, ipa, true, None::<fn(f32) -> std::future::Ready<()>>, None)
        .await
    {
        Ok(_) => println!("RESULT=OK installed"),
        Err(e) => fail(10, &format!("install failed: {e}")),
    }
}
