# 扳机效果与参数

每个扳机通道在 HID 报告里占 11 字节（1 字节模式 + 10 字节参数）。

| 模式 | ID | 参数 | 手感 |
|---|---|---|---|
| `off` | 0x00 | — | 无阻力 |
| `rigid` | 0x01 | `start`(0..9), `force`(0..255) | 从指定分区开始恒定阻力（**做挡墙用这个**） |
| `section`/`pulse` | 0x02 | `start`, `end` | 仅区间内阻力 |
| `feedback` | 0x21 | `start`, `strength`, `force` | 随行程递增（部分固件上很弱） |
| `weapon` | 0x22 | `start`, `amp_a`, `amp_b`, `freq` | 两段式击发（机械"咔哒"） |
| `vibration` | 0x23 | `start`, `amplitude`, `freq` | 区间高频振动 |
| `slope_feedback` | 0x24 | `start`, `end`, `start_force`, `end_force` | 斜坡阻力（同上，可能很弱） |
| `slope_weapon` | 0x25 | `start`, `end`, `amp_a`, `amp_b`, `freq` | 带梯度两段式 |
| `machine` | 0x26 | `start`, `end`, `freq`, `period` | 周期脉冲 |

`start`/`end` 是**行程分区 0..9**（每格约 11%），不是百分比。
所有模式都支持裸参数 `p0..p9`。

## 实测建议（fw 0x0110002a）

> **`feedback` / `slope_feedback` 在这块固件上几乎无手感**，
> 做挡墙请用 `rigid` + 高 `force`（220~255），这是唯一明确可感的模式。

## COD 风格三段式（推荐）

```yaml
- name: l2-pull
  when: "l2 > 0.02 && l2 < 0.45"
  target: l2
  effect: rigid
  params: {start: "0", force: "90"}

- name: l2-wall
  when: "l2 >= 0.45"
  target: l2
  effect: rigid
  priority: 20
  params: {start: "4", force: "235"}      # 44% 处的硬墙

- name: r2-wall
  when: "r2 >= 0.70 && r2 < 0.97"
  target: r2
  effect: rigid
  priority: 20
  params: {start: "6", force: "255"}      # 67% 处的枪机

- name: r2-fired
  when: "r2 >= 0.97"
  target: r2
  effect: off
  priority: 30

- name: r2-mech-click                     # 击发瞬间的"咔哒"
  when: 'pressed("r2_full")'
  target: r2
  effect: weapon
  priority: 40
  hold: 80ms
  params: {start: "6", amp_a: "210", amp_b: "130", freq: "24"}
```

## 马达调幅

```yaml
modulate: { type: sine,   hz: 16, depth: 0.85 }   # ABS 脉冲
modulate: { type: square, hz: 30, depth: 1.0  }   # 碰撞/机械
```

`depth` 越小，最强与最弱的差距越小；`depth: 0.85` 时平均幅度只有 ~57%，
想要"硬"的颗粒感请用 `square`。

## 对比测试

```bash
./bin/ds5ctl bench -trigger both     # 轮巡 8 种模式
./bin/ds5ctl test                    # 震动 + L2/R2 阻力自检
```
