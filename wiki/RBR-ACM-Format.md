# RBR 控制器配置文件（`pfsam.acm`）二进制结构解析

理查德·伯恩斯拉力赛（Richard Burns Rally）的按键与轴绑定不使用文本保存，而是存储在 `SavedGames/pf<ProfileName>.acm` 二进制文件中。

---

## 1. 总体文件布局图表

```
+-------------------------------------------------------------------------+
|                              文件头部 Header                            |
| 0x00..0x03 : "PAMA" 魔数 (4 字节 ASCII)                                |
| 0x04..0x07 : 格式版本号 (uint32_t, 默认 0x00000300)                     |
| 0x08..0x0B : 配置套数 Config Count (uint32_t)                           |
| 0x0C..0x0F : 设备总数 Device Count (uint32_t)                           |
| 0x10..0x13 : 绑定记录总数 Binding Count (uint32_t)                      |
+-------------------------------------------------------------------------+
|                           设备描述表 Device Table                        |
| 每个设备块包含 16 字节 DirectInput GUID + 4 字节 "DEST" 标记            |
| 0x14..0x23 : Device[0] GUID (16 bytes)                                  |
| 0x24..0x27 : "DEST" 标签 (4 bytes)                                      |
| 0x28..0x37 : Device[1] GUID (16 bytes)                                  |
| 0x38..0x3B : "DEST" 标签 (4 bytes)                                      |
| 0x3C..0x4B : Device[2] GUID (16 bytes) (通常为键盘 GUID)                |
| 0x4C..0x4F : "DEST" 标签 (4 bytes)                                      |
+-------------------------------------------------------------------------+
|                           绑定项数据 Bindings Block                      |
| 0x42..0x43 : 2 字节填充 (0x0000)                                        |
| 随后是 N 个定长 12 字节的绑定三元组 (DeviceID, InputCode, FunctionID)     |
| +------------+-----------------------+---------------+                  |
| | DeviceID   | InputCode (Type+Code) | FunctionID    |                  |
| | (uint32_t) | (uint32_t)            | (uint32_t)    |                  |
| +------------+-----------------------+---------------+                  |
| Entry 0 (12 bytes)                                                      |
| Entry 1 (12 bytes)                                                      |
| ...                                                                     |
+-------------------------------------------------------------------------+
```

---

## 2. 字段详细定义

### 头部（Header，20 字节）
| 偏移 | 大小 | 类型 | 含义 | 样例值 |
|---|---|---|---|---|
| `0x00` | 4 | char[4] | 识别魔数 | `PAMA` |
| `0x04` | 4 | uint32 | 引擎版本 | `0x00000300` |
| `0x08` | 4 | uint32 | 配置套数 | `1` |
| `0x0C` | 4 | uint32 | 已识别设备数 | `3` |
| `0x10` | 4 | uint32 | 绑定条目总数 | `11` |

### 设备项（Device Entry，20 字节 / 块）
| 字段 | 大小 | 说明 |
|---|---|---|
| `Device GUID` | 16 字节 | Windows DirectInput 实例 GUID（包含 VID/PID 与设备序号） |
| `DEST Tag` | 4 字节 | ASCII 固定标记 `DEST` |

> 典型设备序号映射：
> - `Device 0`：第一手柄 / 方向盘
> - `Device 1`：第二手柄 / 虚拟手柄
> - `Device 2`：系统键盘

### 绑定三元组（Binding Entry，定长 12 字节）
```c
struct RBRBindingEntry {
    uint32_t device_id;  // 引用设备表的索引 (0, 1, 2...)
    uint16_t input_code; // 按键/轴扫描码 (低16位)
    uint16_t input_type; // 输入类型：0x02=模拟轴 (Axis), 0x04=按键 (Key)
    uint32_t function_id;// 绑定的功能 ID (转向/油门/刹车等)
};
```

---

## 3. 功能 ID（Function ID）对照表

| Function ID | 英文名称 | 对应动作 | 推荐设备类型 |
|---|---|---|---|
| `0` | `Steering` | 方向盘 / 转向 | 模拟轴 (`input_type = 0x02`) |
| `1` | `Steer Left` | 数字左转 | 按键 (`input_type = 0x04`) |
| `2` | `Steer Right` | 数字右转 | 按键 (`input_type = 0x04`) |
| `3` | `Throttle` | 独立油门踏板 | 模拟轴 / 按键 |
| `4` | `Accelerate and Brake` | 油门刹车合并轴 | 模拟轴 |
| `5` | `Footbrake` | 独立刹车踏板 | 模拟轴 / 按键 |
| `6` | `Handbrake` | 手刹 | 模拟轴 / 按键 |
| `7` | `Gear Up` | 升挡 | 按键 |
| `8` | `Gear Down` | 降挡 | 按键 |
| `9` | `Reverse Gear` | 倒挡 | 按键 |
| `12` | `View / Camera` | 视角切换 | 按键 |

---

## 4. 常见问题：为什么手动改 ini 会丢键位

1. **唯一性**：游戏在主菜单退出时会以 `pfsam.acm` 的二进制内容为准写盘。
2. **设备插拔错位**：如果物理设备断开或顺序变动，设备表的 GUID 索引发生偏移，所有 `device_id` 就会指向错误硬件，导致“方向变油门”等映射错乱。
