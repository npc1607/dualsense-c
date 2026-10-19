# 标准信号

规则表达式里可直接引用；未提供或过期的信号为 `0`，用 `has("x")` 判断有效性。

## 车辆遥测

| 信号 | 范围 | 说明 |
|---|---|---|
| `throttle` `brake` `clutch` | 0..1 | 踏板 |
| `steer` | −1..1 | 方向盘 |
| `speed` | km/h | 时速 |
| `rpm` | 0..1 | 按红线归一化 |
| `gear` | 数值 | 档位 |
| `slip` `lockup` `abs` `tc` `rumble` | 0..1 | 打滑/抱死/ABS/TC/路面震动 |
| `accel_g` `lat_g` | G | 垂直/横向加速度 |
| `surface` | 0..1 | 路面材质/脏污 |
| `slip_fl/fr/rl/rr` | 0..1 | 各轮滑移 |

## 手柄输入

| 信号 | 范围 | 说明 |
|---|---|---|
| `l2` `r2` | 0..1 | 扳机行程 |
| `l2_full` `r2_full` | 0/1 | 扳机触底（≥0.85） |
| `l1` `r1` `cross` `circle` `square` `triangle` | 0/1 | 按键 |
| `up/down/left/right` `ps` `touchpad` `mute` | 0/1 | 方向键/功能键 |
| `lx ly rx ry` | −1..1 | 摇杆 |
| `stick_l` `stick_r` | 0/1 | 摇杆下压 |

## 传感器

| 信号 | 说明 |
|---|---|
| `gyro_x/y/z` | 陀螺仪（归一化 −1..1） |
| `accel_x/y/z` | 加速度计 |
| `touch_x/y` | 触控板坐标 0..1（1920×1080） |
| `touch_active` | 是否接触 |
| `battery` | 电量 0..1（10% 步长） |
| `charging` | 充电中 0/1 |

## 音频

| 信号 | 说明 |
|---|---|
| `bass` | 20–110 Hz 包络 |
| `mid` | 160–450 Hz 包络 |
| `loudness` | 全带包络 |
| **`onset`** | 瞬态检测（fast/slow 包络差），**对背景音量免疫** |

## 音频 AI（YAMNet）

| 信号 | 说明 |
|---|---|
| `surface_water` `surface_metal` `surface_wood` `surface_gravel` | 地面材质置信度 |
| `audio_gunshot` `audio_explosion` `audio_impact` | 战况事件置信度 |

## 力反馈与自定义

| 信号 | 说明 |
|---|---|
| `ffb_left` `ffb_right` | 虚拟手柄捕获的游戏震动幅度 |
| `custom0` ~ `custom3` | 自定义映射通道 |
