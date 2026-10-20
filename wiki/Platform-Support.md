# 平台支持

## Linux

完整支持。原生操作 `/dev/hidraw*`，延迟最低。

```bash
make install-udev     # 一次性
./bin/ds5d -profile fps
```

## Windows

Windows 现已全功能适配，默认构建保持 100% 纯 Go（`CGO_ENABLED=0`，使用 Win32 / SetupAPI / WASAPI COM / ViGEmClient 动态加载），无需安装 C 工具链即可直接编译或从 Linux/macOS 交叉编译：

```bash
make windows
# 产物：
#   bin/ds5d.exe       主守护进程（自适应扳机、马达、灯条、WASAPI 音频采集与 4 声道音圈输出）
#   bin/ds5ctl.exe     设备枚举、自检、校准与报文嗅探工具
#   bin/ds5vpad.exe    基于 ViGEmBus 的虚拟 Xbox 360 手柄与力反馈捕获守护进程
#   bin/ds5bridge.exe  AC / ACC / ACR 共享内存遥测桥接器
#   bin/ffclient.exe   基于 XInput 的震动测试客户端
#   bin/hidprobe.exe   音圈报告变体硬件探测工具
```

### Windows 已适配模块一览

| 功能 | Windows 实现机制 | 说明 |
|---|---|---|
| HID 硬件层（`internal/device`） | SetupAPI + `hid.dll` + `ReadFile`/`WriteFile` | 自适应扳机、马达、灯条、陀螺仪/加速度计/触控板/电量解析 |
| 音频采集（`source/audio`） | 原生 WASAPI Loopback（`internal/wasapi`） | 默认直接通过 COM `IAudioClient` + `IAudioCaptureClient` 抓取系统回环音频（`float32le`），无需外部录音工具；亦支持通过 `command` 参数自定义（如 `ffmpeg`） |
| 音圈输出（`sink/haptics`） | 原生 WASAPI 4 声道渲染（`internal/wasapi`） | 自动定位 DualSense USB 声卡端点，通过 `WAVEFORMATEXTENSIBLE`（48 kHz 4ch `s16le`，`SPEAKER_QUAD` `0x33` 掩码）向 Ch3/4 音圈直写 PCM |
| 音圈直通（`passthrough`） | WASAPI Loopback → 25–180 Hz 带通 → WASAPI 4ch Render | 零外部依赖，把游戏原始低频谱实时送到左右音圈 |
| 虚拟手柄（`internal/vpad`） | `ViGEmClient.dll` + ViGEmBus 驱动 | 创建虚拟 Xbox 360 手柄并转发物理 DualSense 输入，通过 `vigem_target_x360_register_notification` 原生回调捕获游戏力反馈 |
| 力反馈镜像（`source/ffb`） | `ds5vpad.exe` → UDP `FFB1` → `ds5d.exe` | 比 Linux `uinput` 具备更完整的实时震动强度回调 |
| AI 推理（`-tags onnx`） | `make build-windows-onnx` | 支持 Windows 宽字符模型路径（`wchar_t*`）与可配置 `ORT_WIN_DIR` / `MINGW_CC` |
| 启动脚本（`scripts/ds5-acr.ps1`） | PowerShell 5.1+ | 原生管理 `ds5d.exe` 与 `ds5bridge.exe`（`start`/`stop`/`restart`/`bridge-restart`/`status`/`logs`） |
| 诊断工具（`ffclient.exe` / `hidprobe.exe`） | `XInputSetState` / `internal/device` | 验证虚拟手柄震动链路与音圈 HID 报告标志位 |

### 运行时可选前置条件

1. **虚拟手柄与力反馈捕获（`ds5vpad.exe`）**：
   - 安装 [ViGEmBus](https://github.com/nefarius/ViGEmBus/releases) 驱动。
   - 将 `ViGEmClient.dll`（x64）放在 `bin\ds5vpad.exe` 同目录或系统 `PATH` 中。
2. **进程内 ONNX 音频分类器（`ds5d-onnx.exe`）**：
   - 使用 `make build-windows-onnx ORT_WIN_DIR=/path/to/onnxruntime-win-x64` 编译，并将 `onnxruntime.dll` 放在 `ds5d-onnx.exe` 同目录。
   - 或使用零 C 依赖方案：运行独立分类器进程，通过 UDP 把 `surface_*` / `audio_*` 发给 `ds5d.exe`（`source/audio_ai` 线协议平台无关）。

### 注意事项

1. 音圈输出（`sink/haptics`）需要通过 USB 连接手柄（蓝牙模式下手柄不暴露 4 声道 USB 声卡）。
2. 蓝牙模式下的 0x31 报告与 CRC32 校验建议先用 `ds5ctl.exe test` 验证。
3. 避免 Steam Input 等后台驱动同时写物理手柄震动端口（使用 `ds5vpad.exe` 时可关闭 Steam Input）。
