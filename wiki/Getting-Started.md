# 快速开始

## 环境要求

- Go 1.24+
- Linux（hidraw）或 Windows（HID API）
- DualSense 手柄，**推荐 USB 有线**（蓝牙带宽受限、稳定性较差）

## 1. 构建

```bash
make            # 编译 bin/ds5d, ds5ctl, ds5vpad, ds5radio, ffclient 以及 bin/ds5bridge.exe
make bridge     # bin/ds5bridge.exe（AC/ACC/ACR 用，Windows 目标）
make build-onnx # bin/ds5d-onnx（可选：进程内音频 AI，需 onnxruntime）
```

## 2. 权限（Linux，一次性）

```bash
make install-udev
# 等价于
#   sudo cp udev/70-dualsense.rules udev/71-ds5vpad.rules /etc/udev/rules.d/
#   sudo udevadm control --reload-rules && sudo udevadm trigger
```

装完**重新插拔手柄**。验证：

```bash
./bin/ds5ctl list
# /dev/hidraw6 [usb] 054c:0ce6 Sony Interactive Entertainment DualSense Wireless Controller
```

> 手柄节点编号会随重插/重枚举变化（`hidraw6 → hidraw7`）。**不要在脚本里写死 `-hidraw`**，
> 让程序自己 `Find()`；`ds5d` 掉线后会自动重连。

## 3. 自检

```bash
./bin/ds5ctl test          # 震动 → L2 阻力 → R2 阻力 → 反馈模式
./bin/ds5ctl bench         # 轮巡 8 种扳机效果，对比手感
./bin/ds5ctl watch         # 实时打印按键/扳机/传感器（含电量）
```

## 4. 跑起来

```bash
# 射击/动作游戏（输入驱动，免配置）
./bin/ds5d -profile fps

# 通用双段扳机（任何游戏）
./bin/ds5d -profile generic

# 赛车载具遥测（Forza）
./bin/ds5d -profile forza-horizon

# AC / ACC / AC Rally（两段式 / 自动模式）
./scripts/ds5-acr.sh start      # 启动 ds5d 守护进程（AppID 默认 3917090 = AC Rally）
./scripts/ds5-acr.sh bridge     # 游戏启动并进入赛段后，注入共享内存桥
# 或直接使用全自动侦测模式：
./scripts/ds5-acr.sh auto       # 启动 ds5d 并自动侦测游戏进程后注入桥
./scripts/ds5-acr.sh status
./scripts/ds5-acr.sh stop
```

## 5. 常驻

```bash
nohup ./bin/ds5d -profile fps >> /tmp/ds5d.log 2>&1 &
```

## 无手柄联调

```bash
./bin/ds5d -dry-run -profile fps -trace        # 纯逻辑仿真
./bin/ds5d -list                               # 列出内置 profile
./bin/ds5ctl check -profile my.yaml            # 静态校验 YAML
```
