#!/bin/sh
# Bootstraps a TLS certificate for *.knowoff.local before nginx starts.
#
# Precedence: a real certificate mounted at certs/external/{fullchain,privkey}.pem
# always wins. Otherwise a self-signed wildcard cert is generated once (and
# reused across restarts, since certs/ is a bind mount) at
# certs/selfsigned/. Either way the active pair is copied to certs/active/,
# which is the fixed path every vhost's ssl_certificate directive points at.
set -eu

CERT_ROOT=/etc/nginx/certs
EXTERNAL_DIR="$CERT_ROOT/external"
SELFSIGNED_DIR="$CERT_ROOT/selfsigned"
ACTIVE_DIR="$CERT_ROOT/active"

mkdir -p "$EXTERNAL_DIR" "$SELFSIGNED_DIR"

if [ -f "$EXTERNAL_DIR/fullchain.pem" ] && [ -f "$EXTERNAL_DIR/privkey.pem" ]; then
  echo "[nginx-entrypoint] using mounted certificate at $EXTERNAL_DIR"
  SOURCE_DIR="$EXTERNAL_DIR"
else
  if [ ! -f "$SELFSIGNED_DIR/fullchain.pem" ] || [ ! -f "$SELFSIGNED_DIR/privkey.pem" ]; then
    echo "[nginx-entrypoint] no certs/external/*.pem found — generating a self-signed dev certificate for *.knowoff.local"
    openssl req -x509 -nodes -newkey rsa:2048 -days 825 \
      -keyout "$SELFSIGNED_DIR/privkey.pem" \
      -out "$SELFSIGNED_DIR/fullchain.pem" \
      -subj "/CN=knowoff.local/O=Knowoff Dev" \
      -addext "subjectAltName=DNS:knowoff.local,DNS:*.knowoff.local"
  else
    echo "[nginx-entrypoint] reusing previously generated self-signed certificate"
  fi
  SOURCE_DIR="$SELFSIGNED_DIR"
fi

rm -rf "$ACTIVE_DIR"
mkdir -p "$ACTIVE_DIR"
cp "$SOURCE_DIR/fullchain.pem" "$ACTIVE_DIR/fullchain.pem"
cp "$SOURCE_DIR/privkey.pem" "$ACTIVE_DIR/privkey.pem"
chmod 600 "$ACTIVE_DIR/privkey.pem"

exec nginx -g "daemon off;"
