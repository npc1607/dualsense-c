# Far Far West 实例（射击）

一个完整的射击游戏适配：**自适应扳机 + HD 音圈 + 音频 AI**。

游戏 AppID `3124540`，UE 引擎，没有共享内存遥测，原生不支持 DS5。
所以用 `combo` 源同时拿两类输入：

```
trigger 源 → 手柄自身 L2/R2 行程   → 瞄准挡墙 / 开火 / 连发节奏
audio 源   → 包络 + YAMNet         → 瞬态冲击 + 材质纹理
```

## 运行

```bash
make build-onnx
./bin/ds5d-onnx -profiles ./profiles -profile far-far-west-hd -rate 125 -trace
```

（免 AI 的精简版：内置 `far-far-west` profile，用 `./bin/ds5d` 即可。）

## 三条触感通路

| 通路 | 延迟 | 作用 |
|---|---|---|
| 音圈直通（`mode: passthrough`） | ~40ms | 音圈重放游戏低频 → 真实材质纹理 |
| 瞬态 `onset` | ~44ms | 枪声/撞击的起音，双马达 + 双音圈 |
| AI 事件（`crossed`） | ~150ms | 确认是枪声/爆炸，补强 |

## 扳机

```
L2 2%–45%  rigid  90    → 轻阻力
L2 ≥45%    rigid 235    → 瞄准挡墙
R2 2%–70%  rigid 110    → 开火前段
R2 70%–97% rigid 255    → 枪机硬墙
R2 击发    weapon 210/130/24Hz  → 机械「咔哒」80ms
```

## 连发要互斥

按住 R2 时只保留**一条** 14Hz 节奏（优先级 48），单发路径加 `r2 < 0.80` 互斥。
否则瞬态 26Hz + 连发 14Hz + AI 突发三种频率打架，加特林会"抖得没规律"。

## 调参速查

| 现象 | 改哪里 |
|---|---|
| 走路一直震 | 删掉常驻 `haptic_*` 增益；`low_hz` 抬到 55–70 |
| 开火拖尾 | AI 事件用 `crossed()`；`hold` 降到 80–90ms |
| 加特林乱 | 按住与其他路径互斥，只留一条节奏 |
| 开枪没反应 | 开 `env_debug` 看 `onset`，不够就降门槛 |
| 音圈太闷/太脆 | `low_hz` 55↔35；`high_hz` 200↔260 |
