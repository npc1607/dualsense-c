# FPS 辅助瞄准与手柄增强技术指南 (FPS Aim Assist & Input Augmentation)

> 本文档面向纯单机 FPS 游戏玩家与手柄算法开发者，详细解析**摇杆响应曲线**、**One Euro 滤波**、**陀螺仪体感微调 (Gyro Aiming)**、**弹道反冲抑制 (Anti-Recoil)** 的数学原理、工程实现，以及如何通过声明式 Profile 接入 `dualsense-c` 工具链。

---

## 1. 核心前置：游戏原生支持度与架构分流原则

在单机 FPS 中，必须根据游戏对 PS5 DualSense 的**原生支持程度**选择不同的架构策略，**切忌一刀切开启虚拟手柄**：

| 游戏类型 | 典型代表作品 | 游戏内置能力 | 推荐运行模式 | 工具链动作与策略 |
|---|---|---|---|---|
| **类型 A：原生支持 DualSense** | 《死亡回归 Returnal》<br>《最后生还者 Part 1》<br>《瑞奇与叮当》<br>《神秘海域 4》 | 官方自驱动自适应扳机、48kHz 音圈触感，且**设置菜单内置陀螺仪瞄准与曲线** | **完全直通让路 (Bypass)** | **不启动 `ds5vpad` 与 `ds5d`**。<br>直接 USB 原生连接，防止 HID 写入竞争与手柄双重输入冲突。 |
| **类型 B：传统/非原生 FPS** | 《DOOM 永恒》<br>《原子之心》<br>《生化危机 4/7/8》<br>《辐射 4》/《潜行者 2》 | 仅认最老式的 Xbox 360 (XInput)，无自适应扳机，无陀螺仪，摇杆死区大且呆板 | **全套接管增强 (Full Emulation)** | **启动 `ds5vpad` + `ds5d`**。<br>补齐 S 曲线、One Euro 滤波、L2 陀螺仪锁头、R2 二道火扳机与连发压枪。 |
| **类型 C：原生 DS5 输入接管** | 游戏原生支持 DualSense（有原生扳机/触觉），但**游戏内摇杆曲线呆板、无手抖滤波或缺少体感** | 游戏原生驱动自适应扳机与音圈，认 PS5 原生图标与按键 | **虚拟 DS5 代理 (Virtual DualSense Proxy)** | **手柄依然输出原生 DS5 协议**（保持游戏 PS5 UI 与原生扳机透传），但**摇杆曲线、One Euro 滤波、陀螺仪微调与压枪算法由本项目完全接管重塑**。 |

### 类型 C（原生 DS5 输入接管）的核心实现机制
很多玩家遇到的痛点是：游戏认 DualSense（有官方扳机阻尼和 PlayStation 图标），但游戏自带的视角灵敏度非常难用、没有防抖滤波、或者缺少 L2 举镜体感微调。

此时我们**不将手柄转为 Xbox，也不转为鼠标**，而是采用 **Virtual DualSense 代理架构**：
1. **输入上行（物理手柄 $\to$ 算法 $\to$ 游戏）**：
   - `ds5vpad` 独占抓取物理 DualSense；
   - 摇杆输入经过 **S 型曲线** 塑形，经 **One Euro 滤波** 剔除生理手抖；
   - 举镜时将物理陀螺仪数据融合到视角轴中；
   - 最终以 **Sony DualSense (VID 0x054C, PID 0x0CE6)** 的原生协议打包输出虚拟手柄。
   - **效果**：游戏以为自己连的是原生 PS5 手柄，屏幕显示 $\triangle / \bigcirc / \times / \square$ 图标，但摇杆和瞄准精度获得了质的飞跃！
2. **输出下行透传（游戏 $\to$ 物理手柄）**：
   - 游戏原生的自适应扳机指令与音圈触觉信号（HID Output Report 0x31）由代理层直接透明穿透（Passthrough）送达物理手柄，**完全保留游戏原汁原味的原生扳机阻尼与 HD 触感**。

---

## 2. 总体架构流水线

为了在手柄上获得媲美“鼠标+机械键盘”的单机击杀快感，我们在物理手柄（DualSense）与虚拟手柄（`ds5vpad`）之间构建了一条**纯输入驱动的零延迟信号增强流水线**：

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/fps-aim-assist-pipeline.svg" alt="FPS 辅助瞄准流水线架构" width="100%">
</p>

