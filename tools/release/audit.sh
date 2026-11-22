#!/usr/bin/env bash
# Fails if a release file isn't a clean copy (docs/adr/0011-releases.md):
# no personal data, secrets, databases or state from a build host or an
# earlier run; nothing that one install would share with another.
# Usage: tools/release/audit.sh <NAME>.wsl <NAME>-update.tar.gz
set -euo pipefail
wsl="${1:?usage: audit.sh <name>.wsl <name>-update.tar.gz}"
update="${2:?usage: audit.sh <name>.wsl <name>-update.tar.gz}"
fail=0
bad() { echo "audit: $*" >&2; fail=1; }

# Names that never belong in a release, anywhere.
forbidden='(^|/)(\.env|\.bash_history|\.git-credentials|\.netrc|desktop\.env|id_(rsa|ecdsa|ed25519)[^/]*|config\.json\.bak)$|(^|/)\.(ssh|aws|gnupg)/|(^|/)\.docker/config\.json$|\.(db|sqlite|sqlite3|db-wal|db-shm|pdf|kdbx)$'
# Where user or runtime state would live.
stateful='^(home/[^/]+/|root/|var/lib/docker/|var/lib/containerd/|tmp/|var/tmp/|opt/hub/data/)'

list() { tar -tzvf "$1" | awk '{size=$3; $1=$2=$3=$4=$5=""; sub(/^ +/, ""); sub(/^\.\//, ""); print size "\t" $0}'; }

check_tree() { # label, listing on stdin
  local label=$1 size path
  while IFS=$'\t' read -r size path; do
    [ -n "$path" ] || continue
    if [[ $path =~ $forbidden ]]; then bad "$label: forbidden file $path"; fi
    if [[ $path =~ $stateful && ! $path =~ /$ ]]; then
      case "$path" in
      home/hub/.bashrc | home/hub/.profile | home/hub/.bash_logout | root/.bashrc | root/.profile) ;;
      *) bad "$label: state file $path" ;;
      esac
    fi
    if [[ $path == var/log/* && ! $path =~ /$ && $size != 0 ]]; then bad "$label: non-empty log $path"; fi
    if [[ $path == etc/machine-id && $size != 0 ]]; then bad "$label: /etc/machine-id is set (installs would share it)"; fi
  done
}

# (Process substitution, not pipes: check_tree must run in this shell to set $fail.)
check_tree "$(basename "$wsl")" < <(list "$wsl")
check_tree "$(basename "$update")" < <(list "$update" | sed 's#\thub/#\topt/hub/#')

# Only the expected files ship in /opt/hub.
while IFS= read -r path; do
  case "$path" in
  hub/ | hub/bin/ | hub/bin/hub | hub/apps/ | hub/apps/*/ | hub/apps/*/app.manifest.yaml | hub/apps/*/compose.yaml | \
    hub/apps/*/*.svg | hub/apps/*/*.png | hub/images/ | hub/images/*.tar.gz | hub/images/*.tag | hub/launch.sh | \
    hub/splash.html | hub/first-run.sh | hub/update.sh | hub/hub.ico | hub/release.env) ;;
  *) bad "$(basename "$update"): unexpected file $path" ;;
  esac
done < <(tar -tzf "$update" | sed 's#^\./##')

# App images: no data, secrets or home files baked in.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
tar -xzf "$update" -C "$tmp" --wildcards 'hub/images/*'
for img in "$tmp"/hub/images/*.tar.gz; do
  tag="$(cat "${img%.tar.gz}.tag")"
  if ! docker image inspect "$tag" >/dev/null 2>&1; then gunzip -c "$img" | docker load -q >/dev/null; fi
  cid="$(docker create "$tag")"
  docker export "$cid" | gzip -1 >"$tmp/app.tar.gz"
  docker rm "$cid" >/dev/null
  check_tree "image $tag" < <(list "$tmp/app.tar.gz" | grep -E $'\t(data|home|root|tmp|var/tmp)/' || true)
  rm -f "$tmp/app.tar.gz"
done

if [ "$fail" -ne 0 ]; then
  echo "audit: FAILED — not a clean copy" >&2
  exit 1
fi
echo "audit: clean"
