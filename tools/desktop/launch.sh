#!/usr/bin/env bash
# hub desktop launcher (docs/adr/0009-desktop-launcher.md). The "hub" Desktop
# shortcut starts it with signed Windows tools only:
#   conhost.exe --headless wsl.exe -d <distro> --cd <repo> -e bash tools/desktop/launch.sh
# (App Control on this machine blocks unsigned programs, so there is no
# launcher .exe). It opens the hub in a Chrome app window, starting the hub
# first if it isn't running; when that window closes it stops what it
# started. The wsl.exe running this script keeps WSL up meanwhile.
set -uo pipefail
cd "$(dirname "$0")/../.."

conf="${HUB_DESKTOP_CONF:-$HOME/.config/hub/desktop.env}"
# CHROME (Linux path to chrome.exe), STATE (Linux path of %LOCALAPPDATA%\hub), STATE_WIN.
# shellcheck source=/dev/null
. "$conf" || { echo "missing $conf — run: pnpm nx run desktop:install" >&2; exit 1; }
: "${HUB_ADDR:=127.0.0.1:7000}" "${HUB_SERVE_ARGS:=}" "${CHROME_EXTRA:=}"
port="${HUB_ADDR##*:}"
host="hub.localhost:$port"
url="http://$host/"
mkdir -p "$STATE"
log="$STATE/launcher.log"

say() { echo "$(date '+%F %T') $*" >>"$log"; }
healthy() { curl -sf -m 2 -H "Host: $host" "http://127.0.0.1:$port/api/healthz" >/dev/null; }
# shellcheck disable=SC2086 # CHROME_EXTRA is a list of switches
browser() {
  "$CHROME" --app="$1" --user-data-dir="$STATE_WIN\\browser" --no-first-run \
    --no-default-browser-check --window-size=1440,920 $CHROME_EXTRA
}
alert() {
  local msg=${1//\'/\'\'}
  powershell.exe -NoProfile -Command "Add-Type -AssemblyName PresentationFramework; [void][System.Windows.MessageBox]::Show('$msg', 'hub')" >/dev/null 2>&1
}

# Already running (a terminal, another window, start-on-login): just open a window.
if healthy; then
  say "hub already running; opening a window"
  browser "$url" >/dev/null 2>&1 &
  disown
  sleep 2
  exit 0
fi

say "starting the hub"
for _ in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 0.5; done
# shellcheck disable=SC2086 # HUB_SERVE_ARGS is a list of flags
dist/platform/core/hub serve --session --addr "127.0.0.1:$port" $HUB_SERVE_ARGS >"$STATE/core.log" 2>&1 &
core=$!

# The window opens at once on a "Starting hub…" page that switches over.
sed "s#{{HUB_URL}}#$url#g" tools/desktop/splash.html >"$STATE/starting.html"
(
  for _ in $(seq 360); do
    healthy && { say "hub is up"; exit 0; }
    kill -0 "$core" 2>/dev/null || { say "hub exited early"; alert "The hub couldn't start. Details: %LOCALAPPDATA%\\hub\\core.log"; exit 1; }
    sleep 0.5
  done
  alert "The hub didn't start within 3 minutes. Details: %LOCALAPPDATA%\\hub\\core.log"
) &
browser "file:///${STATE_WIN//\\//}/starting.html" >/dev/null 2>&1

say "window closed; stopping the hub"
HUB_ADDR="127.0.0.1:$port" dist/platform/core/hub stop >>"$log" 2>&1
for _ in $(seq 360); do kill -0 "$core" 2>/dev/null || break; sleep 0.5; done
kill "$core" 2>/dev/null && say "hub killed after timeout"
say "done"