### 信号流转阶段说明
1. **物理采集 (Raw Input)**：从 DualSense HID 报告以 250Hz~500Hz 读取右摇杆原始位移 `(RX_raw, RY_raw)`、六轴陀螺仪角速度 `(Gyro_Yaw, Gyro_Pitch)` 及双扳机行程 `(L2, R2)`。
2. **动态自适应滤波 (Filter)**：进入 One Euro Filter 算法，低速过滤手抖，高速动态提高截断频率保持 0 延迟。
3. **响应曲线塑形 (Response Curve)**：将滤波后的摇杆位移映射为非线性视口角速度，压制中心死区抖动，强化后段大转身。
4. **陀螺仪体感融合 (Gyro Blending)**：当且仅当 `L2 > 0.15`（举镜瞄准）时，将手腕的高精度旋转角速度投影叠加到右摇杆轴上。
5. **弹道反冲抑制 (Anti-Recoil)**：当 `R2` 开火时，根据射击持续时间在 `RY` 轴持续叠加向下补偿向量与高斯散布。
6. **虚拟手柄下发 (Output)**：经 Linux `uinput` 或 Windows `ViGEm` 输出为标准的 DirectInput 或 XInput 手柄。

---

## 3. 摇杆响应曲线算法与实战效果

手柄右摇杆直接控制的是“准心转动的速度”，而非像鼠标一样的“绝对位移”。原始线性输入（Linear）不仅难以微调，还容易拉过头。

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/fps-aim-curves.svg" alt="四大摇杆算法曲线对比" width="100%">
</p>

### (1) 经典 S 型曲线 (Sigmoid / S-Curve) ⭐ 最推荐
将输入分为三个阶段：**微调爆头区（低斜率）** $\to$ **平稳巡航跟枪区（匀速）** $\to$ **极速转身区（陡峭）**。

#### 数学模型
设物理归一化摇杆偏离量为 `r ∈ [0, 1]`（`r = sqrt(x^2 + y^2)`），基础 Sigmoid 公式为：

```text
f(r) = 1 / (1 + e^(-k * (r - r0)))
```

为了保证零点无位移（`r=0` 时输出 `0`）且推满全速（`r=1` 时输出 `1`），做双端归一化：

```text
S(r) = ( f(r) - f(0) ) / ( f(1) - f(0) )
```

- **推荐参数**：`k = 6.0 ~ 8.0`（控制过渡段的陡峭程度），`r0 = 0.5 ~ 0.55`（中点居中偏后）。

#### 实战手感
- 摇杆轻微推动（`0% ~ 35%` 行程）：输出极其轻微，中远距离点射弱点时，准心不会因为手抖滑出敌人头部。
- 摇杆推满（`75% ~ 100%`）：斜率瞬间飙升，近身遇到刺客怪时，一瞬间就能完成 180° 甩头。

---

### (2) 指数幂次曲线 (Power Curve, `y = x^γ`)
传统 COD 与战地默认采用的手感模式。

#### 数学模型
```text
S(r) = r^γ   (γ 通常取 2.0 ~ 2.4)
```

#### 实战手感
- 小位移段被大幅抑制，瞄准稳定性高。
- 缺点是后段提速相对平缓，如果想要极快的大转身，通常需要配合“外圈加速（Outer Threshold Boost）”。

---

### (3) 双区阶跃曲线 (Dual-Zone)
《守望先锋》主机版特有模式。

#### 数学模型
```text
当 r < r_split 时:   S(r) = α * (r / r_split)
当 r >= r_split 时:  S(r) = α + (1 - α) * ((r - r_split) / (1 - r_split))
```
- **典型参数**：分割点 `r_split = 0.85`，内圈速度比 `α = 0.35`。

#### 实战手感
- 内圈 85% 行程是一个固定极慢的线性微调区，推到底最后 15% 突然爆发全速。适合肌肉记忆习惯恒定转速的玩家。

---

## 4. 高精度滤波算法（消除生理颤抖）

手柄摇杆与陀螺仪在极微小位移下，不可避免地会受到**人体手指生理震颤（8~12Hz 微颤）**与**传感器零漂噪声**的影响。

### (1) 指数移动平均滤波 (EMA)
最基础的低通滤波，计算量极小：
```text
y[t] = α * x[t] + (1 - α) * y[t-1]   (α ∈ (0, 1])
```
- **缺点**：固定 `α` 会导致权衡困境——`α` 小能滤除手抖，但高速拉枪时会带来 30~50ms 的肉眼可见严重拖尾延迟。

---

