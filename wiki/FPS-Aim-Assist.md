# FPS 辅助瞄准与手柄增强技术指南 (FPS Aim Assist & Input Augmentation)

> 本文档面向纯单机 FPS 游戏玩家与手柄算法开发者，详细解析**摇杆响应曲线**、**One Euro 滤波**、**陀螺仪体感微调 (Gyro Aiming)**、**弹道反冲抑制 (Anti-Recoil)** 的数学原理、工程实现，以及如何通过声明式 Profile 接入 `dualsense-c` 工具链。

---

## 1. 总体架构流水线

为了在手柄上获得媲美“鼠标+机械键盘”的单机击杀快感，我们在物理手柄（DualSense）与虚拟手柄（`ds5vpad`）之间构建了一条**纯输入驱动的零延迟信号增强流水线**：

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/fps-aim-assist-pipeline.svg" alt="FPS 辅助瞄准流水线架构" width="100%">
</p>

### 信号流转阶段说明
1. **物理采集 (Raw Input)**：从 DualSense HID 报告以 250Hz~500Hz 读取右摇杆原始位移 $(RX_{raw}, RY_{raw})$、六轴陀螺仪角速度 $(G_{yaw}, G_{pitch})$ 及双扳机行程 $(L2, R2)$。
2. **动态自适应滤波 (Filter)**：进入 One Euro Filter 算法，低速过滤手抖，高速动态提高截断频率保持 0 延迟。
3. **响应曲线塑形 (Response Curve)**：将滤波后的摇杆位移映射为非线性视口角速度，压制中心死区抖动，强化后段大转身。
4. **陀螺仪体感融合 (Gyro Blending)**：当且仅当 $L2 > 0.15$（举镜瞄准）时，将手腕的高精度旋转角速度投影叠加到右摇杆轴上。
5. **弹道反冲抑制 (Anti-Recoil)**：当 $R2$ 开火时，根据射击持续时间在 $RY$ 轴持续叠加向下补偿向量与高斯散布。
6. **虚拟手柄下发 (Output)**：经 Linux `uinput` 或 Windows `ViGEm` 输出为标准的 DirectInput 或 XInput 手柄。

---

## 2. 摇杆响应曲线算法与实战效果

手柄右摇杆直接控制的是“准心转动的速度”，而非像鼠标一样的“绝对位移”。原始线性输入（Linear）不仅难以微调，还容易拉过头。

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/fps-aim-curves.svg" alt="四大摇杆算法曲线对比" width="100%">
</p>

### (1) 经典 S 型曲线 (Sigmoid / S-Curve) ⭐ 最推荐
将输入分为三个阶段：**微调爆头区（低斜率）** $\to$ **平稳巡航跟枪区（匀速）** $\to$ **极速转身区（陡峭）**。

#### 数学模型
设物理归一化摇杆偏离量为 $r \in [0, 1]$（$r = \sqrt{x^2 + y^2}$），Sigmoid 映射公式为：
$$f(r) = \frac{1}{1 + e^{-k \cdot (r - r_0)}}$$
为了保证当 $r=0$ 时输出必须为 $0$，$r=1$ 时输出必须为 $1$，做双端零点归一化：
$$S(r) = \frac{f(r) - f(0)}{f(1) - f(0)}$$
*典型参数*：$k = 6.0 \sim 8.0$（控制中间过渡段的陡峭程度），$r_0 = 0.5 \sim 0.6$（转折点居中偏后）。

#### 实战手感
* 摇杆轻微推动（$0\% \sim 35\%$ 行程）：输出极其轻微，中远距离点射弱点时，准心不会因为手抖滑出敌人头部。
* 摇杆推满（$75\% \sim 100\%$）：斜率瞬间飙升，近身遇到刺客怪时，一瞬间就能完成 180° 甩头。

---

### (2) 指数幂次曲线 (Power Curve, $y = x^\gamma$)
传统 COD 与战地默认采用的手感模式。

#### 数学模型
$$S(r) = r^\gamma \quad (\gamma \in [1.8, 2.5])$$
*典型参数*：$\gamma = 2.2$。

#### 实战手感
* 小位移段被大幅抑制，瞄准稳定性高。
* 缺点是后段提速相对平缓，如果想要极快的大转身，通常需要配合“外圈加速（Outer Threshold Boost）”。

---

### (3) 双区阶跃曲线 (Dual-Zone)
《守望先锋》主机版特有模式。

#### 数学模型
$$S(r) = \begin{cases} 
\alpha \cdot \frac{r}{r_{split}} & (r < r_{split}) \\
\alpha + (1 - \alpha) \cdot \frac{r - r_{split}}{1 - r_{split}} & (r \ge r_{split}) 
\end{cases}$$
*典型参数*：分割点 $r_{split} = 0.85$，内圈缩放比例 $\alpha = 0.35$。

#### 实战手感
* 内圈 85% 行程是一个固定极慢的线性微调区，推到底最后 15% 突然爆发全速。适合肌肉记忆习惯恒定转速的玩家。

---

## 3. 高精度滤波算法（消除生理颤抖）

手柄摇杆与陀螺仪在极微小位移下，不可避免地会受到**人体手指生理震颤（8~12Hz 微颤）**与**传感器零漂噪声**的影响。

### (1) 指数移动平均滤波 (EMA)
最基础的低通滤波，计算量极小：
$$y_t = \alpha \cdot x_t + (1 - \alpha) \cdot y_{t-1} \quad (\alpha \in (0, 1])$$
*缺点*：固定 $\alpha$ 会导致权衡困境——$\alpha$ 小能滤除手抖，但高速拉枪时会带来 30~50ms 的肉眼可见严重拖尾延迟。

