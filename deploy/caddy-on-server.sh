#!/usr/bin/env bash
# Run ON the server (already logged in). Adds modelsexam.com to the EXISTING Caddy without touching other sites.
# Safe: backs up the Caddyfile, validates before reloading (graceful reload, no downtime), rolls back on failure.
set -euo pipefail
DOMAIN="${DOMAIN:-modelsexam.com}"; PORT="${PORT:-8088}"; CF=/etc/caddy/Caddyfile
curl -fsS -o /dev/null "http://127.0.0.1:${PORT}/" || { echo "app not answering on 127.0.0.1:${PORT}; deploy it first"; exit 1; }
sudo grep -q "# modelsexam begin" "$CF" && { echo "already configured in $CF"; exit 0; }
sudo grep -qE "(^|[ ,])(www\.)?${DOMAIN//./\\.}([ ,{]|$)" "$CF" && { echo "$DOMAIN already appears in $CF; not touching"; exit 1; }
sudo cp "$CF" "$CF.bak.$(date +%s)"
sudo tee -a "$CF" >/dev/null <<CADDY

# modelsexam begin
${DOMAIN}, www.${DOMAIN} {
	encode zstd gzip
	reverse_proxy 127.0.0.1:${PORT} {
		flush_interval -1
	}
}
# modelsexam end
CADDY
if sudo caddy validate --config "$CF" --adapter caddyfile; then
  sudo systemctl reload caddy && echo "ok: https://${DOMAIN} (certificate is issued automatically once DNS points here)"
else
  echo "validation failed, restoring backup" >&2
  sudo cp "$(ls -t "$CF".bak.* | head -1)" "$CF"; exit 1
fi
