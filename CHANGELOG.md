# Changelog

按 tag 分节。release 流水线会自动抽取对应小节（`## <tag>`）作为发布说明，
所以**新版本发布前必须在这里加一节**，否则发布正文会退化成一句 compare 链接。

## Unreleased

### 文档
- README 新增「Windows 适配缺口」与「Windows 适配路线」：逐项列出未适配功能
  （音频采集/音圈输出/直通/AI 引擎构建/虚拟手柄/力反馈/脚本/调试工具）及阻塞点

- 新增 `docs/audio-ai-haptics.md`：音频 AI 的接入步骤、推理流程（生命周期/窗口/
  类别映射）、HD 触感的三条触发通路、信号与参数、标定方法、延迟预算、调优速查
- 新增流程图 `docs/audio-ai-flow.svg`（推理管线）与 `docs/audio-ai-hd.svg`
  （三条触感通路），由 `scripts/generate_audio_ai_flow.js` 生成
- README：延伸阅读加入新文档、`haptics` 块补充 `mode/low_hz/high_hz`、
  信号表补充 `onset` 与 AI 材质/事件信号、目录结构补充 `internal/audioai`

### 修复：音圈（Voice-Coil）终于通了
- **根因**：`valid_flag0` 是「应用掩码」。ds5d 每 8ms 都写
  `COMPATIBLE_VIBRATION|HAPTICS_SELECT`（BIT0+BIT1）而 motor 字节为 0，
  固件因此认为「执行器由 HID 震动引擎驱动，幅度 0」，
  **USB 音频通道（Ch3/4）的音圈信号被完全忽略**。
  表现为：往 Ch3/4 灌 40Hz 毫无反应，而喇叭通道正常。
- **修复**：只有本帧真的有震动时才声明这两个位；空闲时 `valid_flag0` 只保留
  扳机位（`0x0C`），执行器交还给音频流。`valid_flag2` 的玩家灯位保持原样。
- 实测（fw 0x0110002a, USB）：修复后播放 50Hz 低频，`bass` 升到 0.47 时
  **音圈产生真实震动**。
- 附带确认：`valid_flag0` 的 BIT4-7（音量/audio_control）必须保持清零，
  否则会把整个 USB 音频 DAC 静音（此前的「完全没声音」就是这个原因）。
- 新增回归测试 `TestVibrationFlagsOnlyWhenRumbling`：
  空闲帧不得声明震动位、必须保留扳机位、音量位恒为 0。
- `assetto-corsa` profile 接入音圈：路肩 / 撞击 / 路面细颗粒同时驱动
  HID 马达（体感）与音圈（细节）。

### 新增（Far Far West 射击 profile 与 combo 源）
- `source/combo`：一个 profile 只能写一个 source，但射击游戏需要「手柄扳机 +
  系统音频」两路同时输入。combo 并行跑多个子源并把帧合并
  （`signal.Set` 按信号独立更新，互不覆盖）
- `far-far-west` profile（AppID 3124540）：COD 风格扳机挡墙
  （L2 44% 硬墙 235、R2 67% 硬墙 255、触底泄压）+ 单发重击 + 按住连发后坐力
  + 枪声/爆炸的音频触感（马达与音圈并行）
- 音圈参数按射击手感调过：载波 70→52Hz、PCM 增益 0.7→0.95、单发音圈 130ms

### 修复（Far Far West 实测暴露）
- `ds5-acr.sh` 之外新增按实测数据调参的依据：本作枪声在 160–450Hz 几乎无能量
  （mid 峰值 0.041），任何 `mid` 门槛都会让连发规则在第二枪后失效 →
  改用 `loudness`/`bass` 驱动音频触感，连发只按扳机位置判定
- `hold` 语义纠正：按住时 `when` 持续为真，`hold` 只决定松手后的尾巴。
  之前用 300ms 救连发，导致单点松手后拖尾
- assetto-corsa：撞击必须是全场最强输出，路肩封顶降到 235/220，
  撞击斜率提高（150+45·x / 140+45·x）
- 测试 `TestBuiltinAssettoCorsaRoadFeel` 改为多帧取峰值比较，
  避免调幅相位导致单帧断言抖动

### 新增
- `source/audio`：抓 PipeWire monitor → 三段带通（`bass` 20-110Hz、`mid` 160-450Hz）
  与全带 `loudness` 的包络信号，attack/release 可配，纯 Go + CLI 子进程
- `sink/haptics`：往 DS5 的 4 声道 USB 声卡 Ch3/4 写 48kHz PCM 驱动音圈，
  新增 `haptic_left` / `haptic_right` target 与 profile 的 `haptics:` 配置块
- `source/ffb` + `cmd/ds5vpad`：uinput 虚拟 Xbox 手柄 + 独占物理手柄，
  捕获游戏 FF_RUMBLE 上传幅度并经 UDP 9998 交给 ds5d；`tools/ffclient` 可离线验证
- 新增 `audio-haptics` 与 `ffb-recoil` 两个内置 profile
- `internal/vpad` 平台分层：Linux 用 uinput 实现，Windows 留 ViGEm 接口占位
- 新增 `udev/71-ds5vpad.rules`
- 新增 `tools/hidprobe`：逐项对比输出报告的 flag/长度/音量组合，用编号震动做标记
  （`-no-marker` 可关闭，否则标记震动会掩盖音圈本身的反馈）

