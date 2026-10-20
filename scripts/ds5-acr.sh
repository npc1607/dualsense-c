#!/usr/bin/env bash
# Start/stop the Assetto Corsa (Rally) haptics stack: the native daemon plus the
# in-prefix bridge that reads acpmf_physics.
#
# 两段式启动流程 (避免 Proton 前缀锁冲突):
#   第 1 步: ./scripts/ds5-acr.sh start   (启动 Linux 原生 ds5d 守护进程)
#   第 2 步: 在 Steam 中启动游戏 (AC Rally / AC / ACC) 并进入赛段
#   第 3 步: ./scripts/ds5-acr.sh bridge  (游戏启动后，将桥注入已有的前缀连接共享内存)
#
# 或者使用全自动模式:
#   ./scripts/ds5-acr.sh auto             (启动 ds5d 并自动监听等待游戏启动后注入桥)
#
# usage: scripts/ds5-acr.sh {start|bridge|auto|stop|restart|status|logs}
#
# env overrides:
#   DS5_APPID    Steam AppID (default 3917090 = Assetto Corsa Rally)
#   DS5_PROFILE  profile name (default assetto-corsa)
#   DS5_RATE     HID report rate (default 125; USB supports 250)
#   DS5_BIN      build directory (default <repo>/bin)
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${DS5_BIN:-$REPO/bin}"
APPID="${DS5_APPID:-3917090}"
PROFILE="${DS5_PROFILE:-assetto-corsa}"
RATE="${DS5_RATE:-125}"

RUN_DIR="${XDG_RUNTIME_DIR:-/tmp}/ds5-acr"
DS5D_LOG="$RUN_DIR/ds5d.log"
BRIDGE_LOG="$RUN_DIR/bridge.log"
DS5D_PID="$RUN_DIR/ds5d.pid"
BRIDGE_PID="$RUN_DIR/bridge.pid"

die() { printf 'ds5-acr: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

require_bin() {
	[ -x "$BIN/ds5d" ] || die "missing $BIN/ds5d (run: make build)"
	[ -f "$BIN/ds5bridge.exe" ] || die "missing $BIN/ds5bridge.exe (run: make bridge)"
}

is_ds5d_running() {
	pgrep -x ds5d >/dev/null 2>&1
}

is_bridge_running() {
	pgrep -f "ds5bridge\.exe" >/dev/null 2>&1
}

is_game_running() {
	pgrep -f "acr\.exe|acs\.exe|AssettoCorsa" >/dev/null 2>&1
}

start_daemon() {
	if is_ds5d_running; then
		info "ds5d 已经在运行中 (PID: $(pgrep -x ds5d | tr '\n' ' '))"
		return 0
	fi
	rm -f "$DS5D_LOG"
	setsid nohup "$BIN/ds5d" -profile "$PROFILE" -rate "$RATE" -trace \
		>"$DS5D_LOG" 2>&1 < /dev/null &
	echo $! >"$DS5D_PID"
	sleep 1.5
	if ! grep -q 'driving ' "$DS5D_LOG" 2>/dev/null; then
		info "ds5d 启动失败:"
		tail -5 "$DS5D_LOG" >&2
		return 1
	fi
	info "ds5d     : 启动成功 (profile $PROFILE, $(grep -o 'driving .*' "$DS5D_LOG" | head -1))"
	grep -q 'haptics: voice-coil' "$DS5D_LOG" && info "           双音圈触感已激活 (48kHz USB Audio)"
}

start_bridge() {
	if is_bridge_running; then
		info "bridge   : 共享内存桥已经在运行中 (PID: $(pgrep -f 'ds5bridge\.exe' | tr '\n' ' '))"
		return 0
	fi
	rm -f "$BRIDGE_LOG"
	info "bridge   : 正在向 Proton 前缀 (AppID $APPID) 注入 ds5bridge.exe ..."
	setsid nohup protontricks-launch --appid "$APPID" "$BIN/ds5bridge.exe" \
		-layout acr -debug >>"$BRIDGE_LOG" 2>&1 < /dev/null &
	echo $! >"$BRIDGE_PID"
	sleep 2
	info "bridge   : 注入完成，正在后台监听 acpmf_physics 物理内存"
}

stop_all() {
	# 干净终止桥与守护进程
	killall -q ds5bridge.exe 2>/dev/null || true
	pkill -f "protontricks-launch.*ds5bridge" 2>/dev/null || true
	killall -q ds5d 2>/dev/null || true
	rm -f "$DS5D_PID" "$BRIDGE_PID"
	sleep 0.5
	info "ds5-acr  : 全部服务已停止，手柄已释放"
}

status() {
	info "=== ds5-acr 运行状态 ==="
	if is_ds5d_running; then
		info "ds5d     : 正在运行 (PID: $(pgrep -x ds5d | tr '\n' ' '))"
	else
		info "ds5d     : 未运行"
	fi

	if is_bridge_running; then
		info "bridge   : 正在运行 (PID: $(pgrep -f 'ds5bridge\.exe' | tr '\n' ' '))"
	else
		info "bridge   : 未运行"
	fi

	if is_game_running; then
		info "游戏检测 : 游戏正在运行 (ACR/AC/ACC)"
	else
		info "游戏检测 : 游戏未运行 (请在 Steam 中启动)"
	fi

	if [ -f "$DS5D_LOG" ] && is_ds5d_running; then
		local last_sig
		last_sig="$(grep 'signals:' "$DS5D_LOG" | tail -1 | sed 's/.*signals: //' | cut -c1-90)"
		info "当前信号 : ${last_sig:-(等待赛道数据...)}"
	fi

	if [ -f "$BRIDGE_LOG" ] && is_bridge_running; then
		local line
		line="$(grep -E 'streaming|not found|packetId|EVENT' "$BRIDGE_LOG" | tail -1)"
		info "桥端状态 : ${line:-等待中...}"
	fi
}

case "${1:-}" in
start)
	require_bin
	mkdir -p "$RUN_DIR"
	start_daemon || exit 1
	info ""
	info "【两段式第一阶段完成】ds5d 守护进程已就绪！"
	info "下一步操作："
	info "  1. 打开 Steam 启动 Assetto Corsa Rally 并进入赛道；"
	info "  2. 游戏启动后，在终端运行: ./scripts/ds5-acr.sh bridge"
	info "     或者直接运行: ./scripts/ds5-acr.sh auto (自动监听拉起)"
	;;
bridge)
	require_bin
	mkdir -p "$RUN_DIR"
	start_bridge
	info "查看实时遥测日志: ./scripts/ds5-acr.sh logs"
	;;
auto)
	require_bin
	mkdir -p "$RUN_DIR"
	start_daemon || exit 1
	info ""
	info "【自动监听模式已启动】"
	info "请在 Steam 中启动游戏，本脚本将自动侦测游戏并无缝注入桥..."
	while true; do
		if is_game_running; then
			info "检测到游戏已启动！正在自动注入 ds5bridge.exe ..."
			sleep 2
			start_bridge
			break
		fi
		sleep 1.5
	done
	info "全部就绪！查看日志: ./scripts/ds5-acr.sh logs"
	;;
stop)
	stop_all
	;;
restart)
	stop_all
	mkdir -p "$RUN_DIR"
	require_bin
	start_daemon || exit 1
	;;
status)
	status
	;;
logs)
	tail -n 40 -f "$DS5D_LOG" "$BRIDGE_LOG" 2>/dev/null
	;;
*)
	echo "usage: $0 {start|bridge|auto|stop|restart|status|logs}" >&2
	exit 2
	;;
esac
