#!/usr/bin/env bash
# Installs the hub Desktop shortcut on Windows (docs/adr/0009-desktop-launcher.md).
# Run inside WSL via `pnpm nx run desktop:install`; safe to re-run.
#   - writes ~/.config/hub/desktop.env for tools/desktop/launch.sh
#   - puts hub.ico in %LOCALAPPDATA%\hub
#   - creates Desktop\hub.lnk → conhost.exe --headless wsl.exe … launch.sh
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
[ -n "${WSL_DISTRO_NAME:-}" ] || { echo "Run this inside WSL." >&2; exit 1; }
[ -f "$root/dist/tools/desktop/hub.ico" ] || { echo "Build the icon first: pnpm nx run desktop:icon" >&2; exit 1; }
[ -x "$root/dist/platform/core/hub" ] || { echo "Build the core first: pnpm nx build core" >&2; exit 1; }

ps() { powershell.exe -NoProfile -NonInteractive -Command "$1" | tr -d '\r'; }

state_win="$(ps '[Environment]::GetFolderPath("LocalApplicationData")')\\hub"
state="$(wslpath -u "$state_win")"
chrome=""
for p in "/mnt/c/Program Files/Google/Chrome/Application/chrome.exe" \
  "/mnt/c/Program Files (x86)/Google/Chrome/Application/chrome.exe" \
  "$(wslpath -u "$(ps '[Environment]::GetFolderPath("LocalApplicationData")')")/Google/Chrome/Application/chrome.exe"; do
  [ -f "$p" ] && { chrome="$p"; break; }
done
[ -n "$chrome" ] || { echo "Chrome wasn't found; install it and run this again." >&2; exit 1; }

mkdir -p "$state" "$HOME/.config/hub"
cp "$root/dist/tools/desktop/hub.ico" "$state/hub.ico"
cat >"$HOME/.config/hub/desktop.env" <<CONF
# Written by tools/desktop/install.sh — re-run it to update.
CHROME='$chrome'
STATE='$state'
STATE_WIN='$state_win'
CONF

ps "\$s = (New-Object -ComObject WScript.Shell).CreateShortcut([IO.Path]::Combine([Environment]::GetFolderPath('Desktop'), 'hub.lnk'));
    \$s.TargetPath = \"\$env:WINDIR\\System32\\conhost.exe\";
    \$s.Arguments = '--headless ' + \"\$env:WINDIR\\System32\\wsl.exe\" + ' -d $WSL_DISTRO_NAME --cd $root -e bash tools/desktop/launch.sh';
    \$s.IconLocation = '$state_win\\hub.ico,0';
    \$s.WindowStyle = 7;
    \$s.Description = 'Open the hub (starts it if needed; closing the window stops it)';
    \$s.Save()"
echo "Installed the Desktop shortcut \"hub\" (Chrome: $chrome)."
