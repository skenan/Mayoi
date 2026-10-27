#!/usr/bin/env bash
# Builds Mayoi and puts it in /Applications in place of the one there,
# quitting it first and opening it again if it was running. The settings,
# the recent folders and the windows stay: they live in
# ~/Library/Application Support/Mayoi, not in the app. Mayoi Dev, of
# scripts/dev.sh, is another app and is left alone.
# Usage: scripts/install.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP=/Applications/Mayoi.app
ID=dev.mayoi.editor
if [ $# -ne 0 ]; then
    sed -n '2,7s/^# \{0,1\}//p' "$0" >&2
    exit 1
fi

cd "$ROOT"
go tool mygo build -skip-dmg
BUILT="$ROOT/build/darwin-arm64/Mayoi.app"

running() { [ -n "$(lsappinfo find bundleid="$ID")" ]; }

# Quitting as the menu does, so that it keeps its windows for the next
# start, and asks about changes not saved.
was_running=false
if running; then
    was_running=true
    echo "==> quitting Mayoi"
    osascript -e "tell application id \"$ID\" to quit"
    for _ in $(seq 60); do
        running || break
        sleep 0.5
    done
    if running; then
        echo "Mayoi is still running: save or discard its changes, quit it, and run this again." >&2
        exit 1
    fi
fi

echo "==> installing $APP"
rm -rf "$APP"
cp -R "$BUILT" "$APP"

if $was_running; then
    echo "==> opening Mayoi"
    open "$APP"
fi
