# 平台支持

## Linux

完整支持。原生操作 `/dev/hidraw*`，延迟最低。

```bash
make install-udev     # 一次性
./bin/ds5d -profile fps
```

## Windows

HID 硬件层已实现（`internal/device/hidraw_windows.go`）：扳机、马达、灯条、输入解析、
规则引擎、Profile、`trigger`/`udp`/`audio_ai`（UDP）数据源都可用。

```bash
make windows     # bin/ds5d.exe、bin/ds5ctl.exe（纯 Go 交叉编译，无需 mingw）
```

### 尚未适配的部分

| 功能 | 阻塞点 | 需要的改动 |
|---|---|---|
| 音频采集（`source/audio`） | 依赖 `parec` | WASAPI loopback 工具，或 ffmpeg + 虚拟声卡 |
| 音圈输出（`sink/haptics`） | 依赖 `pacat` 写 4 声道 | 向 DS5 的 4 声道 WASAPI 端点写 PCM |
| 音圈直通（`passthrough`） | 上述两项 | 同上落地后自动可用 |
| AI 推理（`-tags onnx`） | cgo 需 mingw + `onnxruntime.dll`，include 路径写死 | flags 可配置化 + `build-windows-onnx` |
| 虚拟手柄（`internal/vpad`） | 需虚拟总线驱动 | 接入 ViGEmBus（其 rumble 回调比 uinput 更完整） |
| 力反馈镜像（`source/ffb`） | 依赖虚拟手柄 | 随 vpad 一起 |
| `scripts/ds5-acr.sh` | bash | PowerShell 版本 |
| `ffclient` / `hidprobe` | evdev FF / hidraw ioctl | Windows HID API 实现 |

### 零改动兜底

Windows 上可让探测器作为独立进程，通过 UDP 把 `surface_*` / `audio_*`
发给 `ds5d.exe`（`source/audio_ai` 线协议平台无关）——不必把模型塞进主进程。

### 注意事项

1. 蓝牙模式下的 0x31 报告与 CRC32 校验需验证
2. 避免 Steam Input 等后台驱动抢占震动端口
