# 故障排查

## 手柄相关

| 现象 | 原因 | 处理 |
|---|---|---|
| `ds5ctl list` 找不到设备 | 权限未生效 | `make install-udev` 后**重新插拔** |
| 持续 `no DualSense found` | VID/PID 未收录或第三方手柄 | `-hidraw /dev/hidrawN` 显式指定 |
| 马达震但扳机无力 | 选错了 hidraw 节点 | `ds5ctl bench -hidraw /dev/hidrawN` 遍历 |
| 扳机手感与预期不符 | 固件差异 | `ds5ctl bench` 对比 8 种模式；**挡墙优先用 `rigid`** |
| 手柄反复断连重连 | USB 接触/供电；掉线后节点会换号 | 换线/直插主板后置口；`ds5d` 会自动重连 |

> **不要写死 `-hidraw`** —— 节点编号会随重枚举变化（`hidraw6 → 7 → 8`）。

## 与 Steam 冲突

| 现象 | 说明 |
|---|---|
| 震动被覆盖/闪烁 | Steam Input 也在写 HID 输出；关闭该游戏的 Steam Input |
| 关了 Steam Input 游戏就不能操作 | 该游戏只认虚拟 Xbox 手柄；保持开启，我们的触感走 hidraw 不受影响 |
| 需要"游戏能操作 + 独占输出" | 用 `ds5vpad` 自建虚拟手柄（见 [力反馈镜像](FFB-and-Virtual-Pad)） |

## 音频触感

| 现象 | 原因 | 处理 |
|---|---|---|
| 信号一直是 0 / 永久静音 | `pw-record` 的 `--target` 被忽略 | 改用 `parec`/`pacat`（本项目默认） |
| 延迟 1 秒以上 | `parec` 少了 `--latency-msec` | 命令模板里补 `--latency-msec=20 --process-time-msec=2` |
| 音圈完全没反应 | 声道映射错位 | `pacat` 必须带 `--channel-map=front-left,front-right,rear-left,rear-right` |
| 连手柄扬声器都没声音 | `valid_flag0` 的 BIT4-7 被置位 → DAC 静音 | 用 `0x0F`，不要用 `0xFF` |
| 音圈"没反应" | 空闲报告声明了 `COMPATIBLE_VIBRATION/HAPTICS_SELECT` | 只在真有震动时声明（本项目已处理） |
| 走路一直震 | 常驻底纹/材质增益过高 | 删掉常驻 `haptic_*`；`low_hz` 抬到 55–70 |
| 事件拖尾 1 秒 | AI 事件用电平判断 | 改用 `crossed(sig, 0.30)` |

## AI 相关

| 现象 | 处理 |
|---|---|
| `unknown name onset` / `surface_*` | 二进制版本旧，`make build` 后重试 |
| `built without an inference engine` | 需要用 `make build-onnx`（`-tags onnx`） |
| `Invalid rank for input: waveform` | 模型要求 rank-1 输入（代码已处理） |
| AI 看起来很慢 | 先确认采集延迟（`parec --latency-msec`），再判断模型 |

## 共享内存游戏（AC/ACC/ACR）

| 现象 | 处理 |
|---|---|
| 桥一直 `shared memory not found` | 还没进赛道（正常）；或确认 AppID 正确 |
| 桥日志停止增长但进程还在 | 桥卡死 → `./scripts/ds5-acr.sh bridge-restart` |
| `signals: (none)` | 桥没连上或 ds5d 未监听 9999；`ds5-acr.sh status` |
| 遥测数值异常波动 | 用 `ds5ctl sniff -listen :9999` 或桥的 `-debug` 核对字段 |

## 通用排查手段

```bash
./bin/ds5ctl list                 # 设备枚举
./bin/ds5ctl watch                # 按键/扳机/传感器/电量
./bin/ds5ctl check -profile x.yaml # 静态校验
./bin/ds5d -profile x -trace      # 实时信号
./bin/ds5d -dry-run -profile x -trace   # 无手柄仿真
```

## 排查方法论（血泪）

1. **先看日志再调参**：门槛、延迟的多次误判都是靠 `-trace` / `*_debug` 定位的
2. **抽样日志会掩盖尖峰**：脉冲信号要在**发送端按帧**记录，不要在消费端按时间采样
3. **共享资源要同时验证**：喇叭和音圈共用 DAC，不能用"喇叭有声"推断音频通路 OK
4. **HID 报告是状态不是指令**：进程退出后最后一次报告仍然生效，必要时物理重插
5. **单测覆盖不了硬件语义**：协议/帧格式必须上真机，并准备最小复现工具
