#!/usr/bin/env bash
# Generates a self-signed TLS certificate for local development.
# Output: certs/server.crt and certs/server.key
set -euo pipefail

CERTS_DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
mkdir -p "$CERTS_DIR"

openssl req -x509 \
  -newkey rsa:4096 \
  -keyout "$CERTS_DIR/server.key" \
  -out    "$CERTS_DIR/server.crt" \
  -days   365 \
  -nodes \
  -subj "/C=US/ST=Dev/L=Dev/O=GoPerMail/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"

chmod 600 "$CERTS_DIR/server.key"
echo "Self-signed cert written to $CERTS_DIR"
