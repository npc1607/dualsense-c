# GitHub Wiki 源文件

这些 Markdown 就是项目 wiki 的页面，通过 `scripts/publish-wiki.sh` 推送到
`https://github.com/npc1607/dualsense-c.wiki.git`。

第一次需要手动建 wiki 仓库（GitHub 没有 API）：

1. 打开 <https://github.com/npc1607/dualsense-c/wiki>
2. 点 **Create the first page**，标题填 `Home`，随便保存
3. 回来运行：

```bash
./scripts/publish-wiki.sh
# 之后每次改完再跑一次即可
```

页面间链接用**页面名**（不带 `.md`），例如 `[快速开始](Getting-Started)`。
`_Sidebar.md` 是左侧导航，`_Footer.md` 是页脚。
