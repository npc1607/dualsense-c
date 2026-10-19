# YAMNet Audio AI Bridge

本工具是 `dualsense-c` 的伴生音频事件探测器原型，用于在外部进程中运行轻量音频小模型（Google YAMNet ONNX，~3.8MB），将识别到的游戏脚步材质（水洼、金属、木板、碎石）及战斗音效（枪声、爆炸、受击）通过 UDP 发送至 `dualsense-c` 的 `source: audio_ai`。

---

## 快速使用

### 1. 模拟联调测试（无需声卡与音频模型）

直接在终端启动模拟发包：

```bash
python3 tools/yamnet_bridge/yamnet_bridge.py --mock
```

将在后台每隔 1.5 秒轮流发送水花、金属踩击、枪声等事件至 `127.0.0.1:9998`。

### 2. 与 dualsense-c 联调验证

在终端 A 启动模拟器：
```bash
python3 tools/yamnet_bridge/yamnet_bridge.py --mock
```

在终端 B 启动仿真守护进程观察信号流：
```bash
./bin/ds5d -dry-run -profile audio-ai-demo -trace
```

---

## 依赖说明（实机推理）

若需对游戏真实声卡音频进行端到端实时推理：

```bash
pip install numpy soundfile onnxruntime sounddevice
```

- 下载 YAMNet ONNX 模型：
  可直接从 HuggingFace 或开源社区获取 `yamnet.onnx` 并放置在当前目录。
- 启动实时监听：
  ```bash
  python3 tools/yamnet_bridge/yamnet_bridge.py --model yamnet.onnx --target 127.0.0.1:9998
  ```
