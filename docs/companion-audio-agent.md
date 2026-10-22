# 全品类手柄伴生声学播报系统 (Companion Audio Agent)

> **设计核心原则**：架构设计大于一切。以统一低延迟 IPC 为主干，对标 Audio-AI 分层范式，声明式 Profile 规则驱动，物理隔离四声道硬件路由。

---

## 一、 背景与声学拓扑 (Hardware Topology)

PlayStation 5 旗舰游戏（如《GT7》、《F1》系列、射击游戏）震撼的沉浸感体验很大程度来源于 DualSense 控制器的声学与力学协同：
- **掌心两侧（Ch3/Ch4 触觉音圈）**：沉浸于粗暴而细腻的高频路感、悬挂冲击与浮沙切削；
- **手掌正中（Ch1/Ch2 内置扬声器）**：伴随着真实的对讲机开关嘀声（PTT Beep）与沙沙的电台静电杂音，车队工程师与领航员的指令清晰传来（“左后轮抓地力下降，稳住出弯！”、“Box this lap, box box”）。

在 Linux / Steam 平台上，DualSense 同时是一块标准的 **4 声道 48kHz USB 声卡**：
- **Ch1 / Ch2（FL, FR 前置声道）**：直连手柄外壳正面的微型内置扬声器（及 3.5mm 耳机孔）；
- **Ch3 / Ch4（RL, RR 后置声道）**：直连手柄两侧内部的高清触觉音圈（Voice-Coil Actuators）。

本项目通过解构 DualSense 硬件声卡拓扑，建立了一套**完全解耦、由统一 IPC 遥测总线驱动、声明式 Profile 规则与端侧小模型（Qwen2.5-0.5B）混合驱动的全品类伴生广播系统**。

---

## 二、 声明式 Profile 规则体系 (Declarative Rules)

语音播报不是硬编码在程序里的死规则，而是原生接入现有的 YAML Profile 体系。在任意 Profile 中直接使用 `target: speak` 或 `target: radio` 即可编写伴生语音：

### 1. 语法规范

```yaml
rules:
  - name: 规则标识名
    when: "物理条件表达式（支持 expr-lang 语法与数学函数）"
    target: speak 或 radio # speak: 纯声伴生 / radio: 电台播报
    priority: 优先级数值   # 1~100，数值越大优先级越高，支持抢占打断
    clip: "音频切片名称"   # 存放在 assets/radio/ 下的 .wav 文件名（不含扩展名）
    cooldown: 冷却时间     # 如 10s, 15s，防止重复触发轰炸
    hold: 持续保持时间     # 如 1.2s，需满足条件持续一定时间才触发
    text: "自然语言说明"   # 提示词内容，供大模型理解与日志记录
```

### 2. 跨品类实战范例

#### A. 竞速拉力（ACR / RBR / 地平线）
```yaml
# profiles/assetto-corsa.yaml
rules:
  # 严重失控打滑（优先级 80，冷却 10 秒）
  - name: radio-tire-slip
    when: "speed > 20 && (slip > 1.2 || max(slip_fl, slip_fr) > 1.5)"
    target: radio
    priority: 80
    clip: "tire_slip"
    cooldown: 10s
    text: "注意，后轮严重打滑，稳住方向！"

  # 持续红线超转（高转持续 1.2 秒才报警）
  - name: radio-engine-redline
    when: "custom0 > 7200"
    target: radio
    priority: 70
    clip: "engine_redline"
    hold: 1.2s
    cooldown: 14s
    text: "转速逼近红线，立即升挡！"

  # 飞坡与严重撞击（最高优先级 100 抢占）
  - name: radio-heavy-crash
    when: "speed > 15 && absf(accel_g) > 0.45"
    target: radio
    priority: 100
    clip: "heavy_crash"
    cooldown: 15s
    text: "收到严重撞击，检查车身受损！"
```

#### B. 地牢肉鸽（《以撒的结合：忏悔》）
```yaml
# profiles/isaac.yaml
rules:
  # 濒死血量告警（血量剩一颗心以下）
  - name: speak-isaac-low-hp
    when: "has('hp') && hp > 0 && hp <= 1.0"
    target: speak
    priority: 85
    clip: "low_hp_warning"
    cooldown: 8s
    text: "生命值垂危，快找红心！"

  # Boss 房强敌侦测
  - name: speak-boss-intro
    when: "hazard >= 1.0"
    target: speak
    priority: 75
    clip: "boss_intro"
    cooldown: 20s
    text: "侦测到强敌反应，准备迎战！"

  # 按键炸弹放置语音
  - name: speak-bomb-placed
    when: 'pressed("square")'
    target: speak
    priority: 40
    clip: "bomb_placed"
    cooldown: 2s
```

#### C. 动作魂系（《艾尔登法环》/《黑神话：悟空》）
```yaml
# profiles/souls.yaml
rules:
  # 精力见底喘息（防止误报已加 has('stamina') 防护）
  - name: speak-stamina-low
    when: "has('stamina') && stamina <= 0.15 && stamina >= 0.0"
    target: speak
    priority: 60
    clip: "stamina_low"
    cooldown: 6s
    text: "精力枯竭，退后重整防线..."

  # 处决破绽警报
  - name: speak-fatal-blow
    when: "hazard > 0.8"
    target: speak
    priority: 95
    clip: "fatal_blow"
    cooldown: 12s
    text: "致命破绽！"
```

---

## 三、 声学 DSP 流水线 (`pipeline.go`)

