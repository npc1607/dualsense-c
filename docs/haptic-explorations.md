# DualSense 触觉人机交互前沿探索与高级规划方案

## 一、方案背景与定位

dualsense-c 的架构本质是一个**高实时性（125~250Hz）、声明式可编程的物理力觉与触觉渲染引擎（Programmable Haptic & Kinesthetic Engine）**。

依托 PS5 DualSense 独特的硬件素质：
- **高精度输入**：0..255 模拟行程扳机、1920×1080 电容触摸板、6 轴高采样率 IMU（陀螺仪与加速度计）；
- **高精度输出**：双通道 Voice-Coil 音圈致动器（支持 48kHz 任意波形）、双微型步进马达（8 种模式自适应阻尼与挡墙）、独立 RGB 与指示灯。

本方案旨在跨出常规的简单遥测映射，系统规划具有突破性的触觉人机交互能力，涵盖：**触控板虚拟机械齿轮刻度感**、**空间声学触觉雷达**、**武器故障排障模拟**、**视觉多模态环境感知**、**直驱方向盘力矩降维模拟**、**手感录音机** 与 **双人手柄力矩纠缠**，以及面向生态接入与工具链的扩展（**DSX 协议兼容**、**SimHub 接入**、**内存读取**、**手柄扬声器赛车工程师**、**屏幕取色 / OCR 信号源**、**LLM 生成 Profile**、**Web 调参面板**）。

---

## 目录

