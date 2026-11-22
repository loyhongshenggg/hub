#!/bin/bash
# macOS launcher (docs/adr/0012-macos-release.md): opens the app in a
# Chrome or Edge app window, starting the hub (native, no Docker) first if
# needed; when the window closes it stops what it started. Run by the Dock
# app (~/Applications/<TITLE>.app) or the `<name>` command.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=/dev/null
. "$here/release.env"
# Apps launched from the Dock get a minimal PATH; OCR tools come from Homebrew.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
state="$HOME/Library/Application Support/$TITLE"
mkdir -p "$state"
log="$state/launcher.log"
port="$PORT" host="hub.localhost:$PORT" url="http://hub.localhost:$PORT/"
say() { echo "$(date '+%F %T') $*" >>"$log"; }
healthy() { curl -sf -m 2 -H "Host: $host" "http://127.0.0.1:$port/api/healthz" >/dev/null; }
alert() { osascript -e "display alert \"$TITLE\" message \"$1\"" >/dev/null 2>&1 || echo "$1" >&2; }

browser=""
for app in "/Applications/Google Chrome.app" "$HOME/Applications/Google Chrome.app" \
  "/Applications/Microsoft Edge.app" "$HOME/Applications/Microsoft Edge.app"; do
  for bin in "$app/Contents/MacOS/Google Chrome" "$app/Contents/MacOS/Microsoft Edge"; do
    [ -x "$bin" ] && { browser="$bin"; break 2; }
  done
done
[ -n "$browser" ] || { alert "$TITLE needs Google Chrome or Microsoft Edge. Install one, then open $TITLE again."; exit 1; }
window() {
  # shellcheck disable=SC2086
  "$browser" --app="$1" --user-data-dir="$state/browser" --no-first-run --no-default-browser-check \
    --window-size=1440,920 ${CHROME_EXTRA:-}
}

if healthy; then
  say "already running; opening a window"
  window "$url" >/dev/null 2>&1 &
  disown
  exit 0
fi

say "starting"
"$here/bin/hub" serve --native --session --root "$here" --addr "127.0.0.1:$port" --data "$state/data" >"$state/core.log" 2>&1 &
core=$!
stop() {
  say "stopping"
  HUB_ADDR="127.0.0.1:$port" "$here/bin/hub" stop >>"$log" 2>&1
  for _ in $(seq 120); do kill -0 "$core" 2>/dev/null || break; sleep 0.5; done
  kill "$core" 2>/dev/null
  say "done"
}
trap 'stop; exit 0' TERM INT # quitting the Dock app stops the hub too

sed -e "s#{{HUB_URL}}#$url#g" -e "s#{{TITLE}}#$TITLE#g" "$here/splash.html" >"$state/starting.html"
(
  for _ in $(seq 240); do
    healthy && { say "up"; exit 0; }
    kill -0 "$core" 2>/dev/null || { say "exited early"; alert "$TITLE couldn't start. Details: ~/Library/Application Support/$TITLE/core.log"; exit 1; }
    sleep 0.5
  done
) &
window "file://$state/starting.html" >/dev/null 2>&1
stop