本项目提供 4 种专业的数字信号处理（DSP）声学风格渲染器，使用 Direct-Form-1 RBJ 双二阶滤波器（Biquad）实时处理：

| 声学风格 | 对应场景 | 声学处理链路 | 听感特征 |
|---|---|---|---|
| **`radio`** | 赛车、拉力、现代战术射击 | 75ms PTT 开启嘀声 ＋ 350~3200Hz 级联带通滤波 ＋ 非线性 Tanh 模拟饱和 ＋ 60ms 结束嘀声 | 逼真对讲机与维修区无线电杂音 |
| **`studio`** | 以撒肉鸽旁白、剧情角色台词 | 48kHz 无损透明 PCM ＋ 动态峰值软限幅 | 清晰透彻的高保真无损近场原声 |
| **`mech`** | 科幻机甲、外骨骼动力装甲 AI | 105Hz 正弦载波环形调制 ＋ 5.5ms 金属梳状反馈延迟 | 带有金属回响的战术机载电脑声 |
| **`whisper`** | 魂系指头巫女、以撒恶魔天使低语 | 250Hz 低频去共振高通 ＋ 4.5kHz 空气感棚架提升 ＋ 向上动态扩展 | 空灵、神秘的耳边低语 |

---

## 四、 本地端侧小模型（Qwen2.5-0.5B）编译、部署与运行

为了在玩家全屏打游戏时不占用游戏显卡的显存，系统采用极轻量端侧语言模型 **Qwen2.5-0.5B-Instruct**（显存/内存开销 < 400MB，CPU 占用 < 3%，生成耗时仅需 **70~90 毫秒**）。

### 1. 模型与引擎目录规范
模型与原生引擎自包含在项目根目录的 `models/` 目录下（受 `.gitignore` 保护）：
```text
models/
├── bin/
│   ├── llama-server    # 高性能 C/C++ 推理服务程序
│   ├── llama-cli       # 离线命令行单次推理工具
│   └── *.so            # 动态加速链接库（Zen4/AVX2/x64）
└── qwen/
    └── qwen2.5-0.5b-instruct-q4_k_m.gguf  # 4-bit 量化模型文件（469MB）
```

### 2. 模型下载指引
可以使用国内镜像站或官方 Hugging Face 极速下载：

```bash
mkdir -p models/qwen

# 方式 A：通过 ModelScope 极速下载（国内推荐，平均 15MB/s）
curl -L -o models/qwen/qwen2.5-0.5b-instruct-q4_k_m.gguf \
  "https://modelscope.cn/models/qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/master/qwen2.5-0.5b-instruct-q4_k_m.gguf"

# 方式 B：通过 HuggingFace 镜像站下载
curl -L -o models/qwen/qwen2.5-0.5b-instruct-q4_k_m.gguf \
  "https://hf-mirror.com/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"
```

### 3. 本地引擎管理 (`scripts/start-llm.sh`)
项目在 `scripts/start-llm.sh` 中提供了全套运维命令，支持基于 PID 文件的精准启停管理（绝不误杀系统内其他进程）：

```bash
# 启动本地模型推理服务（专用端口 9995）
./scripts/start-llm.sh start

# 检查运行状态与健康检查
./scripts/start-llm.sh status

# 运行单次微秒级推理测试
./scripts/start-llm.sh test

# 停止服务
./scripts/start-llm.sh stop
```

### 4. 端口规划与零硬编码设计
系统严格规划内部端口，杜绝 `8080` 冲突：
- `9999`：游戏 UDP 桥（Assetto Corsa / ACC / ACR）
- `9998`：IPC 力反馈镜像总线（`FFB1`）
- `9997`：IPC 遥测广播总线（`SIG1`）
- **`9995`：DualSense 本地伴生小模型推理服务专用端口**

**三级动态解析体系（`ResolveLLMEndpoint`）**：
1. 命令行参数优先：`ds5radio -llm-endpoint "..."`
2. 环境变量覆盖：`export DS5_LLM_ENDPOINT="http://127.0.0.1:9995/completion"`
3. 缺省安全兜底：默认定向到 `http://127.0.0.1:9995/completion`，底层 HTTP 传输显式避开系统 HTTP 代理劫持（`Proxy: nil`），确保回环请求 100% 直通。

---

## 五、 接入运行与联动使用

### 1. 启动守护进程与伴生广播器
在终端中同时启动守护进程与伴生广播器：

```bash
# 终端 1：启动 ds5d 守护进程（挂载 IPC 遥测与语音广播）
./bin/ds5d -profile assetto-corsa -rate 125

# 终端 2：启动伴生广播器并挂载本地 Qwen2.5-0.5B 推理引擎与人设
./bin/ds5radio -llm -persona rally listen   # WRC 拉力赛车领航员
./bin/ds5radio -llm -persona f1 listen      # F1 车队维修区工程师
./bin/ds5radio -llm -persona isaac listen   # 《以撒的结合》暗黑哥特旁白
./bin/ds5radio -llm -persona souls listen   # 魂系（法环）指头巫女
./bin/ds5radio -llm -persona mech listen    # 科幻战术机甲装甲 AI
```

### 2. 手柄扬声器音量调优
由于 DualSense 内置微型扬声器灵敏度特性，推荐音量设定在 **85%** 档位（既清晰可辨，又绝不破音爆音）：
```bash
# 设置 DualSense 扬声器音量为最佳 85%
pactl set-sink-volume alsa_output.usb-Sony_Interactive_Entertainment_DualSense_Wireless_Controller-00.Direct__Direct__sink 85%
```
