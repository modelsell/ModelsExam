#!/usr/bin/env bash
# Usage: [PORT=8088] [SITE_URL=http://host:8088] [INSTALL_DOCKER=1] ./deploy/deploy.sh [host]
# Defaults may come from a local, git-ignored .env.deploy (DEPLOY_HOST, DEPLOY_USER,
# DEPLOY_PASSWORD, SITE_URL, TRUSTED_PROXIES, MODEL_CHECK_CLOUDFLARE, AUTH_REQUIRE_HTTPS);
# with DEPLOY_PASSWORD set, sshpass supplies the password.
# Behind a reverse proxy on the same host (Caddy/nginx -> 127.0.0.1:PORT) the container
# sees the proxy as the docker bridge gateway, so TRUSTED_PROXIES defaults to 172.17.0.1/32;
# without it the app cannot tell HTTPS requests apart and refuses sign-in.
# MODEL_CHECK_SECRET_KEY (in .env.deploy) is the master key that encrypts saved API keys.
# It is sent over stdin to /opt/modelsexam-secrets/secret.key (root, 600) on the first
# deploy, mounted read-only into the container, and never placed in the data directory or
# its backups. A different key than the one on the server is refused (use rotate-key).
# Isolated deploy: touches only /opt/modelsexam-* and the container named "modelsexam".
# It never stops/removes other containers or services, never edits nginx/firewall,
# and refuses to start if the chosen host port is used by anything other than our
# own "modelsexam" container. A redeploy builds first, then stops the old container
# and starts the new one (a few seconds of downtime); /opt/modelsexam-data is kept.
# Auth: your SSH key, or type the ssh/sudo password when ssh asks (never put it in this file).
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -f .env.deploy ]; then
  set -a; . ./.env.deploy; set +a
fi
HOST="${1:-${DEPLOY_HOST:?Usage: ./deploy/deploy.sh host (or set DEPLOY_HOST in .env.deploy)}}"
SSH_USER="${SSH_USER:-${DEPLOY_USER:-ubuntu}}"   # non-root login; remote steps use sudo
PORT="${PORT:-8088}"
SITE_URL="${SITE_URL:-http://$HOST:$PORT}"
TRUSTED_PROXIES="${TRUSTED_PROXIES-172.17.0.1/32}"
AUTH_REQUIRE_HTTPS="${AUTH_REQUIRE_HTTPS:-true}"
MODEL_CHECK_CLOUDFLARE="${MODEL_CHECK_CLOUDFLARE:-false}"
# One SSH connection for every step, so a password is asked for only once.
SSH_OPTS=(-o ControlMaster=auto -o "ControlPath=$HOME/.ssh/modelsexam-%r@%h:%p" -o ControlPersist=10m)
if [ -n "${DEPLOY_PASSWORD:-}" ]; then
  command -v sshpass >/dev/null || { echo "ERROR: DEPLOY_PASSWORD is set but sshpass is not installed" >&2; exit 1; }
  export SSHPASS="$DEPLOY_PASSWORD"   # read by sshpass -e; never put on the command line
  ssh() { sshpass -e ssh "${SSH_OPTS[@]}" "$@"; }
else
  ssh() { command ssh "${SSH_OPTS[@]}" "$@"; }
fi

echo ">> preflight (read-only)"
ssh "$SSH_USER@$HOST" sudo env PORT="$PORT" INSTALL_DOCKER="${INSTALL_DOCKER:-0}" bash -s <<'REMOTE'
set -eu
if ss -ltn | awk '{print $4}' | grep -qE "[:.]${PORT}\$"; then
  # A redeploy finds our own container on the port; anything else is refused.
  if command -v docker >/dev/null && docker port modelsexam 2>/dev/null | grep -qE "[:.]${PORT}\$"; then
    echo "ok: port ${PORT} is held by the current modelsexam container (redeploy)"
  else
    echo "ERROR: port ${PORT} is already in use; pick another with PORT=..." >&2; exit 1
  fi
else
  echo "ok: port ${PORT} free"
fi
if ! command -v docker >/dev/null; then
  [ "$INSTALL_DOCKER" = 1 ] || { echo "ERROR: docker not installed. Re-run with INSTALL_DOCKER=1 to allow installing it (adds docker iptables rules; does not touch existing services)." >&2; exit 1; }