### 修复（均为真机功能测试发现）
- `hid/report.go`：USB 输出报告的 `valid_flag0` 从 `0xFF` 改为 `0x0F`。
  `0xFF` 会「启用但并不填充」headphone/speaker/microphone 音量与 audio_control
  这几个字段，而它们的值恒为 0，等于把控制器 USB 音频 DAC 静音。
  实测：`0xFF` 下往 Ch1/2 灌 1kHz 完全没有声音；改 `0x0F` 后 1kHz 正常出声。
  马达与扳机效果不受影响（bit0-3 未变，6Hz 满幅震动实测正常）
- `sink/haptics`：`pacat` 必须显式指定 `--channel-map=front-left,front-right,
  rear-left,rear-right`。默认的 4 声道布局是 `front-left,front-center,
  front-right,rear-center`，会把第 3/4 通道混进耳机通路，音圈收不到任何数据
- `source/audio`：空闲 sink 会让 `ReadFull` 永久阻塞、源再也不发帧，下游看到过期值；
  改为带读超时，空闲时主动输出衰减帧
- `source/audio`：包络原本每个音频块只调一次 `step`，而系数是按每采样算的，
  90ms 的释放被拉长到约 23 秒；改为按块长度解析衰减 `stepN`
- `source/audio` / `sink/haptics`：`pw-record` / `pw-cat` 的 `--target` 在部分
  PipeWire 版本上被静默忽略（实测录到的是默认节点），默认命令改用 `parec` / `pacat`
- `sink/haptics`：`DetectSink` 把 `pactl` 整行（含格式/状态）当成 sink 名

### 新增（ACR / ACC / AC Rally 路面反馈）
- AC / ACC / AC Rally 桥接新增底盘与路面信号：`accel_g`（accG[1] 垂直加速度，已归一为 G）、
  `lat_g`（accG[0]）、`steer`、`surface`（四轮 `tyreDirtyLevel` 最大值）、
  `custom1`（`carDamage` 最大值）、`custom2`（`suspensionTravel` 最大值）
- `FieldSpec` 支持 `Scale`（单位换算）与 `Count`（数组取绝对值最大项），
  `ds5bridge -debug` 同时打印 accelG / latG / steer / surface / damage / susp
- `assetto-corsa` profile 新增路面细节规则：铺装路面细颗粒、路肩拍频震动、
  路外草地砂石粗糙感、撞击重锤（左右马达不同频率以形成拍频）
- 规则引擎新增 `absf(x)` 绝对值函数

### 修复
- `sink` 增加热插拔恢复：控制器 USB 重枚举后 hidraw 节点会换号（如 hidraw6 → hidraw8），
  以前写失败只会一直重试旧的 fd、永不重连，表现为「手柄掉线后再也没有输出」；
  现在写失败会每秒重试一次 `device.Find()` 并重新 attach
- `abs()` 在任何 profile 中都不可用：`abs` 是 ABS 介入信号名，会遮蔽同名函数；
  README 中该说明已更正，写绝对值请用 `absf(x)`

## v0.1.3 - 2026-10-19

### 新增
- 输入解析补齐手柄传感器：陀螺仪 `gyro_x/y/z`、加速度 `accel_x/y/z`、触控板坐标
  `touch_x/touch_y/touch_active`、电量 `battery/charging`（`ds5ctl watch` 同时打印原始字节便于校准）
- 规则新增 `mute_led` target（静音键指示灯）
- 规则引擎内置三角函数 `sin` / `cos` 支持，`generic` profile 新增充电淡蓝呼吸灯与满电指示规则
- 新增 `docs/hardware-capabilities.md`：系统分析 DS5 硬件能力、信号源限制与 Voice-Coil 高清音频触感方案

### 修复
- `player_leds` 之前可能不生效：`valid_flag2` 现在只在值非 0 时打开 player-LED 控制位
  （0 仍表示"不动它"，避免把默认的玩家指示关掉）

> 传感器偏移按社区文档实现，未逐固件验证；用 `ds5ctl watch` 先确认再写规则。

## v0.1.2 - 2026-10-19

### 新增
- `CHANGELOG.md` 与 `.github/scripts/release-notes.sh`：发布正文改为「本版本改动 +
  包内文件说明 + 快速开始」，标题改为 `dualsense-c <tag>`

### 变更
- 发布已存在时同时更新标题与正文，不再只覆盖资产

## v0.1.1 - 2026-10-19

### 修复
- `.gitignore` 补上 `dist/`：CI 暂存产物会被 Go 判为未跟踪文件，导致二进制里
  的模块版本戳变成 `v0.1.0+dirty`（`go version -m` 可见）

## v0.1.0 - 2026-10-19

首个版本。

### 新增
- 四层框架：`source → signal → rules → sink`，换游戏只改 YAML
- `fps` / `generic`：输入驱动（任意游戏可用），瞄准挡墙 + 开火后坐力
- `forza-horizon`（UDP 5300，Data Out / Dash）、`beamng`（OutGauge 4444）
- `dirt-family`（UDP 20777，附 `ds5ctl sniff` 校准流程）
- `assetto-corsa`：`ds5bridge.exe` 在 Proton 前缀内读 `acpmf_physics`，UDP 回传
- `ds5ctl`：`list` `profiles` `check` `test` `bench` `watch` `sniff` `version`
- Linux hidraw + Windows HID（实验性），USB 48B / 蓝牙 78B 报告
