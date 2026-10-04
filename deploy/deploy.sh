#!/usr/bin/env bash
# Usage: [PORT=8088] [SITE_URL=http://host:8088] [INSTALL_DOCKER=1] ./deploy/deploy.sh host
# Isolated deploy: touches only /opt/modelsexam-* and the container named "modelsexam".
# It never stops/removes other containers or services, never edits nginx/firewall,
# and refuses to start if the chosen host port is already in use.
# Auth: your SSH key, or type the ssh/sudo password when ssh asks (never put it in this file).
set -euo pipefail
HOST="${1:?Usage: ./deploy/deploy.sh host}"
SSH_USER="${SSH_USER:-ubuntu}"   # non-root login; remote steps use sudo
PORT="${PORT:-8088}"
SITE_URL="${SITE_URL:-http://$HOST:$PORT}"
cd "$(dirname "$0")/.."

echo ">> preflight (read-only)"
ssh "$SSH_USER@$HOST" sudo env PORT="$PORT" INSTALL_DOCKER="${INSTALL_DOCKER:-0}" bash -s <<'REMOTE'
set -eu
if ss -ltn | awk '{print $4}' | grep -qE "[:.]${PORT}\$"; then
  echo "ERROR: port ${PORT} is already in use; pick another with PORT=..." >&2; exit 1
fi
if ! command -v docker >/dev/null; then
  [ "$INSTALL_DOCKER" = 1 ] || { echo "ERROR: docker not installed. Re-run with INSTALL_DOCKER=1 to allow installing it (adds docker iptables rules; does not touch existing services)." >&2; exit 1; }
fi
echo "ok: port ${PORT} free"; df -h /opt | tail -1; free -m | sed -n 2p
REMOTE

echo ">> upload source"
tar --exclude=.git --exclude=web/node_modules --exclude=web/dist --exclude=data --exclude='*.db' --exclude='.env*' -czf - . \
  | ssh "$SSH_USER@$HOST" 'sudo rm -rf /opt/modelsexam-src && sudo mkdir -p /opt/modelsexam-src && sudo tar -xzf - -C /opt/modelsexam-src'

echo ">> build and run"
ssh "$SSH_USER@$HOST" sudo env PORT="$PORT" SITE_URL="$SITE_URL" TRUSTED_PROXIES="${TRUSTED_PROXIES:-}" INSTALL_DOCKER="${INSTALL_DOCKER:-0}" bash -s <<'REMOTE'
set -euo pipefail
command -v docker >/dev/null || curl -fsSL https://get.docker.com | sh
cd /opt/modelsexam-src
# low priority so the build doesn't starve other apps
nice -n 19 docker build -t modelsexam .
docker rm -f modelsexam 2>/dev/null || true   # only our own container
mkdir -p /opt/modelsexam-data
docker run -d --name modelsexam --restart unless-stopped \
  --memory 512m --cpus 1 \
  -p "127.0.0.1:${PORT}:8080" -v /opt/modelsexam-data:/data \
  -e MODEL_CHECK_SITE_URL="$SITE_URL" -e TRUSTED_PROXIES="${TRUSTED_PROXIES:-}" \
  modelsexam
sleep 3
curl -fsS -o /dev/null -w "health: HTTP %{http_code}\n" "http://127.0.0.1:${PORT}/"
REMOTE
echo ">> done: $SITE_URL  (open the port in your cloud firewall if needed)"
