# Changelog

按 tag 分节。release 流水线会自动抽取对应小节（`## <tag>`）作为发布说明，
所以**新版本发布前必须在这里加一节**，否则发布正文会退化成一句 compare 链接。

## Unreleased

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

### 修复（均为真机功能测试发现）
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
