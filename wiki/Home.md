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
| **直驱力矩降维模拟** | 将专业直驱方向盘 FFB 对齐力矩与推头失速骤降映射为扳机阻尼与音圈抓地力 |
| **伴生语音与电台广播** | 通过手柄内置扬声器（Ch1/Ch2）播放车队无线电、领航路书与战术预警，与音圈物理正交；支持声明式规则与端侧小语言模型（Qwen2.5-0.5B） |
| **FPS 辅助瞄准增强** | 双层复合 S 曲线算法，低速微调死区控制与高速快速转向，阻尼动态平滑 |
| 遥测接入 | Forza / BeamNG / AC·ACC·ACR / RBR（NGP UDP 6776）/ 自定义 UDP |

## 30 秒上手

```bash
make                 # 编译 bin/ds5d、bin/ds5ctl、bin/ds5vpad、bin/ds5radio、bin/ffclient
make install-udev    # 一次性：配置 70-dualsense.rules 与 71-ds5vpad.rules
./bin/ds5ctl test    # 手柄自检
./bin/ds5d -profile fps   # 任意射击游戏
```

## 导航

- 新手 → [快速开始](Getting-Started)
- 想给游戏调手感 → [Profile 编写](Profiles) · [扳机效果](Triggers)
- 想要细腻触感 → [HD 触感（音圈）](HD-Haptics) · [音频 AI](Audio-AI)
- 赛车直驱模拟与抓地力极限 → [直驱 FFB 力矩与抓地力曲线](Direct-Drive-FFB)
- 玩 AC/ACC/ACR → [共享内存游戏](ACR-AC-ACC)
- 玩 RBR / RSF 拉力赛 → [理查德·伯恩斯拉力赛](RBR)
- 想要伴生语音与车队电台 → [车载电台与伴生系统](Companion-Audio-Agent) · [电台架构设计](Radio-Agent-Architecture)
- 想要 FPS 瞄准辅助与手感曲线 → [FPS 辅助瞄准增强](FPS-Aim-Assist)
- 出问题了 → [故障排查](Troubleshooting)
