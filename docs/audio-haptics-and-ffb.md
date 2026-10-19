# 音频触感与力反馈镜像：设计与实现

本文记录两个特性从设计到真机验证的全过程：**音频包络触感**（`source/audio` +
`sink/haptics`）和 **力反馈镜像**（`internal/vpad` + `cmd/ds5vpad` +
`source/ffb`），以及过程中踩到的坑和结论。

两部分都不依赖任何游戏遥测：前者听系统音频，后者听游戏发出的标准震动指令。

---

## 目录

- [一、为什么需要这两条通路](#一为什么需要这两条通路)
- [二、音频触感](#二音频触感)
  - [2.1 数据流](#21-数据流)
  - [2.2 为什么非要用音圈](#22-为什么非要用音圈)
  - [2.3 source/audio：把声音变成信号](#23-sourceaudio把声音变成信号)
  - [2.4 sink/haptics：把信号变成 PCM](#24-sinkhaptics把信号变成-pcm)
  - [2.5 关键：HID 报告与音频通道互斥](#25-关键hid-报告与音频通道互斥)
  - [2.6 配置参数](#26-配置参数)
  - [2.7 实测数据](#27-实测数据)
- [三、力反馈镜像](#三力反馈镜像)
  - [3.1 为什么必须造一个虚拟手柄](#31-为什么必须造一个虚拟手柄)
  - [3.2 架构](#32-架构)
  - [3.3 Linux 实现细节](#33-linux-实现细节)
  - [3.4 线协议](#34-线协议)
  - [3.5 已知局限：只能看到 upload，看不到 playback](#35-已知局限只能看到-upload看不到-playback)
  - [3.6 Windows 适配口子](#36-windows-适配口子)
  - [3.7 实测数据](#37-实测数据)
- [四、踩坑与经验](#四踩坑与经验)
  - [4.1 代码层面的坑](#41-代码层面的坑)
  - [4.2 测试方法论的坑](#42-测试方法论的坑)
- [五、验证清单](#五验证清单)
- [六、调参速查](#六调参速查)

---

## 一、为什么需要这两条通路

现有四条信号源各自有明确边界：

| 源 | 能拿到什么 | 拿不到什么 |
|---|---|---|
| `trigger` | 手柄自身按键/扳机行程 | 游戏世界里的任何事件 |
| `udp` | 支持遥测的游戏（Forza/AC/ACC…） | 绝大多数游戏 |
| `audio`（本文） | 任何正在发声的游戏 | 无声事件、精确语义 |
| `ffb`（本文） | 任何发标准震动的游戏 | 音圈级的频谱细节（只有幅度） |

两者互补：

- **`ffb` 精度低但语义准**：游戏说"现在震动 70%"，我们就知道了时机与强度，
  可以顺势给出扳机后坐力。
- **`audio` 语义模糊但精度高**：不知道那是枪声还是引擎，但 30~110 Hz 的包络
  形状是真实的，适合驱动音圈这种能还原波形/频谱的执行器。

---

## 二、音频触感

### 2.1 数据流

```
PipeWire monitor（默认输出的那份混音）
        │  parec --format=float32le --raw
        ▼
source/audio
  滤波器组：bass(20–110Hz) / mid(160–450Hz) / loudness(全带)
  包络：一极点 attack/release（默认 4ms / 90ms）
  归一化：sqrt(RMS) × gain → 0..1
        │  signal.Frame{bass, mid, loudness}
        ▼
规则引擎（YAML）
        │  hid.Frame{HapticL, HapticR}
        ▼
sink/haptics
  合成：55Hz 基频 + 2/3 次谐波 → 48kHz s16le
  写 Ch3/4，Ch1/2 保持静音
        │  pacat --channels=4 --channel-map=...
        ▼
DualSense USB 声卡 → 左右音圈
```

### 2.2 为什么非要用音圈

DualSense 内部是两枚音圈（voice coil），没有传统偏心转子马达。驱动它们有两条路：

| 通道 | 分辨率 | 能表达什么 |
|---|---|---|
| HID 输出报告（`motor_left/right`） | 8 bit 幅度 + 一个调幅频率 | 「嗡嗡嗡」的强弱与快慢 |
| USB 音频 Ch3/4 | 48kHz / 16bit 任意波形 | 频谱、谐波、瞬态包络 |

「路肩的碎、砂石的粗、金属撞击的脆」本质上是**频谱差异**，HID 通道物理上做不出，
只能在音圈上做。这也是本项目早期「路肩震动怎么做都不够细腻」的根本原因。

### 2.3 source/audio：把声音变成信号

**三段滤波器组**

- `bass` 20–110 Hz：引擎低频、爆炸、路面轰鸣 → 主要驱动音圈
- `mid` 160–450 Hz：枪声、碎石、金属撞击的「脆」→ 补充纹理
- `loudness` 全带 RMS：用于判断「有没有大事发生」

每段都是 **3 级 biquad 级联（36 dB/oct）**。单级二阶只有 12 dB/oct，实测会让
50 Hz 引擎声大量漏进 160–450 Hz 的 `mid` 通道，导致频段失去意义。

**包络与归一化**

```go
// 一极点 attack/release：上升快、回落慢，符合震动体感
y = x + (y - x) * (1-c)^n     // c 为 attack 或 release 系数
env = clamp(sqrt(rms) * gain, 0, 1)
```

用 `sqrt` 而不是线性：小音量（远处引擎、草地震动）也能被感知，同时给爆炸留出余量。

**空闲检测（关键）**

PipeWire 在 sink 空闲时会**暂停推送**，`read()` 直接阻塞。若按阻塞读实现，
源会永远停在上一次的响亮值，下游看到的是"冻结的 bass=1.0"。

实现上用 `SetReadDeadline`（80ms，略大于一个 256 帧的块）：
超时就给包络喂一个静音块并主动发一帧 `0`，让效果自然衰减。

**块长与超时的关系**

块必须明显小于超时，否则每次读都超时、数据被丢弃（这个坑实际踩过：
最初块长 1024 帧 = 125ms > 80ms 超时，结果永远读到静音）。

### 2.4 sink/haptics：把信号变成 PCM

**波形合成**

```go
wave = sin(φ) + 0.35*sin(2φ) + 0.12*sin(3φ)   // 归一化到 [-1,1]
φ += 2π * carrierHz / 48000                    // 默认 55 Hz
```

纯正弦听起来像测试音；叠加 2/3 次谐波才有「质感」。载波频率可通过
`haptics.carrier_hz` 调整（低频更闷、高频更麻）。

**幅度平滑**

规则切换是突变的（例如路肩规则突然接管），直接写 PCM 会有爆音。
用一极点平滑 `smooth += 0.25 * (want - smooth)`（每采样），过渡约 4ms。

**时间排程**

按真实时间计算「应该已经写了多少帧」，而不是每次 tick 固定写一块 —— 否则
调度抖动会让音频流逐渐落后于实时，最终 underrun。

**声道映射（关键）**

```bash
pacat --playback --device=<sink> --rate=48000 --channels=4 \
      --channel-map=front-left,front-right,rear-left,rear-right \
      --format=s16le --raw
```

不指定 `--channel-map` 时 PipeWire 默认给 4 声道选
`front-left,front-center,front-right,rear-center`，然后**按位置重混**：
我们写在 Ch3/4 的音圈波形被当成「前右 / 后中」，混进耳机通路，
音圈一个采样都收不到。

指定成与 sink 完全一致的布局后，4 个通道才 1:1 直通硬件。

### 2.5 关键：HID 报告与音频通道互斥

这是整个特性里最难发现的一个问题，也是「灌了 PCM 但音圈毫无反应」的真正原因。

`valid_flag0` 是**应用掩码**，不是状态：

```c
#define DS_OUTPUT_VALID_FLAG0_COMPATIBLE_VIBRATION  BIT(0)  /* 用 motor 字节驱动 */
#define DS_OUTPUT_VALID_FLAG0_HAPTICS_SELECT        BIT(1)  /* 交给 HID 震动引擎 */
```

只要报告里声明了这两位，固件就认为「执行器由 HID 的 motor_left/right 驱动」。
ds5d 每 8ms 都写 `flags0 = 0x0F` 而 motor 恒为 0 → 执行器被锁在 HID 引擎上、
幅度 0，**音频流送来的波形被完全忽略**。

修复：

```go
flags0 := byte(0x04 | 0x08)          // 扳机效果块，始终声明
if f.RumbleL != 0 || f.RumbleR != 0 {
    flags0 |= 0x01 | 0x02            // 只有真的有震动时才接管执行器
}
```

空闲时执行器「无人认领」，音频流才能驱动音圈。回归测试
`TestVibrationFlagsOnlyWhenRumbling` 守住三条约束：

1. 空闲帧不得声明 bit0/bit1
2. 必须保留扳机位（bit2/bit3）
3. bit4–7（音量/audio_control）必须恒为 0

**bit4–7 为什么必须清零**

`0xFF` 会「启用但不填充」headphone/speaker/microphone 音量与 `audio_control`，
这些字节恒为 0，等于把 DAC 音量设成 0。报告是持久状态，ds5d 退出后依然静音 ——
表现为「连手柄扬声器都没声音」。实测：`0xFF` 下往 Ch1/2 灌 1kHz 完全无声，
改成 `0x0F` 后立刻正常。

### 2.6 配置参数

```yaml
source:
  type: audio
  target: auto          # PipeWire sink/monitor 名，或 auto（默认输出）
  rate: 24000           # 采集率（Nyquist 需高于最高频段）
  channels: 2
  format: float32le     # parec 的格式名
  rate_hz: 60           # 信号发射频率
  gain: 3.0             # 包络线性增益
  attack_ms: 4
  release_ms: 90
  bass_hz: "20,110"
  mid_hz: "160,450"
  command: "parec --device={target} --rate={rate} --channels={channels} --format={format} --raw"

haptics:
  target: auto          # 也可写死 sink 名
  carrier_hz: 55
  gain: 0.55            # 0..255 规则输出 → PCM 幅度
  command: "pacat --playback --device={target} --rate={rate} --channels={channels} --channel-map=front-left,front-right,rear-left,rear-right --format=s16le --raw"
```

### 2.7 实测数据

（PipeWire 1.x，USB，fw `0x0110002a`）

| 输入 | bass | mid | loudness | 现象 |
|---|---|---|---|---|
| 静音 | 0.000 | 0.000 | 0.000 | 无震动 |
| 50 Hz 正弦（默认输出） | 0.47 | 0.000 | 0.52 | **音圈持续震动** |
| 1 kHz 正弦 | 0.000 | 0.004 | 0.520 | 音圈无输出（正确） |
| 停止播放 1.5s 后 | 0.018 | 0.002 | 0.016 | 正常衰减 |

频段隔离验证：50 Hz 完全不进 `mid`，1 kHz 完全不进 `bass`（36 dB/oct 级联的效果）。

---

## 三、力反馈镜像

### 3.1 为什么必须造一个虚拟手柄

游戏只会把手柄震动发给**它正在对话的那个设备**。只要 Steam Input 占着物理手柄：

- 游戏看到的是 Steam 造的虚拟 Xbox 手柄
- 震动指令由 Steam 翻译后直接写物理手柄的 HID 输出
- 外部程序（ds5d）既看不到指令，也抢不到输出

`ds5vpad` 反过来做：自己建虚拟手柄、自己独占物理手柄、把游戏震动转成 UDP。

### 3.2 架构

```
物理 DualSense (evdev)
      │  EVIOCGRAB 独占
      ▼
ds5vpad ── 转发按键/摇杆 ──► 虚拟 Xbox 360 手柄 (uinput) ──► 游戏
      │
      └── 捕获 FF_RUMBLE ── UDP 127.0.0.1:9998 ──► source/ffb ──► ds5d
                                                                  │
                                                          马达 / 音圈 / 扳机
```

好处不止「能捕获震动」：

- Steam Input 可以彻底关掉，不再有第二个写 HID 的进程
- 游戏仍然认手柄（虚拟设备是标准 Xbox 身份）
- 闲置时执行器归音频流所有，音圈触感才有机会工作（见 2.5）

### 3.3 Linux 实现细节

**创建虚拟设备**

`/dev/uinput` + 传统 `uinput_user_dev` 结构体：

- 身份 `045e:028e`（有线 Xbox 360 手柄），名字 `ds5vpad virtual pad`
- SDL / Wine 内建映射直接认这个身份，不需要额外 udev/规则
- 能力：`EV_KEY`（按键）、`EV_ABS`（摇杆/扳机/十字键）、`EV_FF`（只声明
  `FF_RUMBLE`，其它类型我们无法合成，声明了反而误导游戏）
- `ff_effects_max = 16`

**独占物理设备**

```go
unix.IoctlSetInt(phys, 0x40044590 /* EVIOCGRAB */, 1)
```

`x/sys/unix` 没有导出 `EVIOCGRAB`，按 `_IOW('E', 0x90, int)` 手算为 `0x40044590`。

非 root 权限问题实际不存在：uinput 生成的设备会自动获得 `uaccess` ACL，
测试确认当前用户可读写。

**键位映射**

DualSense 的 evdev 键码与 Xbox 360 不同，需要重写：

| 物理 | 虚拟 |
|---|---|
| `BTN_SOUTH`（×） | `BTN_A` |
| `BTN_EAST`（○） | `BTN_B` |
| `BTN_WEST`（□） | `BTN_X` |
| `BTN_NORTH`（△） | `BTN_Y` |
| `BTN_TL2/TR2` | 丢弃（扳机走 `ABS_Z/ABS_RZ`） |

摇杆/扳机/十字键的轴编号两边一致，直接透传。

**捕获力反馈**

uinput 把 FF 请求作为 `EV_UINPUT` 事件投递到 uinput fd 上：

```
EV_UINPUT + code=UI_FF_UPLOAD + value=request_id
  → ioctl(UI_BEGIN_FF_UPLOAD)  取回 struct uinput_ff_upload
  → 解析 effect.u.rumble.strong/weak_magnitude
  → ioctl(UI_END_FF_UPLOAD)    回填 retval=0 完成
```

这块必须跑在**独立 goroutine**：内核会阻塞调用者直到 userspace 回填结果，
放在事件泵里会把按键转发一起卡死。

### 3.4 线协议

12 字节定长，平台无关（未来 Windows 后端只要发同样的包）：

```
偏移  字段   类型     说明
0     magic  uint32   "FFB1" = 0x31424646
4     left   uint8    低频马达 0..255
5     right  uint8    高频马达 0..255
6-7   _      uint8    保留
8     seq    uint32   序号（可选，用于诊断丢包）
```

`source/ffb` 把它转成 `ffb_left` / `ffb_right`（0..1）两个信号。

**过期策略**：ds5vpad 侧 500ms 未刷新就把幅度归零，避免暂停或切关卡后马达
一直转。

### 3.5 已知局限：只能看到 upload，看不到 playback

uinput 只暴露 `upload_effect` / `erase_effect` 两个回调，**没有 playback 回调**
（`/usr/include/linux/uinput.h` 里只有 `UI_FF_UPLOAD` / `UI_FF_ERASE`，没有 play 事件）。

因此实现采用「以最近一次上传的幅度为准」：

- Proton/Wine 的 evdev FF 后端在震动强度变化时会重新上传 effect → 跟踪准确
- 若某游戏「上传一次、之后只写 `EV_FF` play/stop」→ 只能拿到首次幅度

想彻底解决需要内核侧回调，或者走 Windows 的 ViGEm（见 3.6）。

### 3.6 Windows 适配口子

`internal/vpad` 已按平台分层：

| 文件 | 内容 |
|---|---|
| `vpad.go` | 平台无关的 `Config` / `Pad` 接口 / `RumbleSink` |
| `vpad_linux.go` | uinput 实现 |
| `vpad_windows.go` | 返回 `ErrUnsupported`，注释里写明 ViGEm 方案 |
| `vpad_other.go` | 其它平台占位 |

Windows 侧落地路径：

1. 用 ViGEmBus（或新版 Xbox Gamepad 虚拟设备 API）创建虚拟手柄
2. 用独占 HID 句柄读物理 DualSense 并转发
3. 实现 ViGEm 的 rumble 回调 —— **它直接给播放事件**，没有 3.5 的限制
4. 继续通过同一个 `RumbleSink` 发布，上层代码零改动

音频侧同理：`haptics.command` 可整体替换成 WASAPI 侧命令（如 `ffplay`/自研工具），
`source/audio` 的 `command` 也可换成 Windows 采集工具。

### 3.7 实测数据

链路自测（`tools/ffclient`，不启动游戏）：

```
上传 rumble effect: strong=40000 weak=20000
ds5vpad: vpad: ff control events: 24 bytes / 72 bytes
ds5d:    signals: ffb_left=0.612 ffb_right=0.306
```

`40000/65535 = 0.610`、`20000/65535 = 0.305` —— 幅度精确还原。
停止后 500ms 内归零。

---

## 四、踩坑与经验

### 4.1 代码层面的坑

| # | 问题 | 表现 | 根因 | 修法 |
|---|---|---|---|---|
| 1 | `pw-record/pw-cat` 的 `--target` 被静默忽略 | 录到的永远是默认节点，看着像"永久静音" | 该 PipeWire 版本未处理该参数 | 改用 `parec/pacat`（实测 peak 0.001 → 0.61） |
| 2 | 空闲 sink 让 `ReadFull` 永久阻塞 | 源停止发帧，下游读到冻结的 `bass=1.0` | PipeWire 空闲时不推送 | 读超时 + 空闲主动发衰减帧 |
| 3 | 包络每块只 `step` 一次 | 90ms 释放被拉长到约 23 秒 | 系数按每采样算，块长 256 | 按块长解析衰减 `stepN` |
| 4 | 块长 > 读超时 | 永远读到静音 | 1024 帧 = 125ms > 80ms | 块长降到 256 帧 |
| 5 | `pacat` 未指定 `--channel-map` | 音圈完全收不到数据 | 默认 4 声道布局会按位置重混 | 显式对齐 sink 的布局 |
| 6 | `valid_flag0=0xFF` | 连手柄扬声器都没声音 | 启用但未填充音量字段 → DAC 音量 0，且报告持久 | 只声明真正用到的位 |
| 7 | 空闲仍声明 `VIBRATION/HAPTICS_SELECT` | 灌 PCM 音圈毫无反应 | 执行器被锁在 HID 引擎（幅度 0） | 只有真的震动时才声明 |
| 8 | `pactl` 输出整行被当成 sink 名 | 播放到错误目标 | 按 `\t` 切分时用了 `SplitN(…,2)` | 全切分后取第二列 |
| 9 | 单级 biquad 频段泄漏 | 50Hz 大量漏进 mid | 12 dB/oct 不够 | 3 级级联（36 dB/oct） |

第 6、7 条是同一个原理的两面：**`valid_flag0` 是应用掩码，声明什么固件就只看什么**。
写 HID 报告的第三方工具几乎都会踩这两个。

### 4.2 测试方法论的坑

这部分比代码坑更值得记：

**1. 标记信号会污染测试**

第一版 `hidprobe` 在每个变体前用「N 次震动」做编号。结果用户反馈「都震了」——
但那是标记震动，不是音圈。加 `-no-marker` 复测才得到真实结果。

> **原则**：验证执行器 A 时，绝不能用同一个执行器 B 发信号做标记。用日志、
> 灯条、或干脆让测试者看屏幕。

**2. 抽样日志会掩盖尖峰**

`ds5d -trace` 每 500ms 采样一次，路肩只有 100~300ms，采样到的 `accel_g=0.005`
完全不能代表真实峰值（帧级是 1.2~1.9）。也是因为这个，最初误判「信号太弱、
规则触发不了」。

> **原则**：脉冲类信号一定要在**发送端按帧记录**（`ds5bridge -events`），
> 不要在消费端按时间采样。

**3. 「没反应」可能发生在任意一层**

同一条链路上有 6 个环节可能出问题（采集命令 → 声道映射 → 固件使能位 →
规则 → PCM 合成 → 执行器）。当时先后怀疑过阈值、偏移、Steam 抢占、声道顺序、
报告长度，都不对。

> **原则**：先做**能区分层的对照实验**，而不是逐层调参。这次真正的分水岭是
> 「往 Ch1/2 灌 1kHz 听声音」—— 一声立刻把「声卡不通」和「只有音圈不通」分开。

**4. 声音和执行器是同一个 DAC**

一开始认为「喇叭有声音 = 音频通路 OK」，但音圈与喇叭共用 DAC，而 DAC 的音量
由 HID 报告控制。所以「喇叭有声」在某个配置下也可能是假象（或反之）。

> **原则**：共享资源的两个消费者，必须**同时验证**，不能用一个推断另一个。

**5. 报告状态是持久的**

HID 输出报告不是"指令"而是"状态"。ds5d 退出后，最后一次「音量 0」的报告
仍然生效，裸 `pacat` 也听不到声音 —— 当时差点把问题归因成"设备没被驱动"。

> **原则**：排查前先确认「当前生效的状态是谁写进去的」，必要时物理重插复位。

**6. 单测覆盖不了硬件语义**

`abs()` 被信号名遮蔽、`sqrt` 未绑定、`valid_flag0` 掩码语义 —— 这些单测全绿，
但只有真机能暴露。反过来，DSP 的频段隔离、包络衰减、PCM 声道独立性这些
纯计算部分，单测非常有效且成本低。

> **原则**：纯算法用单测守住；**协议/帧格式/设备交互必须上真机**，并且要有
> 可复现的最小工具（`ffclient` / `hidprobe` / `bridgesim`）。

---

## 五、验证清单

音频触感（不需要游戏）：

```bash
# 1) 信号层：播 50Hz，看 bass 上来
./bin/ds5d -profile audio-haptics -dry-run -trace
paplay /tmp/t50.wav            # 或用手机外放低频音乐
#   期望：bass 0 → 0.4+，停止后衰减回 0

# 2) 执行器层：只看音圈（probe profile 里没有 HID 马达规则）
./bin/ds5d -profile /tmp/coil-probe.yaml -rate 125
paplay /tmp/t50.wav
#   期望：音圈震动；1kHz 对照文件应无反应
```

力反馈镜像（不需要游戏）：

```bash
./bin/ds5vpad &                                   # 建虚拟手柄 + 独占物理手柄
./bin/ds5d -profile ffb-recoil -rate 125 -trace   # 消费 9998
./bin/ffclient -dev /dev/input/eventN             # N = ds5vpad 打印的节点
#   期望：ffb_left≈0.61 / ffb_right≈0.31，停止后归零
```

协议层排查：

```bash
./bin/hidprobe -dev /dev/hidrawN -only 2 -no-marker -hold 6s
#   -no-marker 必须加，否则编号震动会掩盖真实的执行器反馈
```

---

## 六、调参速查

| 想达到 | 改哪里 |
|---|---|
| 音圈更「闷」/更「麻」 | `haptics.carrier_hz`：35（闷）↔ 90（麻） |
| 音圈整体更弱 | `haptics.gain` 0.55 → 0.35 |
| 音频触感更灵敏 | `source.audio.gain` 3.0 → 4.5，或 `release_ms` 90 → 150 |
| 只想要音乐的低音，不想要人声 | 收窄 `bass_hz` 到 `"30,90"` |
| 撞击不要那么刺耳 | `coil-impact` 的 `180 + 30 *` 降到 `120 + 20 *` |
| 路肩更细腻但不更响 | 提高 `coil-kerb-*` 的 `carrier_hz`，降低 `value` 基值 |
| 后坐力更强 | `ffb-recoil.yaml` 里 `recoil` 规则的 `amp_a/amp_b` |

> 内置 profile 编译进二进制，改完要 `make build`；想免编译就复制到 `profiles/`
> 并用 `-profiles ./profiles -profile <名字>` 加载。