- [一、方案背景与定位](#一方案背景与定位)
- [二、触控板虚拟机械齿轮刻度感 (Virtual Detents)](#二触控板虚拟机械齿轮刻度感-virtual-detents)
- [三、游戏视听感官升维（方向二）](#三游戏视听感官升维方向二)
  - [1. 空间声学触觉雷达 (Spatial Audio-to-Haptic Radar)](#1-空间声学触觉雷达-spatial-audio-to-haptic-radar)
  - [2. 枪械机械卡壳与战术排障模拟 (Gun Jamming Simulator)](#2-枪械机械卡壳与战术排障模拟-gun-jamming-simulator)
- [四、前沿多模态与环境物理模拟（方向三）](#四前沿多模态与环境物理模拟方向三)
  - [1. 基于本地轻量 VLM 的环境气候感知 (Ambient Climate Haptics)](#1-基于本地轻量-vlm-的环境气候感知-ambient-climate-haptics)
  - [2. 赛车直驱模拟器力矩降维模拟 (Direct Drive Wheel Emulator)](#2-赛车直驱模拟器力矩降维模拟-direct-drive-wheel-emulator)
- [五、生态与社交交互机制（方向四）](#五生态与社交交互机制方向四)
  - [1. 手感黑胶唱片与录制回放系统 (Haptic Recorder & Replay)](#1-手感黑胶唱片与录制回放系统-haptic-recorder--replay)
  - [2. 双人手柄力矩纠缠 (Haptic Tether / Duo Interaction)](#2-双人手柄力矩纠缠-haptic-tether--duo-interaction)
- [六、生态接入与工具链扩展（方向五）](#六生态接入与工具链扩展方向五)
  - [1. DSX 协议兼容 (DualSenseX Mod Compatibility)](#1-dsx-协议兼容-dualsensex-mod-compatibility)
  - [2. SimHub 遥测接入 (SimHub Bridge)](#2-simhub-遥测接入-simhub-bridge)
  - [3. 单机游戏内存读取信号源 (Memory Reader Source)](#3-单机游戏内存读取信号源-memory-reader-source)
  - [4. 手柄扬声器赛车工程师 (Race Engineer on Controller Speaker)](#4-手柄扬声器赛车工程师-race-engineer-on-controller-speaker)
  - [5. 屏幕取色 / OCR 信号源 (Screen Probe Source)](#5-屏幕取色--ocr-信号源-screen-probe-source)
  - [6. LLM 辅助生成 Profile (Profile Copilot)](#6-llm-辅助生成-profile-profile-copilot)
  - [7. Web 实时调参面板 (Live Tuning Dashboard)](#7-web-实时调参面板-live-tuning-dashboard)
- [七、工程实施路线与优先级评估矩阵](#七工程实施路线与优先级评估矩阵)

---

## 二、触控板虚拟机械齿轮刻度感 (Virtual Detents)

### 1. 物理原理：触觉神经错觉 (Haptic Illusion)
DualSense 顶部的电容触控板表面是一块绝对平整光滑的哑光塑料/玻璃，物理上没有任何机械活动部件。
然而，当手指在表面滑动时，如果在手指跨过固定坐标间隔的微秒瞬间，由音圈马达（Voice-Coil）下发一个 **高阻尼、单周期、脉宽 5~8ms 的非对称瞬态脉冲（Impulse）**，人脑的触觉神经末梢（梅斯纳小体）会产生强烈的空间错觉：**误认为手指刚刚拨动了一颗真实的物理机械棘轮齿（Detent）**。

```
手指平滑划动轨迹 ────────► [坐标微分计算 dx, dy] ────► 步进阈值判定
                                                       │ 越过刻度点
                                                       ▼
音圈马达 (Ch3/Ch4) ◄── 48kHz 单周期瞬态脉冲 (5ms) ◄── 触发瞬态冲击
                       (产生清脆咔哒机械错觉)
```

### 2. 核心应用场景
1. **赛车序列挡换挡旋钮**：在触控板上下滑动，手指每滑过 15% 距离，手掌感受到一次明显的机械卡顿，并完成一次模拟升挡/降挡；
2. **射击游戏狙击镜倍率步进调节**：手指在触控板横向滑移，呈现 4x / 8x / 16x 逐级跳动的段落阻尼；
3. **武器轮盘选择器**：顺时针/逆时针划圈，精准模拟带有“滚珠定位槽”的特种转盘手感；
4. **视频剪辑关键帧吸附**：在视频时间轴滑动，吸附到转场或标记点时产生磁吸般的阻滞反馈。

### 3. 控制算法实现规范
```go
// 触控板步进刻度检测核心伪代码
type DetentController struct {
    stepInterval float32 // 刻度间隔，如 0.1 (全行程 10 个物理刻度)
    lastStep     int
}

func (d *DetentController) Update(touchX, touchY float32, active bool) bool {
    if !active {
        return false
    }
    currentStep := int(touchX / d.stepInterval)
    if currentStep != d.lastStep {
        d.lastStep = currentStep
        return true // 触发音圈发送 5ms 180Hz 冲击波形
    }
    return false
}
```

---

## 三、游戏视听感官升维（方向二）

### 1. 空间声学触觉雷达 (Spatial Audio-to-Haptic Radar)
- **技术机制**：从操作系统环回捕获游戏输出的 7.1 声道环绕声音频流（Left, Right, Center, LFE, Left Surround, Right Surround, Left Rear, Right Rear）；
- **空间触觉映射**：
  - **后方盲区威胁感知**：当 Left Rear / Right Rear 声道出现高频脚步或开火声时，系统不发声，而是直接将音频包络提取为对应侧音圈马达的指向性触感；
  - **左右侧向破空预警**：子弹划过侧方，对应侧扳机瞬间产生一记微小的颤动断点；
- **核心价值**：为听障玩家提供前所未有的全方位空间态势感知，为硬核竞技玩家打造物理维度的“第六感”。

### 2. 枪械机械卡壳与战术排障模拟 (Gun Jamming Simulator)
- **技术机制**：结合开火频率、连续扫射时序或游戏内特定的装弹事件流；
- **物理表现**：
  - 在武器过热或触发卡壳时，R2 开火扳机在 15% 的初始行程处被硬件**强行硬性锁死（`rigid force: 255`）**，玩家手指无论如何用力扣压也无法越过；
  - **战术排障动作**：玩家必须拍击手柄侧面或快速上扬甩动手柄（由内置 6 轴加速度计检测到特征瞬态冲激），扳机步进马达才伴随一声清脆的“咔哒”机械复位声解除锁定，真实还原真实枪械拉栓排哑弹的肌肉记忆。

---

## 四、前沿多模态与环境物理模拟（方向三）

### 1. 基于本地轻量 VLM 的环境气候感知 (Ambient Climate Haptics)
- **技术机制**：由伴生守护工具以 1 FPS 的极低频率抓取游戏画面，通过本地极轻量级视觉模型（如 FastViT / MobileVLM INT8，纯 CPU 运行，显卡零开销）识别当前画面的环境气象属性：
- **物理手感表现**：
  - **极寒暴雪**：自适应扳机阻尼变重变钝（增加回弹迟滞时间 `slew_down`），模拟低温环境下机械连杆润滑脂冻结的机械黏滞感；
  - **熔岩地热**：左右音圈马达持续输出 35Hz 低频热浪热动波纹，灯条呈现灼热深红呼吸；
  - **暴雨滂沱**：结合前述音频 AI 识别，手心呈现细密密集的水珠敲击感。

### 2. 赛车直驱模拟器力矩降维模拟 (Direct Drive Wheel Emulator)

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/direct-drive-curves.svg" alt="赛车直驱模拟器 FFB 力矩降维与抓地力极限曲线" width="100%">
</p>
<p align="center">
  <sub>（注：架构图源文件见 <a href="https://github.com/npc1607/dualsense-c/blob/main/docs/direct-drive-curves.excalidraw">docs/direct-drive-curves.excalidraw</a>，支持在 <a href="https://excalidraw.com">Excalidraw</a> 中导入编辑）</sub>
</p>

- **技术机制**：专业级赛车模拟器（iRacing, Assetto Corsa, rFactor 2）在输出遥测时包含昂贵直驱方向盘（Direct Drive）专用的物理对齐力矩（Aligning Torque / FFB Torque）；
- **物理手感表现**：
  - 将力矩物理曲线通过非线性算法映射为 **L2 转向阻力（模拟方向盘受阻）** 与 **左右音圈高频路面颗粒（模拟轮胎抓地力边界）**；
  - 进弯前载荷转移导致手柄手感加重，一旦推头（Understeer）抓地力丧失，扳机阻力瞬间松脱落空，让掌上控制器具备专业直驱方向盘的核心路感反馈。
- **自定义曲线支持**：
  - 规则引擎原生内置 `curve(x, pts...)`（多点折线查表插值）与 `scurve(x, power)`（软饱和 S 曲线），车手可在 YAML Profile 中自由定制拐点与阻尼渐进率。

---

## 五、生态与社交交互机制（方向四）

### 1. 手感黑胶唱片与录制回放系统 (Haptic Recorder & Replay)
- **功能定义**：
  - `ds5d record -out custom-shotgun.ds5rec`：以 250Hz 的控制精度录制真实操作时的扳机行程、阻力曲线、马达调幅参数与音圈时序；
  - `ds5d play -file custom-shotgun.ds5rec`：无损高保真复现录制好的手感；
- **社区分发**：支持将调校好的手感导出为自包含的标准 YAML 包，开发者与竞技玩家可一键分享神级手感预设。

### 2. 双人手柄力矩纠缠 (Haptic Tether / Duo Interaction)
- **技术机制**：通过局域网 UDP 广播在两台计算机/控制器之间建立对称的力学镜像信道；
- **物理交互场景**：
  - **双人拔河与牵引**：玩家 A 按下扳机的深度直接转化为玩家 B 扳机的反向阻力，双方产生物理对抗；
  - **协同驾驶/副驾领航**：主驾踩下刹车时，副驾手柄同步感知到踏板行程与阻尼，创造全新的社交交互维度。

---

## 六、生态接入与工具链扩展（方向五）

前五个方向关注“手感能做到多好”，本方向关注“**能接入多少游戏、调校有多快**”。核心思路：复用已有的第三方生态与通用信号获取手段，让 Source 层覆盖面最大化，同时降低 Profile 编写门槛。

### 1. DSX 协议兼容 (DualSenseX Mod Compatibility)
- **背景**：大量社区 Mod（Cyberpunk 2077、Skyrim、GTA V、RDR2、Fallout 4 等）已按 DualSenseX（DSX）的 UDP 协议向本机发送扳机/灯条指令。DSX 本身仅支持 Windows 且闭源收费。
- **技术机制**：
  - 新增 `source/dsx`，监听 UDP（DSX 默认端口 `6969`），解析 JSON 指令包（`instructions[]`，含 `TriggerUpdate` / `RGBUpdate` / `PlayerLED` / `MicLED` / `ResetToUserSettings` 等类型）；
  - DSX 的扳机模式（`Rigid` / `Resistance` / `Bow` / `Galloping` / `SemiAutomaticGun` / `AutomaticGun` / `Machine` / `CustomTriggerValue` 等）映射到 `hid/effect.go` 已有效果；
  - 两种接入方式：**直通模式**（指令直接成为输出帧）与 **信号模式**（指令转为 `dsx_r2_mode` / `dsx_r2_force` 等信号，再经规则引擎二次加工、与其他 Source 叠加）；
  - 按需回复 `GetDSXStatus` 等状态查询，避免部分 Mod 因握手失败而静默。
- **待核实**：DSX v2 / v3 协议字段存在差异，实现前需以实际 Mod 抓包（`ds5ctl sniff -listen :6969`）为准。
- **价值**：一个 Source 即获得一整批已有 Mod 的支持，且原生运行于 Linux / Proton（Mod 在 Wine 内发出的 UDP 可直接到达宿主机）。

### 2. SimHub 遥测接入 (SimHub Bridge)
- **背景**：SimHub 是 Windows 端事实标准的遥测聚合器，已适配数百款赛车/飞行/卡车游戏，并提供统一的属性名（`SpeedKmh`、`Rpms`、`Brake`、`ABSActive`、`TCActive`、`TyreSlip` 等）。
- **技术机制**：
  - 方案 A：编写极小的 SimHub C# 插件，按固定频率把选定属性序列化为 JSON / 二进制推送到 UDP，由 `source/udp` 新增 `simhub` decoder 解析；
  - 方案 B（零插件）：利用 SimHub「Custom Serial Devices」按模板输出字符串到虚拟串口，`ds5d` 侧读取解析；
  - 字段映射写在 Profile 中，SimHub 属性名 → 标准信号名（见 `internal/signal/names.go`）。
- **限制**：SimHub 以 Windows 为主，Linux 下在 Wine 内运行的兼容性需实测；适合作为 Windows 平台的“万能遥测兜底”。
- **价值**：避免逐游戏编写 decoder，长尾游戏一次性覆盖。

### 3. 单机游戏内存读取信号源 (Memory Reader Source)
- **技术机制**：
  - 新增 `source/memory`，在 YAML 中声明进程名、模块名、指针链 / 特征码（AOB）与数据类型，周期性读取血量、弹药、载具速度等数值并转为信号；
  - Linux：Proton 游戏本质是宿主机进程，通过 `/proc/<pid>/maps` 定位 exe 模块基址，用 `process_vm_readv` 读取（需 `CAP_SYS_PTRACE` 或调整 `ptrace_scope`）；
  - Windows：`OpenProcess` + `ReadProcessMemory`；
  - 可导入 Cheat Engine 社区表（`.CT`）中的指针链，复用现成的逆向成果。
- **安全边界**：**只读、仅限单机 / 离线游戏**。对带 EAC / BattlEye / Vanguard 等反作弊的联网游戏默认拒绝启用（进程名黑名单 + 显式确认开关），避免封号风险。
- **价值**：为没有遥测、也没有 Mod 的单机老游戏提供精确信号（优于音频 / 画面推断）。

### 4. 手柄扬声器赛车工程师 (Race Engineer on Controller Speaker)
- **背景**：DualSense 内置扬声器在本项目中尚未使用。USB 模式下手柄声卡为 4 声道：Ch1/2 为扬声器 / 耳机，Ch3/4 为音圈（`sink/haptics` 已占用）。
- **技术机制**：
  - 新增规则 target `speak`（或独立 `sink/voice`），表达式满足时触发一条语音播报，带**边沿触发 + 冷却时间 + 优先级打断**，避免刷屏；
  - 本地 TTS（Piper / espeak-ng），常用短语可预渲染缓存为 PCM，触发延迟可控制在百毫秒内；
  - 与音圈共用同一 4 声道 PCM 流：语音写 Ch1/2，触感写 Ch3/4，互不干扰。
- **示例场景**：
  - 赛车：`tyre_temp_max > 0.9` → “前轮过热”；`fuel_laps < 2` → “油量剩两圈”；进站窗口提示；
  - 射击：弹匣余量过低时手柄低声提示“换弹”。
- **限制**：蓝牙模式下手柄音频通道在 Linux 不可用，本功能仅限 USB 有线。
- **价值**：声音从手中发出，代入感强、演示效果出众，且完全复用现有信号与规则引擎。

### 5. 屏幕取色 / OCR 信号源 (Screen Probe Source)
- **技术机制**：
  - 伴生进程以 10~30 FPS 截取屏幕上 YAML 声明的若干小矩形区域（如血条、弹药数字、氮气条），计算：
    - **颜色占比**：区域内满足 HSV 阈值的像素比例 → 归一化信号（如 `health = 0.35`）；
    - **OCR 数字**：小区域数字识别（Tesseract / 轻量 PaddleOCR）→ 弹药、速度等数值；
  - 截屏后端：Linux X11 用 XShm；Wayland 走 xdg-desktop-portal ScreenCast + PipeWire；Windows 用 DXGI Desktop Duplication；
  - 与 `audio_ai` 相同的 UDP 线协议回传 `ds5d`，主进程零依赖。
- **与其他方向的关系**：与“音频 AI 探测”互补（一个靠看、一个靠听）；其截屏管线可直接作为[本地 VLM 环境气候感知](#1-基于本地轻量-vlm-的环境气候感知-ambient-climate-haptics)的前置基础设施。
- **价值**：无遥测、无 Mod、不碰进程内存，对反作弊最友好的通用信号获取方式。

### 6. LLM 辅助生成 Profile (Profile Copilot)
- **技术机制**：
  - 输入：标准信号表（`internal/signal/names.go`）、内置 Profile 范例（`internal/profile/builtin/*.yaml`）、`ds5ctl sniff` 字段转储，以及用户一句话需求（如“刹车超过 80% 时 L2 模拟 ABS 抖动，转速接近红线时灯条变红”）；
  - 输出：完整 YAML Profile，自动经 `ds5ctl check -profile` 编译校验，失败则把错误回传模型迭代修正；
  - 可选接入本地模型（Ollama）或云端 API，命令形如 `ds5ctl gen -prompt "..." -out my.yaml`。
- **提示词（已落地）**：[`prompts/profile-generator.md`](../prompts/profile-generator.md)，覆盖游戏分类、遥测采集动作脚本、字段校准核对表、Schema 摘要、`check` + dry-run 校验闭环、震动与扳机合理性规范（不合格必须重新生成），以及体感反馈调参对照表。现阶段可直接粘贴给任意 LLM 使用，`ds5ctl gen` 后续只需把它作为 System Prompt 并自动执行校验命令即可。
- **价值**：把“读文档 → 写表达式 → 试错”压缩为一句话，显著降低新游戏适配门槛；启动期编译校验保证生成结果安全可控。

### 7. Web 实时调参面板 (Live Tuning Dashboard)
- **技术机制**：
  - `ds5d -http 127.0.0.1:8765` 启用内嵌 HTTP 服务（`net/http` + `embed` 静态页面，维持纯 Go、零 CGO）；
  - 通过 SSE / WebSocket 以 30Hz 推送信号值、规则命中状态、最终输出帧（扳机模式与力度、马达、灯条）；
  - 页面功能：信号实时波形、`curve()` 拐点拖拽编辑、扳机效果预览、Profile 编辑与**热重载**（需新增 Profile 原子替换能力，加载失败保持旧配置）；
  - 默认仅监听本机回环地址；远程（如手机调参）需显式开启并设置访问令牌。
- **价值**：调参从“改 YAML → 重启 → 体感”变成“边玩边拖曲线”，同时可作为 Profile 分享与社区展示的入口。

---

## 七、工程实施路线与优先级评估矩阵

| 模块名称 | 难度评级 | 软硬件环境要求 | 预估工时 | 推荐落地优先级 |
|---|---|---|---|---|
| **触控板虚拟机械齿轮刻度感** | 低 | 现有 DualSense 硬件即可 | 1 ~ 2 天 | **P1（极力推荐）** |
| **枪械机械卡壳与排障模拟** | 低 | 现有 DualSense 硬件即可 | 1 天 | **P1（高趣味性）** |
| **手感录音机与回放系统** | 中 | 纯软件 Go 逻辑扩展 | 3 ~ 4 天 | **P2（生态基建）** |
| **空间声学触觉雷达 (7.1声道)** | 中 | PipeWire / WASAPI 多声道 | 3 ~ 5 天 | **P2（功能性强）** |
| **直驱方向盘物理力矩降维** | 中 | 现有 UDP 遥测源 | 2 ~ 3 天 | **P3（硬核竞速）** |
| **双人手柄力矩纠缠网络** | 中 | 局域网 UDP 传输 | 3 ~ 4 天 | **P3（新颖社交）** |
| **本地 VLM 气候环境感知** | 高 | 本地 NPU / CPU ONNX | 1 ~ 2 周 | **P4（前瞻探索）** |
| **DSX 协议兼容** | 低 | 纯软件，UDP 监听 | 2 ~ 3 天 | **P1（生态杠杆最大）** |
| **手柄扬声器赛车工程师** | 低 | USB 有线 + 本地 TTS | 2 ~ 3 天 | **P1（演示效果强）** |
| **Web 实时调参面板** | 中 | 纯 Go 内嵌 HTTP | 4 ~ 6 天 | **P2（工具链基建）** |
| **LLM 辅助生成 Profile** | 中 | 本地 Ollama 或云端 API | 3 ~ 4 天 | **P2（降低门槛）** |
| **屏幕取色 / OCR 信号源** | 中 | X11 / Wayland Portal / DXGI | 1 周 | **P2（通用兜底）** |
| **单机游戏内存读取** | 中 | ptrace 权限 / ReadProcessMemory | 4 ~ 5 天 | **P3（仅限单机）** |
| **SimHub 遥测接入** | 中 | Windows + SimHub 插件 | 3 ~ 4 天 | **P3（Windows 兜底）** |
