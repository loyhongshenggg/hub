#!/usr/bin/env bash
# hub desktop launcher (docs/adr/0009-desktop-launcher.md). The Desktop
# shortcut starts it with signed Windows tools only:
#   conhost.exe --headless wsl.exe -d <distro> [--cd <repo>] -e bash <this script>
# (App Control can block unsigned programs, so there is no launcher .exe).
# It opens the hub in a browser app window, starting the hub first if it
# isn't running; when that window closes it stops what it started. The
# wsl.exe running this script keeps WSL up meanwhile.
#
# Two layouts: the repo (tools/desktop/launch.sh, core in dist/) and a
# release (/opt/hub/launch.sh, core in bin/, images/ to load on first start).
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
if [ -x "$here/bin/hub" ]; then
  root="$here" hubbin="$here/bin/hub" serve_root=(--root "$here")
else
  root="$(cd "$here/../.." && pwd)" hubbin="$root/dist/platform/core/hub" serve_root=()
fi
cd "$root"

conf="${HUB_DESKTOP_CONF:-$HOME/.config/hub/desktop.env}"
# CHROME (Linux path to chrome.exe or msedge.exe), STATE / STATE_WIN (the
# launcher's folder under %LOCALAPPDATA%), optionally HUB_ADDR.
# shellcheck source=/dev/null
. "$conf" || { echo "missing $conf — run: pnpm nx run desktop:install (repo) or /opt/hub/first-run.sh (release)" >&2; exit 1; }
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
# Release images ship with the app; load any this Docker doesn't have yet.
load_images() {
  local f tag
  for f in "$root"/images/*.tar.gz; do
    [ -e "$f" ] || continue
    tag="$(cat "${f%.tar.gz}.tag")"
    docker image inspect "$tag" >/dev/null 2>&1 && continue
    say "loading $tag"
    gunzip -c "$f" | docker load >>"$log" 2>&1
  done
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
# shellcheck disable=SC2086 # HUB_SERVE_ARGS is a list of flags
(
  for _ in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 0.5; done
  load_images
  exec "$hubbin" serve --session --addr "127.0.0.1:$port" "${serve_root[@]}" $HUB_SERVE_ARGS
) >"$STATE/core.log" 2>&1 &
core=$!

# The window opens at once on a "Starting hub…" page that switches over.
title=hub
# shellcheck source=/dev/null
[ -f "$root/release.env" ] && title="$(. "$root/release.env" && echo "$TITLE")"
sed -e "s#{{HUB_URL}}#$url#g" -e "s#{{TITLE}}#$title#g" "$here/splash.html" >"$STATE/starting.html"
(
  for _ in $(seq 600); do
    healthy && { say "hub is up"; exit 0; }
    kill -0 "$core" 2>/dev/null || { say "hub exited early"; alert "The hub couldn't start. Details: $STATE_WIN\\core.log"; exit 1; }
    sleep 0.5
  done
  alert "The hub didn't start within 5 minutes. Details: $STATE_WIN\\core.log"
) &
browser "file:///${STATE_WIN//\\//}/starting.html" >/dev/null 2>&1

say "window closed; stopping the hub"
HUB_ADDR="127.0.0.1:$port" "$hubbin" stop >>"$log" 2>&1
for _ in $(seq 360); do kill -0 "$core" 2>/dev/null || break; sleep 0.5; done
kill "$core" 2>/dev/null && say "hub killed after timeout"
say "done"
