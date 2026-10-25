#!/usr/bin/env bash
# Corre los 4 escenarios en orden. Cada uno deja el caos como lo encontró.
set -euo pipefail
dir="$(dirname "$0")"
# Un solo login para los 4 escenarios (el login admite 5 intentos seguidos por IP).
E2E_TOKEN="$(source "$dir/lib.sh" && wait_ready && login && echo "$TOKEN")"
export E2E_TOKEN
for s in 01-compra-feliz 02-timeout-pero-emitio 03-caida-emisor 04-reembolso; do
  bash "$dir/$s.sh"
done
printf '\n\033[1;32mLos 4 escenarios pasaron.\033[0m\n'
