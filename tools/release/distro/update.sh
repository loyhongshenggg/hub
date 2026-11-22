#!/usr/bin/env bash
# Updates this install to the newest release of the same name, keeping its
# data (docs/adr/0011-releases.md). From Windows:  wsl -d <NAME> -e /opt/hub/update.sh
set -euo pipefail
# shellcheck source=/dev/null
. /opt/hub/release.env

tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=50" |
  grep -o "\"tag_name\": *\"$NAME-v[^\"]*\"" | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
[ -n "$tag" ] || { echo "No $NAME release found in $REPO." >&2; exit 1; }
latest="${tag#"$NAME"-v}"
if [ "$latest" = "$VERSION" ]; then
  echo "$TITLE $VERSION is up to date."
  exit 0
fi

echo "Updating $TITLE $VERSION → $latest…"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
base="https://github.com/$REPO/releases/download/$tag"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS"
curl -fL --progress-bar -o "$tmp/$NAME-update.tar.gz" "$base/$NAME-update.tar.gz"
(cd "$tmp" && grep " $NAME-update.tar.gz\$" SHA256SUMS | sha256sum -c --quiet -)

# Stop the app if it is open, then swap in the new files.
HUB_ADDR="127.0.0.1:$PORT" /opt/hub/bin/hub stop >/dev/null 2>&1 || true
mkdir "$tmp/new"
tar -C "$tmp/new" -xzf "$tmp/$NAME-update.tar.gz"
cp -a "$tmp/new/hub/." /opt/hub/
echo "Updated to $latest. Open $TITLE from its Desktop icon."
