# Profile 编写

Profile 是声明式 YAML：描述**数据源**、**信号整形**和**规则**。
适配新游戏通常只需写 YAML，不用改代码。

## 最小示例

```yaml
name: my-game
title: 我的游戏
source:
  type: trigger          # trigger | udp | audio | ffb | audio_ai | combo | sim
  rate_hz: 125

safety:
  signal_ttl: 500ms      # 信号超时视为失效
  release: 200ms         # 规则失效后力反馈衰减时长

rules:
  - name: l2-aim
    when: 'l2 > 0.02 && l2 < 0.55'
    target: l2
    effect: rigid
    priority: 10
    params:
      start: "0"
      force: "150"
```

## 规则字段

| 字段 | 说明 |
|---|---|
| `when` | 表达式，为真时规则生效；省略=恒真 |
| `target` | `l2` `r2` `rumble_left` `rumble_right` `haptic_left` `haptic_right` `lightbar` `player_leds` `mute_led` |
| `effect` | 扳机模式：`off` `rigid` `section` `feedback` `weapon` `vibration` `slope_feedback` `slope_weapon` `machine` |
| `params` | 效果参数，值为表达式 |
| `value` | 马达/音圈/LED 的数值表达式 |
| `priority` | 同一 target 上高优先级**整组覆盖**低优先级 |
| `hold` | 条件转假后继续保持的时长（做脉冲用） |
| `modulate` | 调幅：`{type: sine|square, hz, depth}` |

## 表达式

基于 [expr-lang](https://expr-lang.org)：

```yaml
when: 'pressed("r2_full") && speed > 10'
value: "clamp(220 * bass, 0, 240)"
```

| 函数 | 说明 |
|---|---|
| `has("sig")` | 信号在有效周期内 |
| `pressed(sig)` / `released(sig)` | 以 0.5 为阈值的边沿 |
| `crossed(sig, level)` | **任意阈值**的上升沿（推荐给游戏音量） |
| `held(sig)` | 电平 ≥ 0.5 |
| `clamp/lerp/smoothstep/min/max/round/sqrt/pow/ceil/floor` | 数学 |
| `absf(x)` | 绝对值（`abs` 被 ABS 信号占用） |
| `sin/cos/sec()` | 周期函数 |
| `sec()` | 进程运行秒数 |

## 数据源

| type | 需要什么 | 适用 |
|---|---|---|
| `trigger` | 手柄 | 任何游戏（读自身扳机/按键） |
| `udp` | 游戏遥测 | Forza / BeamNG / 自定义字段 |
| `audio` | PipeWire 工具 | 任何发声的游戏（包络 + 可选 AI） |
| `ffb` | `ds5vpad` | 有标准震动的游戏 |
| `audio_ai` | 外部探测器（UDP） | 平台无关的 AI 方案 |
| `combo` | 多个子源 | 例如 `trigger + audio`（射击游戏） |
| `sim` | 无 | 离线开发 |

`combo` 示例（射击游戏必需，因为单一 source 拿不到两类输入）：

```yaml
source:
  type: combo
  parts:
    - type: trigger
    - type: audio
      target: auto
      rate: 16000
      model: models/yamnet.onnx
```

## 校验与调试

```bash
./bin/ds5ctl check -profile my.yaml     # 编译期校验表达式
./bin/ds5d -profile my.yaml -trace      # 观察实时信号
```

> 自定义 profile 放 `profiles/`，用 `-profiles ./profiles -profile my-game` 加载，
> 免去重新编译（内置 profile 编译进二进制）。
