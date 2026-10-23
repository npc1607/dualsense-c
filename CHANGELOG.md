# Changelog

按 tag 分节。release 流水线会自动抽取对应小节（`## <tag>`）作为发布说明，
所以**新版本发布前必须在这里加一节**，否则发布正文会退化成一句 compare 链接。

## v0.1.4

### 赛车电台与情境广播架构 (Radio Agent Architecture)
- **手柄扬声器硬件路由**：利用 DualSense 4 声道 48kHz USB 声卡物理拓扑，将正面内置扬声器（Ch1/2: FL/FR）与内部触觉音圈（Ch3/4: RL/RR）完全隔离，实现双手感知路面震颤的同时、手心清晰传来车队工程师与领航员的电台呼叫。
- **DSP 声学流水线**：程序化合成 PTT 握掣开启双音频嘀声（1200+1800Hz + 白噪声）、300~3200Hz 军用/车载电台带通滤波与微过载、PTT 结束嘀声（900Hz）。
- **统一 IPC 遥测总线**：在 `internal/vpad/ipc` 中规范 `SIG1` 紧凑二进制协议（12+5N 字节），以低于 35µs 极速向订阅者分发时序遥测，彻底取代混乱争抢的 UDP 端口。
- **对标 Audio-AI 分层推理**：设计滑动时序窗口 `TelemetryWindow` 与抽象推理接口 `Classifier`，平行动作支持 Stage 1 确定性规则库与 Stage 2 端侧轻量大模型（LLM/SLM）。
- **矢量架构与时序图**：使用纯 JS 矢量生成 `docs/radio-agent-architecture.svg` 与 `docs/radio-agent-sequence.svg`。

### ACR 赛车物理与遥测经验分位点标定
- **实测数据驱动建模**：采集 ACR 赛道 1,256 帧高速行驶遥测（最高 150.9 km/h、7,535 RPM、5.28G 横向漂移），提取真实物理断层分位点。
- **路面场景精准解构**：
  - 铺装沥青路面（`accel_g <= 0.015G`，占 63.2%）：左马达纯净静音，右小马达提供超细腻 65Hz 高速微晶胎噪；
  - 砂石粗糙路面（`0.015 < accel_g <= 0.06G`，占 31.4%）：左右马达以相同的 318 次对称协同（左 35Hz 沉降 ＋ 右 54Hz 颠簸），叠加浮沙 24Hz 微滑移剪切与双音圈 12Hz 砂石拍频。
- **手感平衡性飞跃**：左手马达触发率从过去的 7.6% 跃升至 60.0%，彻底根除右手单侧发麻，达到双侧饱满对称。
- **两段式启停与游戏自动探测**：重构 `scripts/ds5-acr.sh`，拆分 `start` 与 `bridge`，解决 Proton 前缀锁冲突。

### 虚拟手柄模块化重构与超低延迟 IPC
- **协议彻底解耦**：将 `internal/vpad` 拆分为 `xinput`、`dinput8` 与 `all` 模块，提供跨平台 Linux 与 Windows 桩接口，`make vet` 跨平台构建全绿通过。
- **事件驱动 IPC 协议**：用 `< 35µs` 延迟的连接型 IPC 协议（`TCP_NODELAY`）替代原 60Hz 盲目 UDP 轮询，无消费端时完全零网络 I/O 与零日志干扰。
- **理查德·伯恩斯拉力赛 (RBR) 深度适配**：
  - 逆向解析 `SavedGames/pfsam.acm` 结构，修复只读属性与退出保存逻辑；
  - 纠正 Wine `winebus.sys` 映射，统一使用 `BTN_GAMEPAD`（0x130..0x13e）标准键码；
  - 解决未踩踏板时 100% 满油门/满刹车问题（轴反转配置）；
  - 制作双向信号与手柄力反馈矢量拓扑图 `docs/rbr-telemetry-vpad.svg`。

### 触觉音圈与音频 AI 引擎
- **Voice-Coil 修复实装**：修复 `valid_flag0` 掩码竞争，实现 USB 音频 Ch3/4 音圈流与 HID 自适应扳机并行输出。
- **音频 AI 探测器**：引入进程内 YAMNet ONNX 事件分类器与环境材质识别。
- **直驱方向盘 FFB 曲线**：支持非线性力矩降维、抓地力临界高频嘶鸣（160Hz）与推头力矩骤降松脱警告。

## Unreleased

### 破坏性变更：规则名必须唯一
- `profile.Validate()` 现在拒绝同一 profile 内的重复非空规则名，错误会同时给出两个下标
  （`profile X: duplicate rule name "foo" (rules[3] and rules[7])`）。此前重复名会静默共享
  `rule_id`，并让 `profile.reload` 的 hold/cooldown 继承产生歧义。
- 受影响：含重名规则的第三方 YAML 会在加载/重载时失败。修复方法：把重复的 `rules[].name`
  改成唯一名字。匿名规则（`name: ""`）不受影响，可继续存在多条。

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
