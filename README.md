# dualsense-c

给 **PS5 DualSense (DS5)** 做一套**通用的自适应扳机 / 震动适配框架**：不是只为某一个游戏写死，而是把「信号源 → 信号 → 规则 → 手柄」拆成四层，游戏差异全部交给 YAML profile。

- **射击游戏（任意）**：不依赖游戏支持，读手柄自身的扳机行程，做「瞄准挡墙 + 开火后坐力」
- **自带遥测的游戏**：Forza 地平线 4/5（UDP 5300）、BeamNG / Live for Speed（OutGauge 4444）、尘埃拉力 / EA WRC（UDP 20777，带校准工具）
- **只有共享内存的游戏**：Assetto Corsa / ACC / AC Rally —— 交叉编译的 `ds5bridge.exe` 在 Proton 前缀内读 `acpmf_physics`，UDP 回传给原生守护进程
- **纯 Go、无 CGO**：Linux 直接用 `/dev/hidraw`，Windows 走 HID 设备接口（代码已就位，未在真机验证）
- **换游戏 = 改 YAML**：偏移、映射、手感参数都在 profile 里，不需要重新编译

```
游戏 ──► [Source 适配器] ──► [signal.Set] ──► [rules 表达式引擎] ──► [sink 固定时钟] ──► DualSense
         trigger / udp / sim     归一化 0..1      平滑/限速/锁存/衰减     125~250 Hz      hidraw / HID
              ▲                                        ▲
              └──────────── profiles/*.yaml ───────────┘
```

---

## 目录

