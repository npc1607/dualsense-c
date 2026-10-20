# prompts/

供 LLM 使用的提示词。文件整体作为 System Prompt，用户消息按文件末尾的「输入模板」填写。

| 文件 | 用途 |
|---|---|
| [profile-generator.md](profile-generator.md) | 生成 / 修正游戏 Profile：分类 → 遥测采集 → 字段校准 → 生成 → `ds5ctl check` + dry-run 校验 → 不合理则重生成 |

维护约定：提示词中的 Schema 摘要（target、effect、参数名、标准信号、表达式函数）来自
`internal/profile/profile.go`、`internal/hid/effect.go`、`internal/signal/names.go`，
修改这些文件时请同步更新对应提示词。
