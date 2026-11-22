#!/usr/bin/env bash
# Builds a Windows release of the hub (docs/adr/0011-releases.md):
#   dist/release/<NAME>/<NAME>.wsl              double-click to install
#   dist/release/<NAME>/<NAME>-update.tar.gz    in-place update (data kept)
#   dist/release/<NAME>/SHA256SUMS
# Usage: tools/release/build.sh <release> <version>    e.g. finance 0.1.0
# Needs Docker, Go, pnpm. GITHUB_REPOSITORY (owner/repo) is baked in for updates.
# DOCKER_BUILD_FLAGS is passed to every docker build (e.g. --network=host).
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root"
rel="${1:?usage: build.sh <release> <version>}"
version="${2:?usage: build.sh <release> <version>}"
# shellcheck source=/dev/null
. "releases/$rel.env"
repo="${GITHUB_REPOSITORY:-${RELEASE_REPO:-}}"
[ -n "$repo" ] || { echo "set GITHUB_REPOSITORY or RELEASE_REPO (owner/repo) for updates" >&2; exit 1; }

out="$root/dist/release/$NAME"
stage="$out/stage"
hub="$stage/opt/hub"
rm -rf "$out"
mkdir -p "$hub/bin" "$hub/apps" "$hub/images" "$stage/distro"

echo "== core"
pnpm nx build core --outputStyle=static >/dev/null
install -m 0755 dist/platform/core/hub "$hub/bin/hub"

for app in $APPS; do
  echo "== app $app"
  tag="hs-hub/$app:$version"
  # shellcheck disable=SC2086
  docker build -q ${DOCKER_BUILD_FLAGS:-} -f "apps/$app/Dockerfile" -t "$tag" . >/dev/null
  docker save "$tag" | gzip -1 >"$hub/images/$app.tar.gz"
  echo "$tag" >"$hub/images/$app.tag"
  mkdir -p "$hub/apps/$app"
  # Shipped apps start with the hub: opening the icon is opening the app.
  sed -e 's/^autoStart:.*$/autoStart: true/' "apps/$app/app.manifest.yaml" >"$hub/apps/$app/app.manifest.yaml"
  grep -q '^autoStart:' "$hub/apps/$app/app.manifest.yaml" || echo 'autoStart: true' >>"$hub/apps/$app/app.manifest.yaml"
  icon="$(sed -n 's/^icon: *\(.*\)$/\1/p' "apps/$app/app.manifest.yaml" | tr -d "'\"")"
  [ -n "$icon" ] && cp "apps/$app/$icon" "$hub/apps/$app/$icon"
  # The release runs the built image instead of building from source.
  docker compose -f "apps/$app/compose.yaml" config --no-interpolate --format json |
    TAG="$tag" python3 -c '
import json, os, sys
c = json.load(sys.stdin)
c.pop("name", None)
for kind in ("networks", "volumes"):  # let the project name scope them again
    for v in (c.get(kind) or {}).values():
        if isinstance(v, dict):
            v.pop("name", None)
for s in c.get("services", {}).values():
    s.pop("build", None)
    s["image"] = os.environ["TAG"]
    s["pull_policy"] = "never"
json.dump(c, sys.stdout, indent=2)' >"$hub/apps/$app/compose.yaml"
done

echo "== scripts"
cp tools/desktop/launch.sh tools/desktop/splash.html tools/release/distro/first-run.sh tools/release/distro/update.sh "$hub/"
chmod 0755 "$hub"/*.sh
(cd tools/desktop && go run . "$hub/hub.ico")
cat >"$hub/release.env" <<CONF
NAME=$NAME
TITLE='$TITLE'
VERSION=$version
REPO=$repo
PORT=$PORT
CONF

echo "== update bundle"
tar -C "$stage/opt" -czf "$out/$NAME-update.tar.gz" hub

echo "== distribution"
cp tools/release/distro/Dockerfile tools/release/distro/wsl.conf tools/release/distro/daemon.json \
  tools/release/distro/net.sh tools/release/distro/docker-net.conf "$stage/distro/"
sed -e "s/@NAME@/$NAME/g" tools/release/distro/wsl-distribution.conf >"$stage/distro/wsl-distribution.conf"
# shellcheck disable=SC2086
docker build -q ${DOCKER_BUILD_FLAGS:-} -f "$stage/distro/Dockerfile" -t "hs-hub/$NAME-rootfs:$version" "$stage" >/dev/null
cid="$(docker create "hs-hub/$NAME-rootfs:$version")"
docker export "$cid" | gzip -6 >"$out/$NAME.wsl"
docker rm "$cid" >/dev/null

echo "== audit"
tools/release/audit.sh "$out/$NAME.wsl" "$out/$NAME-update.tar.gz"

(cd "$out" && sha256sum "$NAME.wsl" "$NAME-update.tar.gz" >SHA256SUMS)
rm -rf "$stage"
ls -lh "$out"
