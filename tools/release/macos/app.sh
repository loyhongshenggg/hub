#!/bin/bash
# `@NAME@` command, installed by Homebrew (Formula/@NAME@.rb).
#   @NAME@          open @TITLE@ (first run also adds it to ~/Applications and the Dock)
#   @NAME@ stop     stop it
#   @NAME@ remove-app   remove the ~/Applications/@TITLE@.app shortcut
set -euo pipefail
# Homebrew links bin/<name> to libexec/<name>: follow the links to libexec.
src="$0"
while [ -L "$src" ]; do
  dir="$(cd "$(dirname "$src")" && pwd)"
  src="$(readlink "$src")"
  case "$src" in /*) ;; *) src="$dir/$src" ;; esac
done
here="$(cd "$(dirname "$src")" && pwd)"
# The Dock app must not point into Cellar/<name>/<version>: `brew upgrade`
# deletes that folder. Homebrew's opt/<name> link survives upgrades.
stable="$here"
case "$here" in
*/Cellar/*/*/libexec) [ -d "${here%%/Cellar/*}/opt/@NAME@/libexec" ] && stable="${here%%/Cellar/*}/opt/@NAME@/libexec" ;;
esac
# shellcheck source=/dev/null
. "$here/release.env"
app="$HOME/Applications/$TITLE.app"

make_app() {
  # Created on this Mac (not downloaded), so Gatekeeper has nothing to quarantine.
  rm -rf "$app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  cp "$here/$NAME.icns" "$app/Contents/Resources/$NAME.icns"
  cat >"$app/Contents/MacOS/$NAME" <<SH
#!/bin/bash
exec "$stable/launch.sh"
SH
  chmod 0755 "$app/Contents/MacOS/$NAME"
  cat >"$app/Contents/Info.plist" <<PL
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>$TITLE</string>
  <key>CFBundleDisplayName</key><string>$TITLE</string>
  <key>CFBundleIdentifier</key><string>localhost.hub.$NAME</string>
  <key>CFBundleExecutable</key><string>$NAME</string>
  <key>CFBundleIconFile</key><string>$NAME</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
</dict></plist>
PL
  # Pin it to the Dock once.
  if ! defaults read com.apple.dock persistent-apps 2>/dev/null | grep -q "$TITLE.app"; then
    defaults write com.apple.dock persistent-apps -array-add \
      "<dict><key>tile-data</key><dict><key>file-data</key><dict><key>_CFURLString</key><string>file://$app/</string><key>_CFURLStringType</key><integer>15</integer></dict></dict></dict>"
    killall Dock 2>/dev/null || true
  fi
}

case "${1:-open}" in
open)
  if [ ! -d "$app" ] || ! grep -q "$stable/launch.sh" "$app/Contents/MacOS/$NAME" 2>/dev/null; then
    make_app
    echo "Added $TITLE to ~/Applications and the Dock."
  fi
  open "$app"
  ;;
stop) HUB_ADDR="127.0.0.1:$PORT" "$here/bin/hub" stop ;;
remove-app) rm -rf "$app" && echo "Removed $app (remove it from the Dock by dragging it out)." ;;
*) echo "usage: $NAME [open|stop|remove-app]" >&2; exit 2 ;;
esac
