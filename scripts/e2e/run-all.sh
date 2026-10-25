#!/usr/bin/env bash
# Corre los 4 escenarios en orden. Cada uno deja el caos como lo encontró.
set -euo pipefail
dir="$(dirname "$0")"
for s in 01-compra-feliz 02-timeout-pero-emitio 03-caida-emisor 04-reembolso; do
  bash "$dir/$s.sh"
done
printf '\n\033[1;32mLos 4 escenarios pasaron.\033[0m\n'
