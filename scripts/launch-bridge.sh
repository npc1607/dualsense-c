#!/usr/bin/env bash
# Launch ds5bridge.exe inside a game's Proton/Wine prefix.
#
# The bridge must share the prefix with the game, because the game publishes its
# telemetry as a *Windows* named section (acpmf_physics) that only processes in
# the same prefix can open.
#
# usage: ./scripts/launch-bridge.sh <steam-appid> [extra bridge flags...]
#   e.g. ./scripts/launch-bridge.sh 805550                 # ACC
#        ./scripts/launch-bridge.sh 3917090 -layout acr -debug
set -uo pipefail

APPID="${1:-}"
if [[ -z "$APPID" ]]; then
    echo "usage: $0 <steam-appid> [extra bridge flags...]" >&2
    echo "  Assetto Corsa 244210 | ACC 805550 | AC Rally 3917090" >&2
    exit 2
fi
shift

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BRIDGE="$HERE/bin/ds5bridge.exe"
LOG="${DS5_BRIDGE_LOG:-/tmp/ds5bridge.log}"

if [[ ! -f "$BRIDGE" ]]; then
    echo "ds5bridge.exe not built. Run: make bridge" >&2
    exit 1
fi

if pgrep -f 'ds5bridge\.exe' >/dev/null; then
    echo "bridge already running"
    exit 0
fi

if command -v protontricks-launch >/dev/null 2>&1; then
    nohup protontricks-launch --appid "$APPID" "$BRIDGE" "$@" >>"$LOG" 2>&1 &
    echo "bridge started via protontricks (pid $!), log: $LOG"
    exit 0
fi

# Fallback: run the game's own Proton wine binary against its prefix.
find_proton_wine() {
    local base="$HOME/.steam/steam/steamapps/common"
    local candidate
    for candidate in \
        "$base/Proton - Experimental/files/bin/wine64" \
        "$base/Proton - Experimental/files/bin/wine"; do
        [[ -x "$candidate" ]] && { echo "$candidate"; return 0; }
    done
    local d
    for d in "$base"/Proton*/files/bin/wine64; do
        [[ -x "$d" ]] && { echo "$d"; return 0; }
    done
    return 1
}

WINE="$(find_proton_wine)" || {
    echo "protontricks not found and no Proton wine located." >&2
    echo "Install protontricks (recommended), or set DS5_PROTON_WINE=/path/to/wine64." >&2
    exit 1
}
WINE="${DS5_PROTON_WINE:-$WINE}"

COMPAT="$HOME/.steam/steam/steamapps/compatdata/$APPID"
if [[ ! -d "$COMPAT/pfx" ]]; then
    echo "prefix not found: $COMPAT/pfx (run the game once first)" >&2
    exit 1
fi

STEAM_COMPAT_DATA_PATH="$COMPAT" \
WINEPREFIX="$COMPAT/pfx" \
    nohup "$WINE" "$BRIDGE" "$@" >>"$LOG" 2>&1 &
echo "bridge started via $WINE (pid $!), log: $LOG"
