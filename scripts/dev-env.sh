#!/usr/bin/env bash
# Crea el .env de la raíz a partir de .env.example y genera los secretos que la plantilla deja vacíos
# (JWT_SECRET, PSP_WEBHOOK_SECRET, OPS_BOOTSTRAP_PASSWORD). Si el .env ya existe, no lo toca.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
env_file="$root/.env"

if [[ -f "$env_file" ]]; then
  echo ".env ya existe; no se modifica."
  exit 0
fi

command -v openssl >/dev/null || { echo "falta openssl" >&2; exit 1; }

secret() { openssl rand -hex 32; }
password() { openssl rand -base64 24 | tr -dc 'A-Za-z0-9' | cut -c1-20; }

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%$'\r'}" # tolera una plantilla con fin de línea CRLF (clon en Windows)
  case "$line" in
    JWT_SECRET=) echo "JWT_SECRET=$(secret)" ;;
    PSP_WEBHOOK_SECRET=) echo "PSP_WEBHOOK_SECRET=$(secret)" ;;
    OPS_BOOTSTRAP_PASSWORD=) echo "OPS_BOOTSTRAP_PASSWORD=$(password)" ;;
    *) echo "$line" ;;
  esac
done < "$root/.env.example" > "$env_file"

echo ".env creado con secretos aleatorios. La contraseña del operador está en OPS_BOOTSTRAP_PASSWORD."
