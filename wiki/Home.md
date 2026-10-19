# dualsense-c Wiki

专为 **PS5 DualSense (DS5)** 打造的自适应扳机与触感框架。
四层解耦：**Source（信号源）→ Signal（归一化信号）→ Rules（声明式规则）→ Sink（硬件输出）**。

## 它能做什么

| 能力 | 说明 |
|---|---|
| 自适应扳机 | 8 种官方效果模式 + 裸参数，L2 瞄准挡墙、R2 二段式击发 |
| HID 震动 | 双马达，支持正弦/方波调幅 |
| **HD 音圈触感** | 往 USB 声卡 Ch3/4 写 48kHz PCM；可直通游戏音频还原真实材质 |
| 音频 AI | YAMNet（进程内，ONNX Runtime）识别地面材质与枪声/爆炸 |
| 力反馈镜像 | 虚拟手柄捕获游戏标准震动 → 触感 + 后坐力 |
| 遥测接入 | Forza / BeamNG / AC·ACC·ACR（共享内存桥）/ 自定义 UDP |

## 30 秒上手

```bash
make                 # 编译 bin/ds5d、bin/ds5ctl
make install-udev    # 一次性：普通用户读写 /dev/hidraw*
./bin/ds5ctl test    # 手柄自检
./bin/ds5d -profile fps   # 任意射击游戏
```

## 导航

- 新手 → [快速开始](Getting-Started)
- 想给游戏调手感 → [Profile 编写](Profiles) · [扳机效果](Triggers)
- 想要细腻触感 → [HD 触感（音圈）](HD-Haptics) · [音频 AI](Audio-AI)
- 玩 AC/ACC/ACR → [共享内存游戏](ACR-AC-ACC)
- 出问题了 → [故障排查](Troubleshooting)
