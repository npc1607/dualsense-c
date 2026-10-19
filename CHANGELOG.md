# Changelog

按 tag 分节。release 流水线会自动抽取对应小节（`## <tag>`）作为发布说明，
所以**新版本发布前必须在这里加一节**，否则发布正文会退化成一句 compare 链接。

## v0.1.3 - 2026-10-19

### 新增
- 输入解析补齐手柄传感器：陀螺仪 `gyro_x/y/z`、加速度 `accel_x/y/z`、触控板坐标
  `touch_x/touch_y/touch_active`、电量 `battery/charging`（`ds5ctl watch` 同时打印原始字节便于校准）
- 规则新增 `mute_led` target（静音键指示灯）

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
