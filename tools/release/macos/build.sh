#!/usr/bin/env bash
# Builds the macOS release of the hub (docs/adr/0012-macos-release.md): no
# Docker, apps run as native programs (`hub serve --native`), installed with
# Homebrew (Formula/<NAME>.rb).
#   dist/release/<NAME>/<NAME>-macos-<arch>.tar.gz   (arm64, amd64)
# Usage: tools/release/macos/build.sh <release> <version>
# TARGETS overrides the platforms (e.g. "linux/amd64" for a local smoke test).
set -euo pipefail
root="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$root"
rel="${1:?usage: build.sh <release> <version>}"
version="${2:?usage: build.sh <release> <version>}"
# shellcheck source=/dev/null
. "releases/$rel.env"
repo="${GITHUB_REPOSITORY:-${RELEASE_REPO:-}}"
[ -n "$repo" ] || { echo "set GITHUB_REPOSITORY or RELEASE_REPO (owner/repo)" >&2; exit 1; }
out="$root/dist/release/$NAME"
mkdir -p "$out"

echo "== web UIs"
pnpm nx run core:embed-shell --outputStyle=static >/dev/null
for app in $APPS; do pnpm nx run "$app-api:embed-ui" --outputStyle=static >/dev/null; done

for target in ${TARGETS:-darwin/arm64 darwin/amd64}; do
  os="${target%/*}" arch="${target#*/}"
  label="$os"; [ "$os" = darwin ] && label=macos
  name="$NAME-$label-$arch"
  stage="$out/stage/$NAME"
  rm -rf "$out/stage" && mkdir -p "$stage/bin" "$stage/apps"
  echo "== $name"
  (cd platform/core && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$stage/bin/hub" ./cmd/hub)
  for app in $APPS; do
    mkdir -p "$stage/apps/$app"
    (cd "apps/$app/api" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "$stage/apps/$app/$app" .)
    icon="$(sed -n 's/^icon: *\(.*\)$/\1/p' "apps/$app/app.manifest.yaml" | tr -d "'\"")"
    [ -n "$icon" ] && cp "apps/$app/$icon" "$stage/apps/$app/$icon"
    cp "apps/$app/compose.yaml" "$stage/apps/$app/" # the manifest names it; unused with --native
    # Native run: keep the manifest, auto-start, and replace the dev
    # processes with the built program (env from the app's compose file).
    env="$(docker compose -f "apps/$app/compose.yaml" config --no-interpolate --format json 2>/dev/null |
      python3 -c 'import json,sys
c=json.load(sys.stdin); s=next(iter(c["services"].values()))
e=s.get("environment") or {}
pairs = [f"{k}={v}" for k,v in e.items() if v is not None] if isinstance(e,dict) else [x for x in e if "=" in x]
print(" ".join(pairs))' || true)"
    python3 - "apps/$app/app.manifest.yaml" "$stage/apps/$app/app.manifest.yaml" "$app" "$env" <<'PY'
import re, sys
src, dst, app, env = sys.argv[1:5]
text = open(src).read()
text = re.sub(r"(?m)^autoStart:.*$", "autoStart: true", text)
if "autoStart:" not in text:
    text += "\nautoStart: true\n"
text = re.sub(r"(?ms)^(#[^\n]*\n)*dev:.*\Z", "", text).rstrip() + "\n"
cmd = (env + " " if env else "") + f"exec ./{app}"
text += f"""
# Native release (hub serve --native): the core runs this program directly.
dev:
  route: web
  processes:
    - name: web
      dir: .
      command: {cmd}
"""
open(dst, "w").write(text)
PY
  done
  cp tools/release/macos/launch.sh "$stage/launch.sh"
  cp tools/desktop/splash.html "$stage/"
  sed -e "s/@NAME@/$NAME/g" -e "s/@TITLE@/$TITLE/g" tools/release/macos/app.sh >"$stage/$NAME"
  chmod 0755 "$stage/launch.sh" "$stage/$NAME"
  (cd tools/desktop && go run . "$stage/$NAME.icns")
  cat >"$stage/release.env" <<CONF
NAME=$NAME
TITLE='$TITLE'
VERSION=$version
REPO=$repo
PORT=$PORT
CONF
  tar -C "$out/stage" -czf "$out/$name.tar.gz" "$NAME"
  rm -rf "$out/stage"
  tools/release/macos/audit.sh "$out/$name.tar.gz" "$NAME"
done
ls -lh "$out"
