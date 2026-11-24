#!/usr/bin/env python3
"""LiveContainer に PC から直接入れるためのファイルを作る（Safari での手作業をなくす）。

LiveContainer は起動前に LCAppInfo.plist の LCPatchRevision を見て、古ければ実行ファイルに
パッチを当てて自分の証明書で署名し直す（LiveContainer の LCAppInfo.m patchExecAndSignIfNeed）。
そこで、新しい .app の中身 ＋ LCPatchRevision を消した LCAppInfo.plist を用意して、
go.ps1 の pushdir で /Documents/Applications/<bundle id>.app に上書きする。

使い方: prepare.py <新しい IPA> <iPhone から取った今の LCAppInfo.plist> <出力フォルダ>
"""
import os
import plistlib
import sys
import zipfile


def main(ipa: str, lcinfo: str, out: str) -> None:
    z = zipfile.ZipFile(ipa)
    apps = sorted({n.split("/")[1] for n in z.namelist() if n.startswith("Payload/") and n.count("/") >= 2 and n.split("/")[1].endswith(".app")})
    if len(apps) != 1:
        sys.exit(f"Payload の .app が 1 つではありません: {apps}")
    prefix = f"Payload/{apps[0]}/"
    os.makedirs(out, exist_ok=True)
    n = 0
    for info in z.infolist():
        if not info.filename.startswith(prefix) or info.filename.endswith("/"):
            continue
        rel = info.filename[len(prefix):]
        dst = os.path.join(out, *rel.split("/"))
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        with open(dst, "wb") as f:
            f.write(z.read(info.filename))
        n += 1
    bundle_id = plistlib.loads(z.read(prefix + "Info.plist"))["CFBundleIdentifier"]

    d = plistlib.load(open(lcinfo, "rb"))
    old = d.pop("LCPatchRevision", None)
    with open(os.path.join(out, "LCAppInfo.plist"), "wb") as f:
        plistlib.dump(d, f, fmt=plistlib.FMT_BINARY)
    print(f"{apps[0]}（{bundle_id}）のファイル {n} 個 ＋ LCAppInfo.plist（LCPatchRevision {old} → 削除）")
    print(f"入れる先: /Documents/Applications/{bundle_id}.app")


if __name__ == "__main__":
    main(*sys.argv[1:4])
