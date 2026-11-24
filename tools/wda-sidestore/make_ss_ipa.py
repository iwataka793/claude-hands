#!/usr/bin/env python3
"""SideStore（LiveContainer 同梱版）向けの WebDriverAgent IPA を作る。

SideStore の本番ビルドでは DEBUG が有効で、署名の準備（ResignAppOperation）が
PlugIns の中の「.xctest」を含む名前のものを削除してしまう。
そこで次のように作り替える：
  1. PlugIns/WebDriverAgentRunner.xctest → PlugIns/WebDriverAgentRunner（名前から .xctest を外す）
  2. テスト本体の CFBundleIdentifier を本体アプリと同じ ID にする
     （SideStore は PlugIns の中身ごとに「同じ ID のプロファイル」を探すため。本体の分しか無い）
起動は go-ios で --xctestconfig=WebDriverAgentRunner を指定する。

使い方: make_ss_ipa.py <元の IPA> <出力 IPA>
"""
import plistlib
import sys
import zipfile

OLD = "PlugIns/WebDriverAgentRunner.xctest/"
NEW = "PlugIns/WebDriverAgentRunner/"


def main(src: str, dst: str) -> None:
    zin = zipfile.ZipFile(src)
    app = None
    for n in zin.namelist():
        if n.startswith("Payload/") and n.count("/") == 2 and n.endswith(".app/"):
            app = n
    if app is None:
        sys.exit("Payload/*.app が見つかりません")
    main_info = plistlib.loads(zin.read(app + "Info.plist"))
    main_id = main_info["CFBundleIdentifier"]

    renamed = 0
    with zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
        for info in zin.infolist():
            data = zin.read(info.filename)
            name = info.filename
            if name.startswith(app + OLD):
                name = app + NEW + name[len(app + OLD):]
                renamed += 1
                if name == app + NEW + "Info.plist":
                    pl = plistlib.loads(data)
                    print(f"テスト本体の ID: {pl.get('CFBundleIdentifier')} → {main_id}")
                    pl["CFBundleIdentifier"] = main_id
                    data = plistlib.dumps(pl, fmt=plistlib.FMT_XML)
            elif name == app + OLD[:-1]:
                name = app + NEW[:-1]
            zi = zipfile.ZipInfo(name, date_time=info.date_time)
            zi.external_attr = info.external_attr
            zi.compress_type = zipfile.ZIP_STORED if name.endswith("/") else zipfile.ZIP_DEFLATED
            zout.writestr(zi, data)
    print(f"名前を変えた項目: {renamed} 個 / 本体 ID: {main_id}")
    bad = [n for n in zipfile.ZipFile(dst).namelist() if ".xctest" in n.lower()]
    if bad:
        sys.exit(f".xctest が残っています: {bad}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
