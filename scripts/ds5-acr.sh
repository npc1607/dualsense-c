#!/usr/bin/env bash
# Start/stop the Assetto Corsa (Rally) haptics stack: the native daemon plus the
# in-prefix bridge that reads acpmf_physics.
#
# usage: scripts/ds5-acr.sh {start|stop|restart|status|bridge-restart|logs}
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

# Patterns use a bracket so pgrep never matches this script's own command line.
DS5D_PAT='bin/ds5[d] -profile'
BRIDGE_PAT='ds5bridg[e]\.exe'

die() { printf 'ds5-acr: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

require_bin() {
	[ -x "$BIN/ds5d" ] || die "missing $BIN/ds5d (run: make build)"
	[ -f "$BIN/ds5bridge.exe" ] || die "missing $BIN/ds5bridge.exe (run: make bridge)"
}

# pgrep_any prints PIDs matching a pattern, newest first, excluding ourselves.
pgrep_any() {
	pgrep -f "$1" 2>/dev/null | grep -v "^$$\$" || true
}

start_daemon() {
	local pids
	pids="$(pgrep_any "$DS5D_PAT")"
	if [ -n "$pids" ]; then
		info "ds5d already running (pid $(echo "$pids" | tr '\n' ' ')) — leaving it alone"
		return 0
	fi
	rm -f "$DS5D_LOG"
	setsid nohup "$BIN/ds5d" -profile "$PROFILE" -rate "$RATE" -trace \
		>"$DS5D_LOG" 2>&1 < /dev/null &
	echo $! >"$DS5D_PID"
	sleep 2
	if ! grep -q 'driving ' "$DS5D_LOG" 2>/dev/null; then
		info "ds5d failed to start:"
		tail -5 "$DS5D_LOG" >&2
		return 1
	fi
	info "ds5d     : profile $PROFILE, $(grep -o 'driving .*' "$DS5D_LOG" | head -1)"
	grep -q 'haptics: voice-coil' "$DS5D_LOG" && info "           voice coils enabled"
}

start_bridge() {
	local pids
	pids="$(pgrep_any "$BRIDGE_PAT")"
	if [ -n "$pids" ]; then
		info "bridge already running (pid $(echo "$pids" | tr '\n' ' ')) — leaving it alone"
		return 0
	fi
	rm -f "$BRIDGE_LOG"
	setsid nohup protontricks-launch --appid "$APPID" "$BIN/ds5bridge.exe" \
		-layout acr -debug >>"$BRIDGE_LOG" 2>&1 < /dev/null &
	echo $! >"$BRIDGE_PID"
	info "bridge   : starting in Proton prefix of AppID $APPID (may take a few seconds)"
}

stop_all() {
	local pids
	pids="$(pgrep_any "$BRIDGE_PAT")"
	if [ -n "$pids" ]; then
		# shellcheck disable=SC2086
		kill $pids 2>/dev/null
		info "bridge   : stopped"
	fi
	pids="$(pgrep_any 'protontricks-launch .*ds5bridg[e]')"
	if [ -n "$pids" ]; then
		# shellcheck disable=SC2086
		kill $pids 2>/dev/null
	fi
	pids="$(pgrep_any "$DS5D_PAT")"
	if [ -n "$pids" ]; then
		# shellcheck disable=SC2086
		kill $pids 2>/dev/null
		info "ds5d     : stopped"
	fi
	# Audio children are normally reaped by ds5d; sweep leftovers that point at
	# the controller so the voice coils do not keep a stream open.
	pids="$(pgrep_any 'paca[t] --playback --device=.*DualSense')"
	[ -n "$pids" ] && kill $pids 2>/dev/null
	pids="$(pgrep_any 'pare[c] --device=.*DualSense')"
	[ -n "$pids" ] && kill $pids 2>/dev/null
	sleep 1
	rm -f "$DS5D_PID" "$BRIDGE_PID"
	info "controller is back to Steam-only control"
}

status() {
	local dpid bpid
	dpid="$(pgrep_any "$DS5D_PAT" | tr '\n' ' ')"
	bpid="$(pgrep_any "$BRIDGE_PAT" | tr '\n' ' ')"

	info "ds5d     : ${dpid:-not running}"
	info "bridge   : ${bpid:-not running}"

	if [ -n "$dpid" ] && [ -f "$DS5D_LOG" ]; then
		local last_bass last_acc last_sig
		last_sig="$(grep 'signals:' "$DS5D_LOG" | tail -1 | sed 's/.*signals: //' | cut -c1-90)"
		info "signals  : ${last_sig:-(none yet)}"
	fi

	if [ -n "$bpid" ] && [ -f "$BRIDGE_LOG" ]; then
		local line age
		line="$(grep -E 'streaming|not found|packetId|EVENT' "$BRIDGE_LOG" | tail -1)"
		info "bridge   : ${line:-no output yet}"
		age=$(( $(date +%s) - $(stat -c %Y "$BRIDGE_LOG" 2>/dev/null || echo 0) ))
		if [ "$age" -gt 10 ]; then
			info "           ⚠ log has not moved for ${age}s — the bridge is likely wedged;"
			info "             run: $0 bridge-restart"
		fi
	fi

	if ss -lunp 2>/dev/null | grep -q ':9999'; then
		info "udp 9999 : bound (ds5d listening)"
	else
		info "udp 9999 : free"
	fi
	info "hidraw   : $(fuser /dev/hidraw* 2>/dev/null | head -1 || echo 'n/a')"
}

case "${1:-}" in
start)
	require_bin
	mkdir -p "$RUN_DIR"
	start_daemon || exit 1
	start_bridge
	info ""
	info "log: $DS5D_LOG"
	info "log: $BRIDGE_LOG"
	info "start a session in the game — the bridge connects once acpmf_physics appears"
	;;
stop)
	stop_all
	;;
restart)
	stop_all
	mkdir -p "$RUN_DIR"
	require_bin
	start_daemon || exit 1
	start_bridge
	;;
bridge-restart)
	mkdir -p "$RUN_DIR"
	stop_all >/dev/null
	# stop_all also stops ds5d; bring it back so the user does not lose triggers.
	start_daemon || exit 1
	start_bridge
	;;
status)
	status
	;;
logs)
	tail -n 40 -f "$DS5D_LOG" "$BRIDGE_LOG" 2>/dev/null
	;;
*)
	echo "usage: $0 {start|stop|restart|bridge-restart|status|logs}" >&2
	exit 2
	;;
esac
