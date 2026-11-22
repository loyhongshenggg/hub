#!/usr/bin/env bash
# First launch of a hub release distribution (WSL runs this once, as root,
# right after <NAME>.wsl is installed). Puts the app's icon on the Windows
# Desktop. Safe to re-run: wsl -d <NAME> -u root -e /opt/hub/first-run.sh
set -u
# shellcheck source=/dev/null
. /opt/hub/release.env
distro="${WSL_DISTRO_NAME:-$NAME}"
home=/home/hub

ps="$(command -v powershell.exe || echo /mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe)"
pwsh() { "$ps" -NoProfile -NonInteractive -Command "$1" | tr -d '\r'; }

local_win="$(pwsh '[Environment]::GetFolderPath("LocalApplicationData")')"
if [ -z "$local_win" ]; then
  echo "Couldn't reach Windows from WSL (interop is off?). Open $TITLE with: wsl -d $distro -e /opt/hub/launch.sh"
  exit 0
fi
state_win="$local_win\\$NAME"
state="$(wslpath -u "$state_win")"

# Chrome if installed, else Edge (always on Windows 10/11). Both have app windows.
browser=""
for p in "/mnt/c/Program Files/Google/Chrome/Application/chrome.exe" \
  "/mnt/c/Program Files (x86)/Google/Chrome/Application/chrome.exe" \
  "$(wslpath -u "$local_win")/Google/Chrome/Application/chrome.exe" \
  "/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" \
  "/mnt/c/Program Files/Microsoft/Edge/Application/msedge.exe"; do
  [ -f "$p" ] && { browser="$p"; break; }
done

mkdir -p "$state" "$home/.config/hub"
cp /opt/hub/hub.ico "$state/$NAME.ico"
cat >"$home/.config/hub/desktop.env" <<CONF
# Written by /opt/hub/first-run.sh.
CHROME='$browser'
STATE='$state'
STATE_WIN='$state_win'
HUB_ADDR=127.0.0.1:$PORT
CONF
chown -R hub:hub "$home/.config"

pwsh "\$s = (New-Object -ComObject WScript.Shell).CreateShortcut([IO.Path]::Combine([Environment]::GetFolderPath('Desktop'), '$TITLE.lnk'));
  \$s.TargetPath = \"\$env:WINDIR\\System32\\conhost.exe\";
  \$s.Arguments = '--headless ' + \"\$env:WINDIR\\System32\\wsl.exe\" + ' -d $distro -e bash /opt/hub/launch.sh';
  \$s.IconLocation = '$state_win\\$NAME.ico,0';
  \$s.WindowStyle = 7;
  \$s.Description = 'Open $TITLE';
  \$s.Save()"

echo
echo "  $TITLE $VERSION is installed."
echo "  Open it with the \"$TITLE\" icon on your Desktop (the first start takes a minute)."
[ -n "$browser" ] || echo "  Install Google Chrome or Microsoft Edge first, then run: wsl -d $distro -u root -e /opt/hub/first-run.sh"
echo
