# HD 触感（音圈）

DS5 内部是两枚**音圈**（voice coil），没有传统转子马达。驱动它们有两条路：

| 通道 | 分辨率 | 能表达 |
|---|---|---|
| HID 震动字节 | 8 bit 幅度 + 一个调幅频率 | "嗡嗡"的强弱快慢 |
| **USB 音频 Ch3/4** | 48kHz / 16bit 任意波形 | 频谱、谐波、瞬态 |

「碎石、金属、水面」的差别本质是**频谱差异** —— HID 做不出来，只能靠音圈。

## 两种模式

```yaml
haptics:
  mode: passthrough     # synth（合成载波）| passthrough（游戏音频直通）
  target: auto
  low_hz: 55            # passthrough 通带下限
  high_hz: 200          # 上限
  carrier_hz: 55        # synth 模式的基频
  gain: 0.7             # 总增益
  source: auto          # 采集的 monitor
```

| 模式 | 音圈放什么 | 适合 |
|---|---|---|
| `synth` | 55Hz 基频 + 2/3 次谐波，幅度由规则给 | 事件冲击、无音频源 |
| **`passthrough`** | **游戏音频 55–200Hz**，规则给的是**增益** | 真实材质纹理（推荐） |

## 规则写法（passthrough）

```yaml
- name: gravel-coil
  when: 'surface_gravel > 0.20 && surface_gravel >= surface_water && ...'
  target: haptic_left
  value: "clamp(80 + 270 * surface_gravel, 0, 180)"   # 0..255 → 0~70% 增益
```

## ⚠️ 三个必须知道的坑

**1. HID 报告空闲时不能声明震动位**

`valid_flag0` 是应用掩码。若在 motor=0 时仍声明
`COMPATIBLE_VIBRATION|HAPTICS_SELECT`，固件会把执行器锁在 HID 引擎（幅度 0），
**音频流的音圈信号被完全忽略**。本项目只在真的有震动时才声明。

**2. `valid_flag0` 的 BIT4-7 必须清零**

否则会启用但未填充的音量字段（值为 0），导致**整个 USB 音频 DAC 被静音**
（表现为连手柄扬声器都没声音）。

**3. `pacat` 必须指定 `--channel-map`**

```bash
pacat ... --channels=4   --channel-map=front-left,front-right,rear-left,rear-right
```

默认 4 声道布局是 `front-left,front-center,front-right,rear-center`，
会把我们的 Ch3/4 混进耳机通路，音圈一个采样都收不到。

## 音量经验

- 常驻底纹（哪怕 18/255）在脚步频繁的游戏里会显得"一直震"
- 建议只在**识别出材质**或**事件**时给增益
- `haptics.gain` 是总音量，`haptic_*` 是分路增益，两者相乘

## 验证

```bash
# 往 Ch1/2 灌 1kHz 应能听到声音（验证音频通路）
# 往 Ch3/4 灌 40Hz 应能感到震动（验证音圈）
./bin/ds5d -profile audio-haptics -trace
```

详见 [音频 AI](Audio-AI) 与仓库文档 `docs/audio-haptics-and-ffb.md`。
