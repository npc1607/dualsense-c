#!/usr/bin/env bash
# Emit the release body for a tag on stdout.
#
#   .github/scripts/release-notes.sh v0.1.2 > notes.md
#
# Body = CHANGELOG section for the tag + what is in the archive + quick start +
# compare link. Keeping it a script (instead of inline YAML) means it can be
# run and reviewed locally.
set -euo pipefail

tag="${1:-}"
if [ -z "$tag" ]; then
    echo "usage: $0 <tag>" >&2
    exit 2
fi
repo="${GITHUB_REPOSITORY:-npc1607/dualsense-c}"

# awk: from "## <tag>" to the next "## " heading.
section="$(awk -v tag="$tag" '
    /^## / {
        if (found) exit
        if ($2 == tag) { found = 1; next }
    }
    found { print }
' CHANGELOG.md 2>/dev/null || true)"

if [ -z "${section//[[:space:]]/}" ]; then
    section="_这个版本没有 CHANGELOG 条目，改动见文末 compare 链接。_"
fi

prev="$(git describe --tags --abbrev=0 "${tag}^" 2>/dev/null || true)"
if [ -n "$prev" ]; then
    compare="https://github.com/${repo}/compare/${prev}...${tag}"
    echo "<!-- previous tag: ${prev} -->" >&2
else
    compare="https://github.com/${repo}/commits/${tag}"
fi

printf '%s\n\n' "$section"

cat <<'DOC'
---

## 包里有什么

| 文件 | 平台 | 用途 |
| --- | --- | --- |
| `ds5d` / `ds5d.exe` | 全部 | **主程序**：加载 profile → 收信号 → 算规则 → 125~250 Hz 写手柄，常驻后台 |
| `ds5ctl` / `ds5ctl.exe` | 全部 | **运维工具**：`list` 找手柄、`test` 自检、`bench` 挑扳机效果、`watch` 校准按键位、`sniff` 校准 UDP 偏移、`check` 验 profile |
| `ds5bridge.exe` | 全部（Windows 程序） | 仅 **AC / ACC / AC Rally** 需要：放进游戏的 Proton 前缀读共享内存并回传 UDP |
| `profiles/` | 全部 | 自定义 profile 模板（内置 6 个已编进 `ds5d`） |
| `udev/70-dualsense.rules` | Linux | 一次性安装，免 root 写 `/dev/hidraw*` |
| `scripts/launch-bridge.sh` | Linux | 把桥塞进指定 Steam AppID 的 Proton 前缀 |
| `SHA256SUMS` / `*.sha256` | — | 校验下载：`sha256sum -c SHA256SUMS` |

## 快速开始

```bash
tar -xzf ds5c-linux-amd64.tar.gz && cd ds5c-linux-amd64
sudo cp udev/70-dualsense.rules /etc/udev/rules.d/    # 一次，然后重新插拔手柄
./ds5ctl list && ./ds5ctl test                        # 认手柄 + 硬件自检
./ds5d -profile fps                                   # 射击游戏（任意游戏可用）
./ds5d -profile forza-horizon                         # 有遥测的赛车游戏
```

AC / ACC / AC Rally 额外一步：`./scripts/launch-bridge.sh <steam-appid>`。
DOC

printf '\n完整说明见 [README](https://github.com/%s#readme)。\n\n' "$repo"
printf '**Full Changelog**: %s\n' "$compare"
