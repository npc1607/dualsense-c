#!/usr/bin/env bash
# Publish the pages in wiki/ to the project's GitHub wiki.
#
# One-time setup (GitHub has no API to create a wiki):
#   1. Open https://github.com/npc1607/dualsense-c/wiki
#   2. Click "Create the first page", save anything (e.g. title "Home")
#   -> the .wiki.git repository now exists and this script can push to it.
#
# usage: scripts/publish-wiki.sh ["commit message"]
set -uo pipefail

REPO_SLUG="${DS5_WIKI_SLUG:-npc1607/dualsense-c}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$HERE/wiki"
MSG="${1:-docs(wiki): sync pages}"

[ -d "$SRC" ] || { echo "missing $SRC" >&2; exit 1; }

URL="${DS5_WIKI_URL:-git@github.com:$REPO_SLUG.wiki.git}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if ! git ls-remote "$URL" >/dev/null 2>&1; then
	cat >&2 <<EOF
wiki 仓库还不存在：$URL

先做一次（只需一次）：
  1. 打开 https://github.com/$REPO_SLUG/wiki
  2. 点 "Create the first page"，随便存一页（标题用 Home）
然后重新运行本脚本。
EOF
	exit 1
fi

git clone --depth 1 "$URL" "$TMP/wiki" >/dev/null 2>&1 || {
	echo "clone 失败：$URL" >&2
	exit 1
}

# 同步公开文档并清理已废弃页面，排除内部 README.md
rm -f "$TMP/wiki"/*.md
cp "$SRC"/*.md "$TMP/wiki/" 2>/dev/null || true
rm -f "$TMP/wiki/README.md"
cp "$SRC"/*.svg "$TMP/wiki/" 2>/dev/null || true
cp "$SRC"/*.png "$TMP/wiki/" 2>/dev/null || true

cd "$TMP/wiki"
if git diff --quiet && git diff --cached --quiet && [ -z "$(git status --porcelain)" ]; then
	echo "wiki 已是最新，无需推送"
	exit 0
fi
git add -A
git -c user.name="${GIT_AUTHOR_NAME:-$(git config user.name || echo wiki-bot)}" \
    -c user.email="${GIT_AUTHOR_EMAIL:-$(git config user.email || echo wiki@local)}" \
    commit -q -m "$MSG"
git push origin HEAD 2>&1 | tail -3
echo "已发布到 https://github.com/$REPO_SLUG/wiki"