---

### (2) One Euro 滤波 (1€ Filter) ⭐ 工业金标准
1€ 滤波专为解决**人机交互输入中的“低速抖动与高速延迟”矛盾**而设计（广泛用于 VR 手柄追踪、眼动仪）：

#### 核心思想
**动态调整低通滤波器的截断频率（Cutoff Frequency $f_c$）**：
* **静止或微动时**：输入信号的变化率（导数 $\dot{x}$）很低，$f_c$ 自动下降到极小的 `min_cutoff`（如 0.8Hz），提供极强的滤波力度，准心纹丝不动。
* **快速甩枪时**：输入信号变化率极高，$f_c$ 随速度呈线性增加，动态上限可以达到 60Hz 以上，相当于**滤波器瞬时透明**，延迟降至 0 毫秒！

#### 核心计算公式
1. 计算信号一阶差分速度（变化率）：
   $$dx_t = \frac{x_t - x_{t-1}}{T_e}$$
2. 对变化率进行低通滤波，得到平滑速度 $\dot{x}_t$：
   $$\dot{x}_t = \alpha_d \cdot dx_t + (1 - \alpha_d) \cdot \dot{x}_{t-1} \quad (\alpha_d = \text{alpha}(T_e, f_{c,d}))$$
3. 动态自适应截断频率：
   $$f_c = \text{min\_cutoff} + \beta \cdot |\dot{x}_t|$$
4. 计算最终输出滤波系数并平滑原始信号：
   $$\hat{x}_t = \alpha \cdot x_t + (1 - \alpha) \cdot \hat{x}_{t-1} \quad (\alpha = \text{alpha}(T_e, f_c))$$

*核心调优参数*：
- `min_cutoff`（默认 0.8）：越小，静止与微调时越稳定，但过小会导致起步粘滞感。
- `beta`（默认 0.04）：速度响应因子，越大，快速拉枪时恢复零延迟的速度越迅猛。

---

## 4. 陀螺仪体感微调 (Gyro Aiming)

陀螺仪体感不是用来代替右摇杆“大转身”的，而是充当手柄的**“激光微调鼠标”**。

### (1) 坐标系映射与空间转换
DualSense 的 6 轴 IMU 提供三个角速度：
* $\omega_{yaw}$（偏航角速度）：手柄水平左右转动 $\to$ 映射为视口水平偏转 $\Delta RX$
* $\omega_{pitch}$（俯仰角速度）：手柄前后俯仰起伏 $\to$ 映射为视口上下俯仰 $\Delta RY$
* $\omega_{roll}$（翻滚角速度）：手柄方向盘式倾斜

#### 自然握持倾角投影 (Natural Grip Angle)
玩家握持手柄时，手柄通常与水平桌面呈 30°~45° 的自然夹角。如果只取 Yaw，水平旋转手腕会串入 Roll。
我们引入自然倾角 $\theta$（通常设为 35°）进行坐标投影：
$$\Delta RX = \omega_{yaw} \cdot \cos(\theta) - \omega_{roll} \cdot \sin(\theta)$$
$$\Delta RY = \omega_{pitch}$$

### (2) L2 举镜门控机制 (Hold to Aim)
* **未按 L2 时**：陀螺仪完全被门控屏蔽，自由跑动、看景、拿道具不受任何体感干扰，绝不晕 3D。
* **按住 L2 举镜（$L2 > 0.15$）**：
  $$\begin{cases}
  RX_{out} = \text{Curve}(RX_{stick}) + \text{Sens}_x \cdot \Delta RX_{gyro} \\
  RY_{out} = \text{Curve}(RY_{stick}) + \text{Sens}_y \cdot \Delta RY_{gyro}
  \end{cases}$$
* **紧绷死区 (Tightening Threshold)**：
  当手腕角速度小于 $0.35^\circ/\text{s}$ 时忽略，防止自然呼吸或心跳脉动造成准心漂移。

---

## 5. 射击弹道反冲抑制 (Anti-Recoil)

单机 FPS（如《赛博朋克2077》、《孤岛惊魂》）中，自动武器连续开火会导致枪口剧烈上跳。

### (1) 物理模型
监听开火键（R2）：
* $R2 > 0.85$ 时判定为连续开火状态，计时器 $t$ 开始累加。
* 枪械反冲通常在**前 1~3 发发生剧烈骤跳**，随后进入**相对平稳的持续上跳**。
* 设计复合衰减抑制函数：
  $$R(t) = A_{burst} \cdot e^{-\lambda t} + B_{sustain}$$
  * $A_{burst} \cdot e^{-\lambda t}$：前 150ms 施加较强的前段下压，压制枪口初跳。
  * $B_{sustain}$：持续连发时的恒定稳态下压。

### (2) 高斯微随机散布（防机械感僵硬）
完全恒定的下压会让画面显得呆板、失去枪械质感。我们在稳态下压上叠加一个零均值的高斯噪声（Gaussian Noise）：
$$RY_{final} = RY + R(t) + \mathcal{N}(0, \sigma^2)$$
模拟人手自然对抗后坐力时的细微弹性震颤，打击感更为生动拟真。

---

## 6. Profile 声明式配置示例 (`profiles/fps-aim.yaml`)

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

## 7. 如何接入现有工具使用

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