fi
df -h /opt | tail -1; free -m | sed -n 2p
REMOTE

echo ">> master key for saved API keys"
printf '%s\n' "${MODEL_CHECK_SECRET_KEY:-}" | ssh "$SSH_USER@$HOST" 'sudo bash -c '"'"'
set -eu
install -d -m 700 /opt/modelsexam-secrets
key=/opt/modelsexam-secrets/secret.key
umask 077
cat > "$key.new"
if [ "$(tr -d "[:space:]" < "$key.new")" = "" ]; then
  rm -f "$key.new"
  [ -s "$key" ] || { echo "ERROR: no master key on the server; set MODEL_CHECK_SECRET_KEY in .env.deploy (openssl rand -base64 32)" >&2; exit 1; }
  echo "ok: using the master key already on the server"
elif [ -s "$key" ]; then
  if cmp -s "$key" "$key.new"; then rm -f "$key.new"; echo "ok: master key unchanged"
  else rm -f "$key.new"; echo "ERROR: MODEL_CHECK_SECRET_KEY differs from the key on the server; rotate with modelsexam credentials rotate-key" >&2; exit 1; fi
else
  mv "$key.new" "$key"; chmod 600 "$key"; echo "ok: master key installed"
fi
'"'"''

echo ">> upload source"
tar --exclude=.git --exclude=web/node_modules --exclude=web/dist --exclude=data --exclude='*.db' --exclude='.env*' -czf - . \
  | ssh "$SSH_USER@$HOST" 'sudo rm -rf /opt/modelsexam-src && sudo mkdir -p /opt/modelsexam-src && sudo tar -xzf - -C /opt/modelsexam-src'

echo ">> build and run"
ssh "$SSH_USER@$HOST" sudo env PORT="$PORT" SITE_URL="$SITE_URL" TRUSTED_PROXIES="$TRUSTED_PROXIES" AUTH_REQUIRE_HTTPS="$AUTH_REQUIRE_HTTPS" \
  MODEL_CHECK_CLOUDFLARE="$MODEL_CHECK_CLOUDFLARE" INSTALL_DOCKER="${INSTALL_DOCKER:-0}" bash -s <<'REMOTE'
set -euo pipefail
command -v docker >/dev/null || curl -fsSL https://get.docker.com | sh
cd /opt/modelsexam-src
# low priority so the build doesn't starve other apps
nice -n 19 docker build -t modelsexam .
docker stop modelsexam 2>/dev/null || true   # only our own container
docker rm modelsexam 2>/dev/null || true
mkdir -p /opt/modelsexam-data
# Back up the database while no container writes to it. It holds saved API
# keys (encrypted; the master key is not in the data directory), so backups are root-only.
if ls /opt/modelsexam-data/*.db >/dev/null 2>&1; then
  install -d -m 700 /opt/modelsexam-backups
  stamp=$(date +%Y%m%d-%H%M%S)
  tar -C /opt/modelsexam-data -czf "/opt/modelsexam-backups/data-$stamp.tgz" .
  chmod 600 "/opt/modelsexam-backups/data-$stamp.tgz"
  echo "backup: /opt/modelsexam-backups/data-$stamp.tgz"
fi
docker run -d --name modelsexam --restart unless-stopped \
  --memory 512m --cpus 1 \
  -p "127.0.0.1:${PORT}:8080" -v /opt/modelsexam-data:/data \
  -e MODEL_CHECK_SITE_URL="$SITE_URL" -e TRUSTED_PROXIES="${TRUSTED_PROXIES:-}" \
  -e AUTH_REQUIRE_HTTPS="$AUTH_REQUIRE_HTTPS" -e MODEL_CHECK_CLOUDFLARE="$MODEL_CHECK_CLOUDFLARE" \
  -v /opt/modelsexam-secrets/secret.key:/secrets/secret.key:ro -e MODEL_CHECK_SECRET_KEY_FILE=/secrets/secret.key \
  modelsexam
for i in $(seq 1 15); do
  curl -fsS -o /dev/null "http://127.0.0.1:${PORT}/api/status" && break
  sleep 1
done
curl -fsS -o /dev/null -w "health: HTTP %{http_code}\n" "http://127.0.0.1:${PORT}/api/status"
REMOTE
echo ">> done: $SITE_URL  (open the port in your cloud firewall if needed)"
