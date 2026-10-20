# 力反馈镜像（虚拟手柄）

游戏只会把震动发给**它正在对话的那个设备**。只要 Steam Input 占着物理手柄，
外部程序既看不到指令，也抢不到输出。

`ds5vpad` 反过来做：自己建虚拟 Xbox 手柄、独占物理手柄、把游戏震动转成 UDP。

## 架构

<p align="center">
  <img src="https://raw.githubusercontent.com/npc1607/dualsense-c/main/docs/ffb-virtual-pad.svg" alt="力反馈镜像与虚拟手柄架构" width="100%">
</p>
<p align="center">
  <sub>（注：架构图源文件见 <a href="https://github.com/npc1607/dualsense-c/blob/main/docs/ffb-virtual-pad.excalidraw">docs/ffb-virtual-pad.excalidraw</a>，支持在 <a href="https://excalidraw.com">Excalidraw</a> 中导入编辑）</sub>
</p>

## 用法

```bash
# 现代游戏（XInput 协议族，模拟 Xbox 360 手柄）
./bin/ds5vpad &
./bin/ds5d -profile ffb-recoil  # 消费 9998 上的力反馈

# 2000 年代经典赛车/老游戏（DirectInput 8 协议族，模拟罗技 6 轴 12 键手柄）
./bin/ds5vpad -proto dinput8 &
```

支持的硬件协议族：
- `-proto xinput`（默认）：Xbox 360（`045e:028e`），适用于现代游戏与 Steam/Proton 默认映射。
- `-proto dinput8`：罗技 Dual Action（`046d:c216`），纯正 DirectInput 8 架构，拥有独立的 L2（刹车）/ R2（油门）模拟轴和原始 1..12 编号按钮，完全摆脱 XInput 驱动层将扳机强行合并为单轴的限制。

## 验证链路（不开游戏）

```bash
./bin/ds5vpad &
./bin/ds5d -profile ffb-recoil -trace &
./bin/ffclient -dev /dev/input/eventN     # N = ds5vpad 打印的节点
# 期望：ffb_left≈0.61 ffb_right≈0.31（对应 strong=40000/weak=20000）
```

## 已知局限

uinput 只暴露 `UI_FF_UPLOAD` / `UI_FF_ERASE`，**没有 playback 回调**。
所以实现采用「以最近一次上传的幅度为准，超过 500ms 归零」：

- Proton/Wine 的 evdev FF 后端在强度变化时会重新上传 → 跟踪准确
- 「上传一次、只写 EV_FF 播放」的游戏 → 只能拿到首次幅度

彻底解决需要内核侧回调，或 Windows 的 ViGEm（其 rumble 回调直接给播放事件）。

## 权限

uinput 设备通常自动获得 `uaccess` ACL；仓库另提供 `udev/71-ds5vpad.rules` 作为兜底。
