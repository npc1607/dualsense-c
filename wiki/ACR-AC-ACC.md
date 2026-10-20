# AC / ACC / AC Rally（共享内存）

这三款游戏把物理遥测写入 Windows 命名共享内存 `acpmf_physics`。
Proton/Wine 进程在独立命名空间里，原生 Linux 程序读不到，所以需要**同前缀内的桥**。

```
acr.exe (Proton) ──写──► acpmf_physics
                              ▲
ds5bridge.exe (同前缀) ──读────┘ ──UDP 127.0.0.1:9999──► ds5d ──► 手柄
```

## 一键启停

```bash
./scripts/ds5-acr.sh start           # AppID 默认 3917090 (AC Rally)
./scripts/ds5-acr.sh status          # 进程 / signals / 桥是否卡住
./scripts/ds5-acr.sh stop            # 全部停掉，手柄交还 Steam
./scripts/ds5-acr.sh bridge-restart  # 桥卡死时只重启桥
./scripts/ds5-acr.sh logs            # 同时 tail 两个日志

# 覆盖默认值
DS5_APPID=805550 DS5_RATE=250 ./scripts/ds5-acr.sh start   # ACC
```

AppID：AC `244210` · ACC `805550` · AC Rally `3917090`

## 手动等价

```bash
make bridge
./bin/ds5d -profile assetto-corsa -rate 125 &
./scripts/launch-bridge.sh 3917090
```

## 何时才算通

- 桥日志出现 `opened "acpmf_physics"; streaming`
- `ds5d -trace` 出现 `signals: brake=... accel_g=... speed=...`

**只在进入赛道后**共享内存才存在；菜单里桥会一直打印
`shared memory not found — retrying`，这是正常的。

## 实测信号量级（ACR）

| 场景 | `accel_g` 绝对值（G） |
|---|---|
| 平稳巡航 | 0.00 – 0.05 |
| 砂石 / 颠簸 | 0.05 – 0.20 |
| 路肩 | 0.40 – 1.9 |
| 撞击 | 2.0 – 4.3（记录到过 13.17） |

Profile 里的阈值都按这个标定。

注意 `wheelSlip` 是**原始滑移量**（实测可到 8），不是 0..1。直接写 `slip > 0.5`
会让左马达长期饱和；应做归一化，例如
`clamp(230 * smoothstep(0.35, 2.5, slip), 0, 230)`。

---

## 融合全特性架构速查（Profile 特色）

| 体验维度 | 依托物理量 | 输出执行器与特性 |
|---|---|---|
| **L2 无助力液压刹车** | `brake` (脚刹踏板) | L2 `rigid`：前段微空行程，中段线性陡增，后段坚挺刚性液压墙（`force 235~255`） |
| **R2 柔性渐进油门** | `throttle` (油门踏板) | R2 `rigid`：前段轻盈易于细致控油，后段饱满回弹，长赛段手指不疲劳 |
| **红线换挡转速回馈** | `custom0 > 6600` (引擎原始 RPM) | 右马达 58Hz 正弦微颤：逼近红线时给予细腻高频震感，提示最佳升挡时机 |
| **轮胎抓地力临界高频啸叫** | `lat_g > 0.35` &amp; `0.35 < slip < 1.4` | 左音圈 160Hz 细密微颤：模拟直驱方向盘在轮胎达到最大摩擦力极限边缘时的物理嘶鸣 |
| **推头力矩骤降松脱** | `front_slip >= 1.4` &amp; `lat_g > 0.5` | 右马达 24Hz 方波松脱脉冲：前轮失抓失速时提示推头警告 |
| **3D 立体声砂石颗粒** | `accel_g` &amp; `speed > 5` | 左音圈 52Hz + 右音圈 40Hz（形成 12Hz 触觉声学拍频，逼真呈现砂石敲击底盘） |
| **底盘起伏与路面体感** | `accel_g` (垂直 G) | 右马达 54Hz 细密微颤 + 左大马达 35Hz 车身沉降，两路不同频率形成拍频 |
| **ABS / TC 独立反馈** | `abs > 0.05` / `tc > 0.05` | ABS 触发右马达 15Hz 急促方波脉冲；TC 触发右马达 32Hz 平滑震颤 |
| **路肩与颠簸碎震** | `accel_g > 0.35` (路肩尖峰) | 左右马达 30Hz/44Hz 方波碎震（`hold 70ms` 保证瞬态感知） |
| **飞坡落地与硬碰撞重锤** | `accel_g > 1.5` (重着陆/撞击) | 左右马达 16Hz/22Hz 方波重击 + 音圈 255 满幅爆发（`hold 130ms`） |
| **交互式座舱仪表灯条** | `brake`, `throttle`, `custom0` | 刹车赛车红、油门翠绿、接近 8000 转红线电光蓝换挡提示 |

## 常见坑

- **桥在游戏重启后卡死**：日志不再增长但进程还在 → `bridge-restart`
- **`surface` / `carDamage` 在 ACR 恒为 0**：这两类字段该作无实现，规则会静默失效
- **Steam Input 必须开着**（ACR 只认虚拟 Xbox 手柄），但我们的触感走 hidraw，不受影响
