# 理查德·伯恩斯拉力赛（RBR / RallySimFans）

RBR 是 2004 年的 DirectInput 游戏，模拟轴/键位全走 DInput，没有 XInput、也没法直接吃
DualSense 的自适应扳机。所以在 Linux 上要同时解决两件事：

1. **输出侧**：把 NGP 插件的 UDP 遥测变成扳机阻力 / 震动（`profile rbr`）
2. **输入侧**：让游戏看到一个它认得的 DirectInput 手柄（`ds5vpad`）

```
RBR (Proton) ──NGP UDP 6776──► ds5d -profile rbr ──HID──► DS5 扳机/马达/灯条
     ▲
     └── ds5vpad -proto dinput8（uinput 虚拟罗技 DInput8 手柄，独占物理 DS5）◄── 物理 DS5 evdev
```

---

## 1. 游戏侧要打开的开关

| 项 | 位置 | 值 |
|---|---|---|
| UDP 遥测 | `RichardBurnsRally.ini` → `[NGP]` | `udpTelemetry=1` |
| 端点 | 同上 | `udpTelemetryEndpoints=127.0.0.1:6776` |
| 发送间隔 | 同上 | `telemetryTics=5`（每 5 个物理 tick 一包） |

游戏内也有等价开关：`Plugins → NGP` 按 **U** 切换，值会写回 ini。

**坑**：RSF 启动器每次启动都会用它自己存的 `rsf_launcher/Configs/default.ini`
里的 `NGP.udpTelemetry` 覆盖 `RichardBurnsRally.ini`。所以要么在启动器 UI 里打开
UDP telemetry，要么直接改启动器那份配置：

```ini
# rsf_launcher/Configs/default.ini
NGP.udpTelemetry=1
```

遥测**只在 GAME 模式**（进赛道）发送，菜单和回放里没有数据。

---

## 2. 包结构（`profile rbr` 的依据）

数据包来源于游戏自带官方头文件 `Plugins/NGP/sdk/rbr.telemetry.data.TelemetryData.h`，遵循 `#pragma pack(1)` 字节对齐，**定长 664 字节**：

| 数据段名称 | 偏移区间 (字节) | 占用大小 | 包含字段与数据类型说明 |
|---|---|---|---|
| `totalSteps` | `0 .. 3` | 4 字节 | `uint32_t` 仿真物理步数计数器 |
| `Stage` | `4 .. 23` | 20 字节 | `int32_t` 赛段状态 + 4 个 `float32` (赛段进度 0..1、比赛计时、剩余距离等) |
| `Control` | `24 .. 55` | 32 字节 | 5 个 `float32` (方向盘、油门、脚刹、手刹、离合) + 1 个 `int32_t` 挡位 + 2 个 `float32` 踏板压力 |
| `Car` | `56 .. 663` | 608 字节 | 车身动力学数据（车速、三向加速度、转速）及 LF/RF/LB/RB 四轮悬挂阻尼与胎温（每轮独立 128 字节） |
| **完整包体** | **0 .. 663** | **664 字节** | 严格定长连续二进制流 |

`Car` 里对触感有用的字段：

| 字段 | off | 说明 |
|---|---|---|
| speed | 60 | kph |
| accel surge / sway / heave | 112 / 116 / 120 | 纵向 / 横向 / 垂直加速度 |
| rpm | 136 | |
| damper pistonVelocity LF / RF / LB / RB | 180 / 308 / 436 / 564 | 悬挂阻尼速度 = 路面纹理 |
| brake disk temperature LF / RF | 188 / 316 | |
| tire treadTemperature LF / RF | 208 / 336 | |

`Control` 段（24 起）：`steering 24, throttle 28, brake 32, handbrake 36, clutch 40,
gear 44, footbrakePressure 48, handbrakePressure 52`。
**每个 `Suspension` 结构正好 128 字节**（5 f32 + i32 + Damper + Wheel），所以四个轮的
偏移是 152 / 280 / 408 / 536，阻尼速度在各自基址 +28。

