# 音频 AI（YAMNet）

大多数游戏不会告诉外部程序"你踩在碎石上""刚开了一枪"。音频里其实都有，
用 YAMNet（AudioSet 521 类）识别出来，变成信号驱动音圈与马达。

**进程内运行 ONNX Runtime C API**，不需要 Python；默认构建仍是纯 Go（`-tags onnx` 才启用）。

## 安装

```bash
sudo pacman -S onnxruntime-cpu        # Deb: apt install libonnxruntime-dev

mkdir -p models
curl -L -o models/yamnet.onnx   https://huggingface.co/niobures/YAMNet/resolve/main/yamnetonnx/yamnet.onnx
curl -L -o models/yamnet_class_map.csv   https://huggingface.co/niobures/YAMNet/resolve/main/yamnetonnx/yamnet_class_map.csv

make build-onnx     # → bin/ds5d-onnx
```

## 运行

```bash
./bin/ds5d-onnx -profiles ./profiles -profile far-far-west-hd -rate 125 -trace
```

## 推理流程

```
parec 16kHz → 256 帧块 → feedAI 环形缓冲(1s)
   → 每 classify_ms(120ms) 取最近 1s → classifyLoop(独立 goroutine)
   → ONNX Run → [frames,521] 求平均 → 名称映射 → 信号
```

关键：**窗口末端永远是"现在"**，所以 AI 延迟 ≈ `hop + 推理`（~150ms），不是 1 秒。

## 配置

```yaml
source:
  type: combo
  parts:
    - type: trigger
    - type: audio
      rate: 16000                  # YAMNet 只接受 16k
      channels: 2
      format: float32le
      model: models/yamnet.onnx
      class_map: models/yamnet_class_map.csv
      classify_ms: 120             # 10 次/秒
      threads: 4
      ai_debug: true               # 打印 top-3 类别
      env_debug: true              # 每秒打印 raw/rms/onset 峰值
```

## 标定流程（必须做）

1. 开 `env_debug` 进游戏跑 1 分钟
2. 看 `onset`：开枪应 0.5–1.0，走路应 < 0.4
3. 看 `ai_debug` 里事件的**实际类名与分数**（如 `Gunshot=0.50 Explosion=0.49`）
4. 据此设 `crossed("audio_gunshot", 0.30)` 的门槛

## 事件要用边沿触发

YAMNet 的 1 秒窗口会让分数在门槛之上停留多帧，用电平判断会**拖尾 1 秒**：

```yaml
# 错：一直震
when: 'has("audio_gunshot") && audio_gunshot > 0.15'

# 对：一次事件只响一次
when: 'crossed("audio_gunshot", 0.30)'
```

## 瞬态不要用绝对音量

游戏音乐会把 `loudness` 顶到饱和（实测 p90 = 1.000），绝对阈值两头不讨好。
用 `onset`（fast/slow 包络差）：

| 场景 | onset |
|---|---|
| 静音 | 0.000 |
| 稳定音乐床 | 0.110 |
| 枪声（安静时） | 1.000 |
| 音乐床上开枪 | 0.540 |

```yaml
when: 'has("onset") && onset > 0.55 && loudness > 0.35 && r2 < 0.80'
```

完整说明见仓库 [音频 AI 接入与 HD 触感（推理流程 + 流程图）](https://github.com/npc1607/dualsense-c/blob/main/docs/audio-ai-haptics.md)（含流程图）。