### (2) One Euro 滤波 (1€ Filter) ⭐ 工业金标准
1€ 滤波专为解决**人机交互输入中的“低速抖动与高速延迟”矛盾**而设计（广泛用于 VR 手柄追踪、眼动仪）：

#### 核心思想
**动态调整低通滤波器的截断频率（Cutoff Frequency `f_c`）**：
- **静止或微动时**：输入信号的变化率（导数速度 `dx/dt`）极低，`f_c` 自动下降到极小的 `min_cutoff`（如 0.8Hz），提供极强的平滑力度，准心纹丝不动。
- **快速甩枪时**：输入信号变化率极高，`f_c` 随速度呈线性增加，动态上限可以达到 60Hz 以上，相当于**滤波器瞬时透明**，延迟降至 0 毫秒！

#### 核心计算公式
1. **计算一阶差分速度（变化率）**：
   ```text
   dx = (x[t] - x[t-1]) / T_e
   ```
2. **对变化率低通平滑，得到当前速度 `dx_hat`**：
   ```text
   dx_hat = α_d * dx + (1 - α_d) * dx_hat[t-1]
   (其中 α_d = alpha(T_e, d_cutoff))
   ```
3. **动态计算自适应截断频率 `f_c`**：
   ```text
   f_c = min_cutoff + beta * |dx_hat|
   ```
4. **计算最终信号滤波输出**：
   ```text
   x_hat[t] = α * x[t] + (1 - α) * x_hat[t-1]
   (其中 α = alpha(T_e, f_c))
   ```

- **核心调优参数**：
  - `min_cutoff`（推荐 0.8 ~ 1.0）：越小，静止与微调时越稳定，但过小会有轻微粘滞感。
  - `beta`（推荐 0.04 ~ 0.05）：速度响应因子，越大，快速拉枪时恢复零延迟的速度越迅猛。

---

## 5. 陀螺仪体感微调 (Gyro Aiming)

陀螺仪体感不是用来代替右摇杆“大转身”的，而是充当手柄的**“激光微调鼠标”**。

### (1) 坐标系映射与空间转换
DualSense 的 6 轴 IMU 提供三个角速度：
- `Gyro_Yaw`（偏航角速度）：手柄水平左右转动 $\to$ 映射为视口水平偏转 `ΔRX`
- `Gyro_Pitch`（俯仰角速度）：手柄前后俯仰起伏 $\to$ 映射为视口上下俯仰 `ΔRY`
- `Gyro_Roll`（翻滚角速度）：手柄方向盘式倾斜

#### 自然握持倾角投影 (Natural Grip Angle)
玩家握持手柄时，手柄通常与水平桌面呈 30°~45° 的自然夹角。如果只取 Yaw，水平旋转手腕会串入 Roll。
我们引入自然倾角 `θ`（通常设为 35°）进行坐标投影：
```text
ΔRX = Gyro_Yaw * cos(35°) - Gyro_Roll * sin(35°)
ΔRY = Gyro_Pitch
```

### (2) L2 举镜门控机制 (Hold to Aim)
- **未按 L2 时**：陀螺仪完全被门控屏蔽，自由跑动、看景、拿道具不受任何体感干扰，绝不晕 3D。
- **按住 L2 举镜（`L2 > 0.15`）**：
  ```text
  RX_out = Curve(RX_stick) + Sens_x * ΔRX_gyro
  RY_out = Curve(RY_stick) + Sens_y * ΔRY_gyro
  ```
- **紧绷死区 (Tightening Threshold)**：
  当手腕角速度小于 `0.4°/s` 时忽略，防止自然呼吸或心跳脉动造成准心漂移。

---

## 6. 射击弹道反冲抑制 (Anti-Recoil)

单机 FPS（如《赛博朋克2077》、《孤岛惊魂》）中，自动武器连续开火会导致枪口剧烈上跳。

### (1) 物理模型
监听开火键（R2）：
- `R2 > 0.85` 时判定为连续开火状态，计时器 `t` 开始累加。
- 枪械反冲通常在**前 1~3 发发生剧烈骤跳**，随后进入**相对平稳的持续上跳**。
- 设计复合衰减抑制函数：
  ```text
  R(t) = A_burst * e^(-λ * t) + B_sustain
  ```
  - `A_burst * e^(-λ * t)`：前 150ms 施加较强的前段下压，压制枪口初跳。
  - `B_sustain`：持续连发时的恒定稳态下压。