重算方式（改 NGP 版本后必须重算）：按头文件顺序用 `#pragma pack(1)` 累加即可，
校验点是 `sizeof(TelemetryData) == 664`。

---

## 3. `profile rbr` 触感与模拟特性

| 体验维度 | 依托物理量 | 输出执行器与特性 |
|---|---|---|
| **L2 无助力液压刹车** | `brake` (脚刹踏板) | L2 `rigid`：前段微小空行程，中段线性阻尼陡增，后段刚性硬墙（力矩 235~255） |
| **刹车抱死警示** | `brake > 0.65` & `slide_speed > 3` | 左马达 18Hz 方波脉冲：高速重刹导致四轮滑移时提示车手释放刹车防推头 |
| **R2 柔性渐进油门** | `throttle` (油门踏板) | R2 `rigid`：前段轻盈易于细致控油，后段饱满回弹，长赛段手指不疲劳 |
| **红线换挡转速回馈** | `rpm > 7100` | 右马达 60Hz 正弦微颤：逼近 8000rpm 红线时给予细腻高频震感，提示升挡 |
| **3D 立体声砂石颗粒** | `damper_front` / `damper_rear` | 左音圈 52Hz + 右音圈 40Hz（形成 12Hz 自然声学拍频，呈现 3D 砂石敲击底盘颗粒） |
| **底盘起伏与连续路面** | `accel_vert` & `speed > 8` | 右马达 54Hz 细密微颤 + 左大马达 35Hz 车身沉降，左右拍频呈现连续路况 |
| **横向过弯离心支撑** | `accel_lat` (横向 G) | 右马达离心支撑载荷（`smoothstep(2.5, 14.0, absf(accel_lat))`） |
| **斯堪的纳维亚钟摆甩尾** | `accel_yaw` (横摆角加速度) | 右马达 32Hz 方波瞬态脉冲：钟摆（Scandinavian Flick）车尾骤然甩出时警示反打 |
| **四轮砂石侧滑磨削** | `slide_speed` (侧滑绝对速度) | 左马达 24Hz 连续滑移磨削感（`smoothstep(2.0, 10.0, slide_speed)`） |
| **芬兰飞坡着陆防触底** | `jolt > 0.45` \|\| `accel_vert > 12` | 左右马达 16Hz/22Hz 重锤（`hold 120ms`）+ 音圈爆发（255 满幅），真实还原悬挂触底防撞垫 |
| **手刹抱死后轮** | `handbrake > 0.35` | 右马达 14Hz 急促方波脉冲：清晰确认后轮锁死进入发卡弯旋转 |
| **交互式仪表灯条** | `brake`, `throttle`, `handbrake`, `rpm` | 刹车红（拉手刹叠加琥珀橙）、油门翠绿、接近 8000 转红线电光蓝换挡提示 |

所有 `smoothstep` 阈值（0.02 / 0.35 / 0.45 / 1.2、`accel_lat` 的 3 / 18）都只是起点，
`damper_*` 的物理单位取决于 NGP 版本，按手感改。

### 写 profile 时踩到的两个坑（通用，不只是 RBR）

1. **`derived` 的编译环境里只有字段本身**，没有 `clamp` / `absf`。
   绝对值要手写 `max(x, -x)`，聚合成 `max(max(a,b), max(c,d))`。
2. **`derived` 之间不能互相引用**。解码器按 map 顺序编译 derived，顺序不确定，
   `jolt: "max(damper_front, damper_rear)"` 有一半概率报 `unknown name damper_front`。
   要么从原始字段展开，要么等解码器做拓扑排序。
3. 自定义信号名必须在 `signals:` 里声明；标准词表见 `internal/signal/names.go`
   （注意是 **`steer`** 不是 `steering`）。

### 验证（不开游戏）

```bash
# 合成一个 664 字节包，throttle=0.7 brake=0.15 speed=95 rpm=6300
python3 tools/…                      # 或任意发包脚本
./bin/ds5d -dry-run -profile rbr -trace
# 期望：signals: throttle=0.700 brake=0.150 …；frame l2=rigid[… 55 …] r2=rigid[… 97 …]

# 真机抓包
./bin/ds5ctl sniff -listen 0.0.0.0:6776
```

