#!/usr/bin/env bash
# Provision the hub dev toolchain inside WSL Ubuntu. Run as root:
#   sudo bash tools/setup/wsl-toolchain.sh <linux-username> <git-email>
# Idempotent: safe to re-run.
set -euo pipefail

TARGET_USER="${1:?usage: wsl-toolchain.sh <linux-username> <git-email>}"
GIT_EMAIL="${2:-}"
NODE_MAJOR=26
export DEBIAN_FRONTEND=noninteractive

id "$TARGET_USER" >/dev/null 2>&1 || { echo "user $TARGET_USER does not exist"; exit 1; }
step() { echo; echo "=== $* ==="; }

step "1/5 apt base packages"
apt-get update -q
apt-get upgrade -yq
apt-get install -yq ca-certificates curl git jq unzip xz-utils build-essential
# Finance app (Dev mode): render statement PDFs and OCR them.
apt-get install -yq poppler-utils tesseract-ocr tesseract-ocr-eng

step "2/5 Go (latest stable from go.dev, sha256-verified)"
GO_JSON="$(curl -fsSL 'https://go.dev/dl/?mode=json')"
GO_VER="$(jq -r '[.[] | select(.stable)][0].version' <<<"$GO_JSON")"
if [ "$(/usr/local/go/bin/go version 2>/dev/null | awk '{print $3}')" = "$GO_VER" ]; then
  echo "$GO_VER already installed"
else
  GO_FILE="$(jq -r --arg v "$GO_VER" '.[] | select(.version==$v) | .files[] | select(.os=="linux" and .arch=="amd64" and .kind=="archive") | .filename' <<<"$GO_JSON")"
  GO_SHA="$(jq -r --arg v "$GO_VER" '.[] | select(.version==$v) | .files[] | select(.os=="linux" and .arch=="amd64" and .kind=="archive") | .sha256' <<<"$GO_JSON")"
  curl -fsSLo "/tmp/$GO_FILE" "https://go.dev/dl/$GO_FILE"
  echo "$GO_SHA  /tmp/$GO_FILE" | sha256sum -c -
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "/tmp/$GO_FILE"
  rm -f "/tmp/$GO_FILE"
fi
ln -sf /usr/local/go/bin/go /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

step "3/5 Node.js $NODE_MAJOR LTS (nodejs.org tarball, sha256-verified) + pnpm"
NODE_BASE="https://nodejs.org/dist/latest-v${NODE_MAJOR}.x"
SHAS="$(curl -fsSL "$NODE_BASE/SHASUMS256.txt")"
NODE_FILE="$(awk '/linux-x64\.tar\.xz$/{print $2}' <<<"$SHAS")"
NODE_SHA="$(awk '/linux-x64\.tar\.xz$/{print $1}' <<<"$SHAS")"
NODE_WANT="$(sed -E 's/^node-(v[0-9.]+)-.*/\1/' <<<"$NODE_FILE")"
if [ "$(/usr/local/bin/node --version 2>/dev/null || true)" = "$NODE_WANT" ]; then
  echo "node $NODE_WANT already installed"
else
  curl -fsSLo "/tmp/$NODE_FILE" "$NODE_BASE/$NODE_FILE"
  echo "$NODE_SHA  /tmp/$NODE_FILE" | sha256sum -c -
  tar -xJf "/tmp/$NODE_FILE" -C /usr/local --strip-components=1 \
    --exclude='*/CHANGELOG.md' --exclude='*/LICENSE' --exclude='*/README.md'
  rm -f "/tmp/$NODE_FILE"
fi
npm install -g pnpm@latest --no-fund --no-audit

step "4/5 Docker Engine + Compose plugin (Docker's official apt repo)"
. /etc/os-release
CODENAME="${UBUNTU_CODENAME:-$VERSION_CODENAME}"
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat >/etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $CODENAME
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update -q
apt-get install -yq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
usermod -aG docker "$TARGET_USER"
if [ -d /run/systemd/system ]; then
  systemctl enable --now docker
else
  echo "WARNING: systemd not running; docker service not enabled"
fi

step "5/5 per-user config for $TARGET_USER"
sudo -u "$TARGET_USER" git config --global init.defaultBranch main
sudo -u "$TARGET_USER" git config --global user.name >/dev/null || sudo -u "$TARGET_USER" git config --global user.name "$TARGET_USER"
if [ -n "$GIT_EMAIL" ]; then sudo -u "$TARGET_USER" git config --global user.email "$GIT_EMAIL"; fi
BASHRC="/home/$TARGET_USER/.bashrc"
grep -q 'go/bin' "$BASHRC" || echo 'export PATH="$PATH:$HOME/go/bin"' >>"$BASHRC"
sudo -u "$TARGET_USER" mkdir -p "/home/$TARGET_USER/code"

step "versions"
echo "ubuntu : $PRETTY_NAME"
echo "systemd: $([ -d /run/systemd/system ] && echo running || echo NOT running)"
git --version
go version
echo "node   : $(node --version)"
echo "npm    : $(npm --version)"
echo "pnpm   : $(pnpm --version)"
docker --version
docker compose version
echo "DONE"
