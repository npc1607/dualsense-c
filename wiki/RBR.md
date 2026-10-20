# 理查德·伯恩斯拉力赛（RBR / RallySimFans）

RBR 是 2004 年的 DirectInput 游戏，模拟轴/键位全走 DInput，没有 XInput、也没法直接吃
DualSense 的自适应扳机。所以在 Linux 上要同时解决两件事：

1. **输出侧**：把 NGP 插件的 UDP 遥测变成扳机阻力 / 震动（`profile rbr`）
2. **输入侧**：让游戏看到一个它认得的 DirectInput 手柄（`ds5vpad`）

```
RBR (Proton) ──NGP UDP 6776──► ds5d -profile rbr ──HID──► DS5 扳机/马达/灯条
     ▲
     └── ds5vpad（uinput 虚拟 Xbox 手柄，独占物理 DS5）◄── 物理 DS5 evdev
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

来源：游戏自带官方头文件
`Plugins/NGP/sdk/rbr.telemetry.data.TelemetryData.h`，`#pragma pack(1)`，**定长 664 字节**：

```
u32 totalSteps                     0
Stage   (i32 + 4×f32)              4 .. 23
Control (5×f32 + i32 + 2×f32)     24 .. 55
Car     (608 字节)                 56 .. 663
```

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

## 3. `profile rbr` 做了什么

| 信号 | 输出 |
|---|---|
| `brake` | L2 `rigid`，`force = 20 + 235×brake` |
| `throttle` | R2 `rigid`，后段额外加硬（模拟踏板到底） |
| `damper_front` / `damper_rear` | 左 / 右音圈，sine 调制 → 路面纹理 |
| `jolt`（四轮阻尼速度最大值） | 左右马达，`hold 80ms` → 落地/撞石 |
| `accel_lat` | 右马达轻震 → 推头/甩尾 |
| `handbrake` | 右马达 12Hz 脉冲 |
| 灯条 | 刹车红 / 油门绿 / 近红线加蓝 |

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

## 4. 输入侧：`ds5vpad -dinput`（经典 DirectInput 独立踏板模式）

RBR 是纯 DirectInput 8 游戏，在 2004 年没有 Xbox 360 手柄的概念，它要求**所有踏板和摇杆都是独立的 16 位满行程轴**：
- **摇杆**：`-32768..32767`，居中为 0。
- **踏板（L2 刹车、R2 油门）**：作为独立模拟踏板，未按时为 `-32768`，踩到底为 `32767`（行程高达 65535）。

`-dinput` 专门为此而设计：
- `ABS_X` / `ABS_Y`：左摇杆（左右/上下），`-32768..32767`
- `ABS_RX` / `ABS_RY`：右摇杆（左右/上下），`-32768..32767`
- `ABS_Z`（L2 刹车踏板）：独立轴，`-32768..32767`
- `ABS_RZ`（R2 油门踏板）：独立轴，`-32768..32767`

这样在 RBR 的 `Controller Setup` 界面：
- **`Steering`**：按回车 → 拨动左摇杆左右 → 识别为 **X 轴**
- **`Throttle`**：按回车 → 踩下 **R2** → 识别为 **RZ 轴**
- **`Footbrake`**：按回车 → 踩下 **L2** → 识别为 **Z 轴**
- **`Accelerate and Brake`（合并轴）**：按 **DEL** 清空
实现完美的左脚刹车与独立油门控制。

### 顺带修掉的 bug：`uinput_user_dev` 的轴范围

`absmax[]` / `absmin[]` / `absfuzz[]` / `absflat[]` 是**四个独立的 4 字节数组**，
`absmin` 从 `absmax` 之后 `ABS_CNT*4 = 256` 字节开始，不是 `ABS_CNT = 64` 字节。
写错时所有轴的 min 都会变成 0（摇杆变成 0..32767 的单边轴、合并扳机轴也是单边），
表现就是"游戏只认摇杆/扳机没反应"。

验证：

```bash
python3 - <<'EOF'
import fcntl, struct
EVIOCGABS = lambda a: 0x80000000 | (24 << 16) | (0x45 << 8) | (0x40 + a)
fd = open('/dev/input/eventN','rb')            # N = ds5vpad 打印的节点
for axis in (0,1,2,5):
    v = fcntl.ioctl(fd, EVIOCGABS(axis), bytes(24))
    print(axis, struct.unpack('6i', v)[1:3])   # (min, max)
EOF
# 期望：X/Y/Z 都是 (-32768, 32767)，RZ 是 (0, 255)
```

### 仍未解决

RBR `Options → Controls` 里**扳机轴依旧绑不上**（虚拟手柄和真实 DS5 都一样，只有摇杆能绑）。
已排除的方向：轴范围、合并/拆分扳机、Steam Input、设备被独占、uinput absinfo。
下一步候选：

1. 直接写 `SavedGames/pf*.acm`（RBR 的绑定存在这里：`PAMA` + 设备 GUID + `DEST` +
   `(code, index, type)` 三元组表），绕开 UI；
2. 油门/刹车退回键盘数字输入（RBR 支持键盘轴的渐变），接受手感损失；
3. 用 `input-remapper` 之类把扳机映射到 RBR 认的轴（待验证是否同样绑不上）。

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
# 输入侧：虚拟手柄（DirectInput 独立满行程踏板模式）+ 独占物理 DS5
./bin/ds5vpad -dinput &

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
