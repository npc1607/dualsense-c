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

| 场景 | `\|accel_g\|` |
|---|---|---|
| 平稳巡航 | 0.00–0.05 |
| 砂石/颠簸 | 0.05–0.20 |
| 路肩 | 0.40–1.9 |
| 撞击 | 2.0–4.3（记录到过 13.17） |

Profile 里的阈值都按这个标定。注意 `wheelSlip` 是**原始滑移量**（可到 8），
不是 0..1，做门槛时要用 `smoothstep(0.35, 2.5, slip)` 这类归一化。

## 常见坑

- **桥在游戏重启后卡死**：日志不再增长但进程还在 → `bridge-restart`
- **`surface` / `carDamage` 在 ACR 恒为 0**：这两类字段该作无实现，规则会静默失效
- **Steam Input 必须开着**（ACR 只认虚拟 Xbox 手柄），但我们的触感走 hidraw，不受影响
