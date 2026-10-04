#!/usr/bin/env bash
# Usage: [DOMAIN=your-domain.com] [PORT=8088] [SSH_USER=ubuntu] ./deploy/domain.sh host
# Points DOMAIN at the modelsexam container (127.0.0.1:PORT) with HTTPS, without disturbing other apps:
#  - nginx already installed  -> adds ONE new server block file, runs `nginx -t`, reloads only if valid
#                                (rolls the new file back if the test fails). Existing sites are untouched.
#  - nothing on 80/443        -> installs Caddy (automatic HTTPS) with a dedicated config snippet.
#  - something else on 80/443 -> stops and tells you; never kills or replaces it.
# Prerequisite: DNS A records for DOMAIN and www.DOMAIN -> the server IP (certificates need this).
set -euo pipefail
HOST="${1:?Usage: ./deploy/domain.sh host}"
SSH_USER="${SSH_USER:-ubuntu}"
DOMAIN="${DOMAIN:-modelsexam.com}"
PORT="${PORT:-8088}"

ssh "$SSH_USER@$HOST" sudo env DOMAIN="$DOMAIN" PORT="$PORT" bash -s <<'REMOTE'
set -euo pipefail
curl -fsS -o /dev/null "http://127.0.0.1:${PORT}/" || { echo "ERROR: app not answering on 127.0.0.1:${PORT}; deploy first" >&2; exit 1; }
SERVER_IP=$(curl -fsS https://api.ipify.org || true)
DNS_IP=$(getent hosts "$DOMAIN" | awk '{print $1; exit}' || true)
echo "server ip: ${SERVER_IP:-?}  dns($DOMAIN): ${DNS_IP:-none}"
[ -z "$SERVER_IP" ] || [ "$SERVER_IP" = "$DNS_IP" ] || echo "WARNING: DNS does not point here yet; HTTPS issuance will fail until it does."

listeners=$(ss -ltnp 2>/dev/null | awk '$4 ~ /[:.](80|443)$/' || true)
if command -v nginx >/dev/null; then
  CONF=/etc/nginx/conf.d/modelsexam.conf
  [ -d /etc/nginx/conf.d ] || { echo "ERROR: /etc/nginx/conf.d missing" >&2; exit 1; }
  [ -e "$CONF" ] && cp "$CONF" "$CONF.bak"
  cat > "$CONF" <<NGX
server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    client_max_body_size 5m;
    location / {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$remote_addr;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_buffering off;          # keeps SSE check progress streaming
        proxy_read_timeout 600s;
    }
}
NGX
  if nginx -t; then systemctl reload nginx; echo "nginx reloaded"
  else echo "nginx -t failed, rolling back" >&2
       if [ -e "$CONF.bak" ]; then mv "$CONF.bak" "$CONF"; else rm -f "$CONF"; fi; exit 1; fi
  if command -v certbot >/dev/null; then
    certbot --nginx -d "$DOMAIN" -d "www.$DOMAIN" --non-interactive --agree-tos --register-unsafely-without-email --redirect \
      || echo "certbot failed (usually DNS not ready); site works over http meanwhile" >&2
  else
    echo "certbot not installed: for HTTPS run  sudo apt install -y certbot python3-certbot-nginx  then rerun this script"
  fi
elif [ -z "$listeners" ]; then
  apt-get install -y caddy >/dev/null
  printf '%s, www.%s {\n    reverse_proxy 127.0.0.1:%s {\n        flush_interval -1\n    }\n}\n' "$DOMAIN" "$DOMAIN" "$PORT" > /etc/caddy/Caddyfile
  systemctl enable --now caddy && systemctl reload caddy
else
  echo "ERROR: something non-nginx already uses 80/443:" >&2; echo "$listeners" >&2
  echo "Not touching it. Point that proxy at 127.0.0.1:${PORT} for host ${DOMAIN} instead." >&2; exit 1
fi
echo "done: https://${DOMAIN}"
REMOTE
echo ">> then set MODEL_CHECK_SITE_URL and TRUSTED_PROXIES:  SITE_URL=https://$DOMAIN TRUSTED_PROXIES=127.0.0.1/32 ./deploy/deploy.sh $HOST"
