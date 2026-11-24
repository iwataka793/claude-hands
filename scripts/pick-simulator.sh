#!/usr/bin/env bash
# Actions の Mac に入っている iPhone シミュレーターから1台選び、その UDID を出力する。
# 新しい iOS の iPhone を優先する。
set -euo pipefail

xcrun simctl list devices available -j | python3 -c '
import json, sys, re
data = json.load(sys.stdin)["devices"]
best = None
for runtime, devices in data.items():
    m = re.search(r"iOS-(\d+)-(\d+)", runtime)
    if not m:
        continue
    version = (int(m.group(1)), int(m.group(2)))
    for d in devices:
        if not d["name"].startswith("iPhone"):
            continue
        key = (version, d["name"])
        if best is None or key > best[0]:
            best = (key, d["udid"], d["name"], runtime)
if best is None:
    sys.exit("iPhone のシミュレーターが見つからない")
print(best[1])
print(f"選んだシミュレーター: {best[2]} / {best[3]}", file=sys.stderr)
'