- [为什么是"框架"而不是"脚本"](#为什么是框架而不是脚本)
- [快速开始](#快速开始)
- [游戏支持矩阵](#游戏支持矩阵)
- [命令行](#命令行)
- [Profile 格式](#profile-格式)
- [效果（effect）与参数](#效果effect与参数)
- [标准信号表](#标准信号表)
- [校准一个没有内置支持的游戏](#校准一个没有内置支持的游戏)
- [Windows](#windows)
- [故障排查](#故障排查)
- [目录结构](#目录结构)
- [协议参考](#协议参考)
- [Roadmap](#roadmap)

---

## 为什么是"框架"而不是"脚本"

大多数 DS5 方案把「读哪个游戏的哪块内存」和「怎么驱动扳机」写在一起，换游戏就要改代码。这里把它们彻底分开：

| 层 | 职责 | 换游戏时要改吗 |
|---|---|---|
| **Source** | 把一个游戏的状态变成标准信号（`trigger` 输入驱动 / `udp` 遥测 / `sim` 模拟） | 有原生遥测 → 改 YAML；否则不用改 |
| **signal.Set** | 统一归一化、时间戳、过期判定、deadzone/gamma/限速平滑 | 不用改 |
| **rules** | 声明式表达式把信号映射成扳机/马达/灯条效果，支持优先级、锁存、衰减 | 只改 YAML |
| **sink** | 固定 tick 写 HID 报告，热插拔、异常回收 | 不用改 |

三个关键性质：

1. **信号缺失不会误驱动**：规则里用 `has("brake")` 判断，信号过期（`signal_ttl`）后规则自动失效，而不是把刹车当成 0 把扳机锁死。
2. **断流不弹手**：效果消失后按 `release` 时长把推力衰减到 0，不会"啪"一下松开。
3. **profile 即文档**：`internal/profile/builtin/*.yaml` 就是可读的示例，也是默认配置。

---

## 快速开始

### 1. 依赖

- Go 1.24+
- Linux（hidraw）或 Windows（HID，实验性）
- DualSense，**建议 USB 连接**（蓝牙下自适应扳机不稳定；本仓库支持 BT 报告格式，但未在真机长时间验证）

### 2. 编译

```bash
make            # bin/ds5d  bin/ds5ctl
make windows    # bin/ds5d.exe  bin/ds5ctl.exe（交叉编译，纯 Go）
```

### 3. 授权 hidraw（Linux，一次性）

```bash
make install-udev
# 等价于：
#   sudo cp udev/70-dualsense.rules /etc/udev/rules.d/
#   sudo udevadm control --reload-rules && sudo udevadm trigger
```

然后**重新插拔一次手柄**。

### 4. 验证硬件

```bash
./bin/ds5ctl list        # 应看到一个 usb DualSense
./bin/ds5ctl test        # 震动渐强 → L2 阻力 → R2 阻力 → 反馈效果
./bin/ds5ctl bench       # 逐个试 8 种扳机效果，挑手感（ctrl-c 退出）
```

没有反应时用 `-hidraw /dev/hidrawN` 指定节点逐个试。

### 5. 跑起来

**射击游戏（任何游戏，零配置）**

```bash
./bin/ds5d -profile fps
```

L2 前 55% 行程有渐进阻力（瞄准"挡墙"），越过挡墙阻力消失；R2 到 85% 时右马达给一记后坐力脉冲。

**有遥测的赛车游戏**

```bash
./bin/ds5d -profile forza-horizon     # Forza 地平线 4/5、Forza Motorsport
./bin/ds5d -profile beamng            # BeamNG.drive、Live for Speed
```

**只有共享内存的游戏（AC / ACC / AC Rally）**

```bash
make bridge                                   # 交叉编译 bin/ds5bridge.exe
./bin/ds5d -profile assetto-corsa &           # 先起守护进程
./scripts/launch-bridge.sh 805550             # 把桥塞进该 AppID 的 Proton 前缀
# Steam AppID：AC 244210 | ACC 805550 | AC Rally 3917090
```

**不插手柄先验证配置**（profile 开发、CI 都能用）

```bash
./bin/ds5d -dry-run -profile fps -source sim -trace
```

### 6. 后台运行

```bash
nohup ./bin/ds5d -profile fps >> /tmp/ds5d.log 2>&1 &
```

---

## 游戏支持矩阵

### A. 输入驱动（不需要游戏配合，所有游戏都能用）

| 手段 | 说明 | 状态 |
|---|---|---|
| `trigger` 源 | 读手柄自身输入报告（L2/R2 行程、按键、摇杆），规则做挡墙/阻力/后坐力 | ✅ 可用 |

这条路对 FPS / TPS / 动作游戏最实用：游戏不需要任何设置，也不受 Steam Input 影响（前提是 Steam 不抢写震动）。

### B. 游戏自带遥测端口（真实数据，手感最好）

| 游戏 | 端口 | 解码器 | 状态 |
|---|---|---|---|
| Forza Horizon 4 / 5，Forza Motorsport 7 / 2023 | UDP **5300**（Data Out，格式必须选 **Dash**） | `forza` | ✅ 内置 |
| BeamNG.drive | UDP **4444**（Options → Other → OutGauge） | `outgauge` | ✅ 内置 |
| Live for Speed | UDP **4444**（Misc → OutGauge） | `outgauge` | ✅ 内置 |
| 尘埃拉力 2.0 / EA Sports WRC / DiRT 4 | UDP **20777**（Options → UDP Telemetry = On） | `dirt` + `fields` | ⚠️ 模板（偏移需现场校准，见下文） |
| F1 22 / 23 / 24 | UDP 20777（格式随年份变化） | `fields` | ⚠️ 模板 |
| Assetto Corsa / ACC / AC Rally | 共享内存 `acpmf_physics` → `ds5bridge.exe` → UDP **9999** | `acshm` | ✅ 内置（偏移沿用 acr-dualsense 实测值） |
| rFactor 2 / LMU、ETS2 / ATS | 共享内存 | — | 🚧 未实现 |
| iRacing | SDK 内存映射 | — | 🚧 未实现 |

> 为什么尘埃 / F1 只有模板：Codemasters 的包结构在 DR1 → DR2 → DiRT 4 → EA WRC 之间改过多次，同一版本还会交替发送多种长度的包，**内置猜测的偏移只会让扳机乱抖**。所以这里给的是「3 分钟校准流程 + 声明式字段表」，而不是假装支持。

### C. 完全无遥测、也无需输入驱动

兜底的音频 / 震动镜像方案见 [Roadmap](#roadmap)（`source/audio`、`source/ffb`）。

---

## 共享内存游戏（Assetto Corsa / ACC / AC Rally）

这类游戏的遥测是 Windows 命名共享内存（`acpmf_physics`）：游戏进程在 Proton 前缀里写，原生 Linux 进程**看不到**。所以只有"读共享内存"这一小段必须待在前缀里，其余全部原生：

```
acc.exe (Proton) ──写──> acpmf_physics（Wine 共享内存）
                              ▲
ds5bridge.exe (Windows, 同前缀) ──读──┘ → UDP 127.0.0.1:9999
                              ▼
ds5d（Linux 原生）→ 规则 → DualSense
```

```bash
make bridge                                  # 只交叉编译桥，纯 Go 不需要 mingw
./bin/ds5d -profile assetto-corsa &          # 守护进程先起，会等数据
./scripts/launch-bridge.sh 805550            # ACC；AC=244210, AC Rally=3917090
# 或者手动：
protontricks-launch --appid 805550 ./bin/ds5bridge.exe -layout acc -debug
```

桥的行为：

- 每 2 秒重试一次共享内存，**可以先于游戏启动**
- `packetId` 超过 2 秒不变 → 认为本局结束，自动重新打开映射（不用重启桥）
- 数值不合理时打印 `layout mismatch?` 并跳过，不会把乱数送进扳机

**没有游戏也想验证链路**（`tools/bridgesim` 模拟桥的输出）：

```bash
go run ./tools/bridgesim -port 9999          # 终端 A
./bin/ds5d -dry-run -profile assetto-corsa -trace   # 终端 B
```

### 桥的线格式

自描述，不是定长 6 通道：

```
0  magic   uint32  "DS5B" (0x42355344)
4  version uint16  1
6  count   uint8
7  _       uint8
8  fields[count] { id uint8, value float32 }   // id = internal/signal.Standard 的下标
```

新增字段只要改 `internal/bridge/layout.go`，守护进程侧无需改动。

---

## 命令行

### `ds5d` —— 守护进程

| 参数 | 默认 | 说明 |
|---|---|---|
| `-profile` | `fps` | profile 名（内置）或 `.yaml` 路径 |
| `-profiles` | 空 | 额外的 profile 目录 |
| `-source` | 空 | 覆盖 profile 的 source 类型（如 `sim`、`trigger`、`udp`） |
| `-hidraw` | 自动 | 强制设备路径 |
| `-rate` | `125` | 每秒输出报告数（DS5 USB 最高可到 250） |
| `-dry-run` | 关 | 不开手柄，只打印帧（可在 macOS 上开发 profile） |
| `-trace` | 关 | 每秒两次打印信号值 |
| `-list` | 关 | 列出内置 profile 后退出 |
| `-version` | 关 | 打印版本（release 构建时注入 tag） |

### `ds5ctl` —— 维护工具

| 命令 | 说明 |
|---|---|
| `list` | 枚举手柄 |
| `profiles [-dir d]` | 列出内置/自定义 profile |
| `check -profile p` | **只编译规则**，不开手柄，验证 YAML 写对没有 |
| `test` | 震动 + 扳机渐变自检 |
| `bench [-trigger l2\|r2\|both]` | 逐个播放 8 种扳机效果 |
| `watch` | 打印解码后的输入报告（校准按键位表） |
| `sniff -listen :20777` | 十六进制 dump 游戏遥测包（校准 UDP 偏移） |
| `version` | 打印版本 |

---

## Profile 格式

```yaml
name: my-game                # 必填，profile 标识
title: 显示名称
description: |               # 随便写，用于 self-documenting
  ...

source:
  type: udp                  # trigger | udp | sim
  listen: "0.0.0.0:5300"
  decoder: forza             # forza | outgauge | dirt | fields | auto
  rate_hz: 60                # 采样上限
  # dump: true               # 每种包长 dump 一次，排查用

shape:                       # 可选：信号整形（在规则看到之前）
  brake:    { deadzone: 0.02, gamma: 1.0, slew_up: 60, slew_down: 200 }
  throttle: { deadzone: 0.02, gamma: 0.9 }

safety:
  signal_ttl: 500ms          # 多久没更新算"信号消失"
  release: 300ms             # 效果消失后的淡出时长（防弹手）
  status_lightbar: true      # 没有任何规则控制灯条时，用绿/黄表示 收到数据/空闲

signals: [my_extra_signal]   # 可选：额外的自定义信号名

rules:
  - name: l2-brake
    when: 'has("brake") && brake > 0.01'     # 可选；不写则一直生效
    target: l2                               # l2 | r2 | rumble_left | rumble_right | lightbar | player_leds
    effect: rigid                            # 见下表
    priority: 10                             # 越大越后应用（后写覆盖先写）
    hold: 0ms                                # 一次性脉冲的锁存时长
    params:                                  # 每个值都是一个表达式
      start: "0"
      force: "clamp(18 + 237 * brake, 0, 255)"

  - name: recoil
    when: 'pressed("r2_full")'
    target: rumble_right
    priority: 50
    hold: 130ms
    value: "230"
    modulate: { type: sine, hz: 24, depth: 0.65 }
```

### 表达式

规则里的 `when` / `params.*` / `value` 都是表达式，由 [expr-lang](https://expr-lang.org) 编译成字节码：

- 变量：所有[标准信号](#标准信号表)（缺失时为 `0`）
- 内置数学：`min` `max` `abs` `ceil` `floor` `round` `sqrt` `pow`
- 本项目补充：

| 函数 | 说明 |
|---|---|
| `clamp(x, lo, hi)` | 夹取 |
| `lerp(a, b, t)` | 线性插值 |
| `smoothstep(e0, e1, x)` | 平滑过渡（比 if 手感好） |
| `has("sig")` | 信号当前是否新鲜（bool） |
| `pressed("sig")` | 本帧刚越过 0.5（bool，边沿） |
| `released("sig")` | 本帧刚跌破 0.5（bool） |
| `held("sig")` | 当前 ≥ 0.5（bool） |
| `sec()` | 守护进程启动以来的秒数（做周期性效果） |

**编译期校验**：信号名拼错、函数名拼错、类型不匹配都会在启动时报错（`ds5ctl check` 可以离线验证），不会静默失效。

### 优先级与叠加

`priority` 小的先算，大的后算。同一个 target 上的后写规则会**整体覆盖**前一条（不是叠加数值），所以：

```yaml
  - {name: aim,  when: "l2 < 0.5", target: l2, effect: feedback, priority: 10, ...}
  - {name: off,  when: "l2 >= 0.5", target: l2, effect: off,     priority: 20}
```

越过挡墙时高优先级规则接管，把阻力完全释放。

---

## 效果（effect）与参数

DS5 的每根扳机在报告里是一段 11 字节：1 字节 mode + 10 字节参数。参数语义按 `hid-playstation` / 社区文档实现，**不同固件可能略有差异**，用 `ds5ctl bench` 实测确认。

| effect | mode | 语义参数 | 手感 |
|---|---|---|---|
| `off` | 0x00 | — | 松开 |
| `rigid` | 0x01 | `start`(0..9) `force`(0..255) | 从起点开始的持续阻力 |
| `section` / `pulse` | 0x02 | `start` `end` | 只在行程区间内有阻力 |
| `feedback` | 0x21 | `start` `strength` `force` | 阻力随行程变化（瞄准挡墙首选） |
| `weapon` | 0x22 | `start` `amp_a` `amp_b` `freq` | 两段式扳机（枪械扳机断点） |
| `vibration` | 0x23 | `start` `amplitude` `freq` | 区间内振动 |
| `slope_feedback` | 0x24 | `start` `end` `start_force` `end_force` | 阻力斜坡 |
| `slope_weapon` | 0x25 | `start` `end` `amp_a` `amp_b` `freq` | 带斜坡的两段式 |
| `machine` | 0x26 | `start` `end` `freq` `period` | 周期性脉冲（模拟枪） |

任何 mode 都支持裸参数 `p0`..`p9` 直接写线格式，便于试出社区还没记录的效果。

`modulate`（只对 `rumble_left/right` 生效）：

```yaml
modulate: { type: sine, hz: 14, depth: 0.85 }   # 正弦调幅，ABS 脉冲常用 14~20Hz
modulate: { type: square, hz: 30, depth: 1.0 }  # 方波，机械感
```

---

## 标准信号表

| 来源 | 信号 |
|---|---|
| 车辆遥测 | `throttle` `brake` `clutch` `steer` `speed`(km/h) `rpm`(0..1) `gear` `slip` `lockup` `abs` `tc` `rumble` `accel_g` `lat_g` `surface` |
| 手柄输入 | `l2` `r2`（0..1）`l2_full` `r2_full` `l1` `r1` `cross` `circle` `square` `triangle` `up` `down` `left` `right` `lx` `ly` `rx` `ry` `stick_l` `stick_r` `ps` `touchpad` `mute` |
| 自定义 | `custom0`..`custom3` |

约定：**归一化量都在 0..1**（`steer`/摇杆为 -1..1），速度 km/h，`rpm` 已按红线归一化。各解码器的具体映射见 `internal/source/udp/decoders.go` 顶部注释。

---

## 校准一个没有内置支持的游戏

以尘埃拉力为例（任何 UDP 遥测游戏同理）：

```bash
# 1. 抓包：进赛段，反复踩死/松开油门
./bin/ds5ctl sniff -listen :20777

# 2. 观察出现了几种包长，哪个 4 字节在 0.0~1.0 之间连续变化（多半是 f32 的油门/刹车）

# 3. 填进 profile 的 fields 块（不用改 Go 代码）
#    source:
#      type: udp
#      decoder: fields
#      fields:
#        throttle: { off: 116, fmt: f32 }
#        brake:    { off: 120, fmt: f32 }
#        speed:    { off: 8,   fmt: f32, scale: 3.6 }
#        gear:     { off: 132, fmt: u8 }
#      derived:
#        slip: "max(abs(slip_fl), abs(slip_fr))"

# 4. 验证数值范围
./bin/ds5d -dry-run -profile dirt-family -trace

# 5. 去掉 -dry-run 正常使用
```

`fmt` 支持 `u8` `s8` `u16` `s16` `u32` `s32` `f32`，可加 `scale` / `bias`（`value = raw*scale + bias`），`derived` 里可以用任意表达式组合字段。

---

## Windows

设备层已经按平台分文件（`internal/device/hidraw_{linux,windows,other}.go`），Windows 侧用 SetupAPI 枚举 HID 接口 + `CreateFile`/`WriteFile`/`ReadFile`，输出报告与 Linux 共用（USB 48 字节 / BT 78 字节 + CRC）。

**状态：能编译、逻辑与 Linux 对齐，但未在 Windows 真机验证。** 首次上机需要重点确认：

1. 蓝牙报告的 `0x31` 布局与 CRC32 位置（`internal/hid/report.go`）
2. 输入报告读取是否需要 OVERLAPPED 才能被正常打断
3. 与 Steam Input 的写冲突（Windows 上 Steam 会同时写震动）

```bash
make windows
bin/ds5d.exe -profile fps -trace
```

---

## 故障排查

| 现象 | 原因 / 处理 |
|---|---|
| `ds5ctl list` 空 | 没插 USB / udev 规则未生效（重新插拔）/ 权限 |
| `device: no DualSense found` 一直重试 | 同上；或 VID/PID 为第三方 DS5 兼容手柄，用 `-hidraw` 指定 |
| 扳机没阻力但有震动 | 选错 hidraw 节点（多接口），`ds5ctl bench -hidraw /dev/hidrawN` 逐个试 |
| 有震动但扳机手感怪异 | 换效果模式：`ds5ctl bench` 对比 8 种；参数语义在不同固件上有差异 |
| 游戏里完全没反应 | 守护进程没跑；`-trace` 看有没有信号 |
| Forza 没数据 | 游戏里 Data Out 没开 / IP 不是本机 / **格式选了 Sled（232 字节，不含油门刹车）** |
| BeamNG 没数据 | Options → Other → OutGauge 未开启 |
| 尘埃/F1 数值乱跳 | 偏移猜错了，用 `ds5ctl sniff` 重新校准（不要用模板里的注释值直接跑） |
| 与 Steam Input 抢震动 | 游戏属性里关闭 Steam Input，或让 Steam 只管输入 |
| 蓝牙下扳机时好时坏 | 换成 USB 线 |

---

## 目录结构

```
cmd/ds5d/                 守护进程（profile → source → rules → sink）
cmd/ds5ctl/               设备/配置/校准工具
cmd/ds5bridge/            Windows 侧共享内存桥（在 Proton 前缀内运行）
internal/
  bridge/                 桥的线格式 + 共享内存布局（ac/acc/acr）
  hid/                    报告构造（USB/BT）、扳机效果库
  device/                 设备抽象 + hidraw(Linux) / HID(Windows) / stub(其它)
  input/                  DualSense 输入报告解析
  signal/                 信号词汇表 + 带时间戳/整形的信号集
  profile/                profile 结构 + 内置 profile（go:embed）
  rules/                  表达式编译与求值、优先级/锁存/衰减
  sink/                   固定 tick 输出、退出回收
  report/                 按传输方式选择报告格式
  source/                 Source 接口 + 注册表
    trigger/              输入驱动（任意游戏）
    udp/                  原生遥测（forza / outgauge / dirt / acshm / fields）
    sim/                  无设备脚本源（开发/测试）
udev/70-dualsense.rules   免 root 访问 hidraw
scripts/launch-bridge.sh  把桥塞进指定 AppID 的 Proton 前缀
tools/bridgesim/          模拟桥输出（无游戏时验证链路）
profiles/                 放你自己的 profile（用 -profiles 指定）
```

---

## 协议参考

- DualSense 输出报告 / 扳机效果：Linux 内核 `drivers/hid/hid-playstation.c`（`dualsense_output_report_common`、`DS_TRIGGER_EFFECT_*`）、`flok/pydualsense`
- Forza Data Out（Sled 232B / Dash 311B）：`Forza-Motorsport-Data-Out` 系列文档
- OutGauge（LFS / BeamNG）：`outgauge_packet` C 结构，92+ 字节
- Codemasters UDP（尘埃/F1）：各版本规格不同，务必先用 `ds5ctl sniff` 核对

---

## Roadmap

- [x] **Assetto Corsa / ACC / AC Rally**：`cmd/ds5bridge`（Proton 前缀内）+ `acshm` 解码器
- [ ] **rFactor 2 / LMU、ETS2 / ATS**：同一套桥，加新的共享内存布局（`internal/bridge/layout.go`）
- [ ] **iRacing**：SDK 内存映射，布局同样走 `internal/bridge`
- [ ] **`source/ffb`**：uinput 虚拟手柄镜像游戏的力反馈 → 「游戏在震多强」变成信号，覆盖无遥测的 FPS/动作游戏
- [ ] **`source/audio`**：PipeWire 频谱包络兜底（枪声/爆炸驱动扳机微颤）
- [ ] **Windows 真机验证**：蓝牙报告、OVERLAPPED 读、Steam Input 冲突
- [ ] **焦点仲裁**：同时开多个 profile，按活动窗口自动切换
- [ ] **`ds5d replay`**：录制/回放信号，做手感回归测试

---

## 发布（GitHub Actions）

`.github/workflows/release.yml`：推 tag 即构建并发布。

```bash
git tag v0.1.0
git push origin v0.1.0
```

- `verify`：gofmt / vet / test / 三平台交叉编译，任一失败则中止
- `build`：矩阵 `linux-amd64`、`linux-arm64`、`windows-amd64`，产物含 `ds5d`、`ds5ctl`、**`ds5bridge.exe`**、`profiles/`、`README`、`LICENSE`、`udev/`、`scripts/`
- 版本号通过 `-X main.version=<tag>` 注入，`ds5d -version` / `ds5ctl version` 可查
- `release`：合并产物 → 生成 `SHA256SUMS` → `gh release create`（已存在则 `--clobber` 覆盖上传）
- 也可以在 Actions 页面手动 `workflow_dispatch`，填一个 tag

---

## License

MIT