---

## 4. 输入侧：`ds5vpad -proto dinput8`（原生 DirectInput 8 架构与实战避坑）

RBR 是纯 DirectInput 8 游戏，在 2004 年没有 Xbox 360 手柄的概念，它要求**所有踏板和摇杆都是独立的 16 位满行程轴**：
- **摇杆**：`-32768..32767`，居中为 0。
- **踏板（L2 刹车、R2 油门）**：作为独立模拟踏板，未按时为 `-32768`，踩到底为 `32767`（全行程高达 65535）。
- **按键**：标准 Linux `BTN_GAMEPAD` 规范按键 1..10。

`-proto dinput8` 专门为此而设计（基于罗技 Dual Action `046d:c216` 协议）：
- `ABS_X` / `ABS_Y`：左摇杆（左右/上下），`-32768..32767`
- `ABS_RX` / `ABS_RY`：右摇杆（左右/上下），`-32768..32767`
- `ABS_Z`（L2 刹车踏板）：独立轴，`-32768..32767`
- `ABS_RZ`（R2 油门踏板）：独立轴，`-32768..32767`
- 按钮：`BTN_SOUTH` 到 `BTN_THUMBR`，直接映射为 DirectInput Button 1..10

这样在 RBR 的 `Controller Setup` 界面：
- **`Steering`**：按回车 → 拨动左摇杆左右 → 识别为 **X 轴**
- **`Throttle`**：按回车 → 踩下 **R2** → 识别为 **RZ 轴**
- **`Footbrake`**：按回车 → 踩下 **L2** → 识别为 **Z 轴**
- **`Gear Up`**：按回车 → 按 **× 键** → 识别为 **Button 1**
- **`Gear Down`**：按回车 → 按 **口 键** → 识别为 **Button 3**
- **`Handbrake`**：按回车 → 按 **○ 键** → 识别为 **Button 2**
- **`Accelerate and Brake`（合并轴）**：按 **DEL** 清空（必须清空，否则刹车常驻 50%）

---

### 实战踩坑与终极排错经验

#### 1. 踩踏板“松开显示 100% 满载，踩下反而归零”（轴反向）
- **根因**：DualSense 物理扳机松开时是 `-32768`，踩到底为 `+32767`。而 2004 年 RBR 默认沿用了老式航空节气门定义，把物理最小值判断为“油门踩死”，把最大值判断为“怠速”。
- **解决**：在 `input.ini` 与 `rsf_launcher/Configs/default.ini` 中设置轴反转：
  ```ini
  InvertThrottleAxis = true
  InvertBrakeAxis = true
  ```
  或者在游戏内 `Options -> Controls -> Filter / Customise Controls` 里把 `Throttle` 和 `Footbrake` 后面的 `Invert` 切为 `ON` 即可。

#### 2. “刹车处于一半进程（Z axis）且卡住”（合并轴冲突）
- **根因**：游戏默认勾选了老式单轴合并油门刹车（`Accelerate and Brake`），由于居中值为 0，游戏默认折算为 50% 刹车。
- **解决**：在 `Controller Setup` 选中 `Accelerate and Brake`，按键盘 **Delete** 键清空。

#### 3. “手柄按键按了完全没反应，只有摇杆能识别”
- **根因**：Wine 的 `winebus.sys` 驱动层对通用 Gamepad 严格只认 `0x130 (BTN_SOUTH)`..`0x13e (BTN_THUMBR)`。如果底层虚拟手柄驱动把按键发成了老式飞行摇杆段的 `BTN_TRIGGER (0x120)`，Wine 会当成非手柄摇杆忽略，导致游戏在 DirectInput 下接收到 0 个按键。
- **解决**：`ds5vpad -proto dinput8` 采用标准 `BTN_GAMEPAD` 协议直通，手柄按键即可完美映射为 Button 1..10。

