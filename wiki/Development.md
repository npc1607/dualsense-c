# 开发与架构

## 四层架构

```
Source ──► Signal ──► Rules ──► Sink
数据源     归一化      声明式     硬件输出
```

| 层 | 职责 | 适配新游戏时的改动 |
|---|---|---|
| Source | 接入游戏状态，转为标准信号 | 写 YAML / 加解码器 |
| Signal | 时间戳、过期、死区、变化率限制 | 框架维护 |
| Rules | 表达式把信号映射为扳机/马达/灯条动作 | 只改 YAML |
| Sink | 固定时钟（125~250Hz）写 HID / 音圈 | 框架维护 |

## 目录

```
cmd/
  ds5d/         主守护进程
  ds5ctl/       设备检测、自检、抓包校准
  ds5vpad/      虚拟手柄 + 力反馈捕获
  ds5bridge/    Proton 前缀内的共享内存桥（Windows 目标）
internal/
  audioai/      YAMNet 推理（ONNX Runtime C API，-tags onnx）
  bridge/       桥接协议与共享内存布局
  device/       跨平台 HID（Linux hidraw / Windows HID API）
  hid/          输出报告与扳机参数库
  input/        输入报告解析
  profile/      Profile 结构、内置 YAML、校验
  rules/        表达式引擎、优先级、锁存与衰减
  signal/       信号词表与时间序列
  sink/         时钟驱动；haptics/ 音圈合成与直通
  source/       trigger / udp / audio / ffb / audio_ai / combo / sim
  vpad/         虚拟手柄后端（Linux uinput / Windows 占位）
docs/           设计与踩坑文档（必读）
profiles/       用户自定义 profile
tools/          bridgesim / ffclient / hidprobe / yamnet_bridge
```

## 构建与测试

```bash
make            # 常用二进制
make build-onnx # 带 AI（CGO + onnxruntime）
make windows    # 纯 Go 交叉编译
make test       # 全部单测
make vet        # 三平台 build 检查
make fmt        # gofmt
```

## 开发约定

1. **默认构建保持纯 Go / 零 CGO**：CGO 能力一律走 build tag（如 `onnx`）
2. **信号词表只追加**：桥用索引传信号，插入中间会破坏兼容
3. **纯算法用单测**：DSP、包络、PCM 声道（`source/audio`、`sink/haptics`）
4. **协议/帧格式上真机**：准备最小复现工具（`ffclient`、`hidprobe`、`bridgesim`）
5. **HID 报告是应用掩码**：只声明本帧真正填写的字段

## 相关文档

**Wiki 内**

- [音频 AI 接入与 HD 触感](Audio-AI)
- [HD 触感（音圈）](HD-Haptics)
- [力反馈镜像（虚拟手柄）](FFB-and-Virtual-Pad)
- [故障排查](Troubleshooting)

**仓库文档**（设计细节与真机数据）

- [音频 AI 接入与 HD 触感：推理流程与调优](https://github.com/npc1607/dualsense-c/blob/main/docs/audio-ai-haptics.md)
- [音频触感与力反馈镜像：设计与踩坑](https://github.com/npc1607/dualsense-c/blob/main/docs/audio-haptics-and-ffb.md)
- [DS5 硬件能力、驱动边界与高清触感方案](https://github.com/npc1607/dualsense-c/blob/main/docs/hardware-capabilities.md)
