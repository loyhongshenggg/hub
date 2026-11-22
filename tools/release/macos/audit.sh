#!/usr/bin/env bash
# A macOS release tarball holds exactly the expected files: no data,
# secrets or build-host leftovers (docs/adr/0012-macos-release.md).
set -euo pipefail
file="${1:?usage: audit.sh <tarball> <name>}"
name="${2:?usage: audit.sh <tarball> <name>}"
fail=0
while IFS= read -r p; do
  p="${p#./}"
  case "$p" in
  "$name/" | "$name/bin/" | "$name/bin/hub" | "$name/apps/" | "$name/apps/"*/ | "$name/apps/"*/app.manifest.yaml | "$name/apps/"*/compose.yaml | \
    "$name/apps/"*/*.svg | "$name/apps/"*/*.png | "$name/launch.sh" | "$name/splash.html" | "$name/release.env" | \
    "$name/$name" | "$name/$name.icns") ;;
  "$name/apps/"*/*)
    # The app's own program (same name as its folder), nothing else.
    a="${p#"$name/apps/"}"
    [ "${a%%/*}" = "${a#*/}" ] || { echo "audit: unexpected file $p" >&2; fail=1; } ;;
  *) echo "audit: unexpected file $p" >&2; fail=1 ;;
  esac
done < <(tar -tzf "$file")
[ "$fail" -eq 0 ] && echo "audit: $(basename "$file") clean" || { echo "audit: FAILED" >&2; exit 1; }