### (2) 高斯微随机散布（防机械感僵硬）
完全恒定的下压会让画面显得呆板、失去枪械质感。我们在稳态下压上叠加一个零均值的高斯噪声：
```text
RY_final = RY + R(t) + GaussianNoise(0, σ)
```
模拟人手自然对抗后坐力时的细微弹性震颤，打击感更为生动拟真。

---

## 7. Profile 声明式配置示例 (`profiles/fps-aim.yaml`)

在 `dualsense-c` 中，我们将以上所有模块全部封装为可通过 YAML 自由开关、自由配置参数的声明式属性：

```yaml
name: fps-aim
title: 单机 FPS 辅助瞄准与体感增强
description: |
  包含 S 型自适应曲线、One Euro 手抖滤波、L2 举镜陀螺仪精瞄及 R2 弹道自适应压枪。

source:
  type: trigger
  rate_hz: 250

# ----------------------------------------------------
# 1. 摇杆辅助与曲线定义
# ----------------------------------------------------
aim_assist:
  # 摇杆响应曲线
  curve:
    type: sigmoid           # 可选: linear | power | sigmoid | dual_zone
    exponent: 6.5           # Sigmoid 斜率系数 (或 power 的 gamma 指数)
    mid_point: 0.52         # S 弯转折中心 (0.0 ~ 1.0)
    inner_deadzone: 0.04    # 物理摇杆硬件死区
    outer_boost: 1.5        # 推到底部时的加速倍率 (大转身)

  # 滤波抗抖算法
  filter:
    type: one_euro          # 可选: none | ema | one_euro
    min_cutoff: 0.85        # 低速截断频率 (Hz)，越低越防抖
    beta: 0.045             # 高速响应因子，越大越无延迟
    d_cutoff: 1.0           # 导数变化率平滑截断频率

  # 陀螺仪体感微调
  gyro:
    enabled: true
    trigger: l2_hold        # 触发方式: l2_hold (按住瞄准) | touch (轻触触摸板) | always
    trigger_threshold: 0.15 # L2 按下行程阈值
    natural_angle_deg: 35   # 自然手握倾角投影
    deadzone_dps: 0.4       # 紧绷死区 (度/秒)，滤除心跳微颤
    sens_yaw: 1.85          # 水平微调灵敏度
    sens_pitch: 1.35        # 垂直微调灵敏度
    invert_pitch: false

  # 自动反冲抑制 (压枪)
  anti_recoil:
    enabled: true
    trigger: r2_full        # 触发源 (当 R2 踩过 85% 连发时)
    start_delay: 60ms       # 击发后微小延迟
    burst_force: 420        # 前 3 发瞬间爆发下压力度
    burst_decay: 0.015      # 初始爆发衰减速率
    sustain_force: 180      # 持续连发稳态下压力度
    jitter_noise: 25        # 高斯随机散布震颤度 (防机械死板)

# ----------------------------------------------------
# 2. 原生自适应扳机阻尼与触觉反馈联动
# ----------------------------------------------------
rules:
  # L2 瞄准阶段：刚性段落感
  - name: l2-aim-wall
    when: "l2 > 0.05 && l2 < 0.65"
    target: l2
    effect: feedback
    params: { start: 0, strength: 160, force: 140 }

  # R2 开火扳机：二道火击发段落
  - name: r2-fire-break
    when: "r2 >= 0.85"
    target: r2
    effect: off
    priority: 20

  # 连发后坐力震颤
  - name: recoil-rumble
    when: 'pressed("r2_full")'
    target: rumble_right
    priority: 50
    hold: 120ms
    value: "220"
    modulate: { type: square, hz: 28, depth: 0.8 }
```

---

## 8. 如何接入现有工具使用

在现有的 `ds5vpad` 架构中，该模块作为纯输入前置增强拦截器：

```bash
# 启动增强虚拟手柄（加载 fps-aim 算法 profile，输出 Xbox 360 手柄）
./bin/ds5vpad -proto xinput -profile profiles/fps-aim.yaml &

# 启动触觉与灯条反馈服务
./bin/ds5d -profile profiles/fps-aim.yaml &
```

进入任意单机 FPS 游戏后：
1. 游戏直接识别为标准 Xbox 360 手柄。
2. 平时只用右摇杆瞄准与观察。
3. 遇到敌人时按住 **L2** 举镜，陀螺仪瞬间激活，手腕微晃即可实现爆头微调；按下 **R2** 自动抑制枪口上跳，享受极具科技感与机械击发质感的单机打枪体验！