#### 4. “设置好了键位，下次进游戏按键设置又空了”
- **根因一（只读权限）**：从 7z 包解压出来的 `SavedGames/pfsam.acm` 往往携带 Windows 只读属性（在 Linux NTFS 上表现为 `0444`），导致游戏退回菜单时写盘失败；
- **根因二（强退丢数据）**：RBR 只有在按 **Esc** 返回上一级菜单并正常退出游戏时，才会将内存中的按键表固化保存到 `pfsam.acm`。若游戏卡死使用 `kill -9` 强杀进程，内存中的修改来不及写盘；
- **解决**：
  ```bash
  chmod 666 "SavedGames/pfsam.acm"
  ```
  在游戏内按要求配好后，**按 Esc 键正常返回上一级菜单保存一次**，即可永久固化，以后重启游戏再也不需要重复设置！

---

## 5. 退出后 Steam 仍显示"运行中"

现象：退出游戏后启动器窗口关不掉、Steam 一直显示运行中，并弹 KDE 崩溃上报（DrKonqi）。

根因：**wineserver 自己 SIGSEGV**（游戏退出时）：

```
systemd-coredump: Process (wineserver) dumped core.
  #0 close_handle / req_close_handle   (wineserver + 0x2f593)
  #1 call_req_handler
  #2 thread_poll_event
```

在 Proton-CachyOS 11 和 GE-Proton10-34 上都复现过（栈相同），所以换 Proton 没用。
wineserver 一死，`explorer.exe / services.exe / rpcss.exe / plugplay.exe / tabtip.exe`
变孤儿，启动器进程收不到退出响应 → Steam 卡片一直"运行中"。

**待验证的规避**：给该快捷方式加启动选项 `WINE_DISABLE_NTSYNC=1 %command%`。
一次对照实验里关掉 ntsync 后 wineserver 未再崩溃（coredump 计数未增长），
但样本量不足，需要多退几次游戏确认。

临时清理（只杀该前缀的进程，不影响其它 Proton 游戏）：

```bash
~/.local/bin/rsf-kill.sh    # 按 STEAM_COMPAT_DATA_PATH 精确匹配 appid 3039562861
```

---

## 6. 游戏侧启动错误速查

| 现象 | 原因 | 处理 |
|---|---|---|
| 弹 `Invalid fingerprint` 后退出，日志 `Error in PlatformInit` | 根目录缺 `trackinfo.dat`（RBR 会 fopen 它并校验包内指纹） | 从 RSF 安装包的 `rbr_install.7z.001` 里解出原版文件（`-aos` 不覆盖 RSF 改过的 `physics.rbz` / `RichardBurnsRally_SSE.exe`） |
| 启动器提示缺 `.NET 10 Desktop x86` | 前缀里没有 .NET | `protontricks-launch --appid 3039562861 windowsdesktop-runtime-10.0.x-win-x86.exe /install /quiet` |
| 键位/轴反向设置不生效 | NTFS 挂载大小写敏感，游戏读 `input.ini`、启动器写 `Input.ini` | 两个名字硬链接成同一 inode（`ln input.ini Input.ini`） |
| 绑定保存不上 | `SavedGames/pf*.acm` 带只读属性 | `chmod u+w` 后重启游戏 |
| `steam://rungameid/<appid>` 无效 | 非 Steam 快捷方式要 64 位 gameID | 用 `protontricks-launch` 直接拉起，或在 Steam UI 里点 |

---

## 7. 命令速查

```bash
# 输入侧：虚拟手柄（DirectInput 8 原生踏板与按键）+ 独占物理 DS5
./bin/ds5vpad -proto dinput8 &

# 输出侧：消费 NGP 遥测
./bin/ds5d -profile rbr -trace &

# 不开游戏验证 profile
./bin/ds5d -dry-run -profile rbr -trace

# 收不到数据先确认包
./bin/ds5ctl sniff -listen 0.0.0.0:6776

# 退出后清理卡死的 wine 进程
~/.local/bin/rsf-kill.sh
```

注意：RBR **只在进程启动时枚举一次手柄**，所以顺序必须是
「先 `ds5vpad` → 再启动游戏」，否则改了手柄等于没改。
