#!/usr/bin/env bash
# Funciones comunes de los escenarios E2E. Se carga con `source`; no se ejecuta sola.
# Requiere el sistema arriba (`docker compose up --build`), curl y jq.
set -euo pipefail

ORCH="${ORCHESTRATOR_URL:-http://localhost:8080}"
SIM="${SIMULATOR_URL:-http://localhost:8090}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

for tool in curl jq; do
  command -v "$tool" >/dev/null || { echo "falta $tool (en Windows: winget install jqlang.jq)" >&2; exit 1; }
done

# ---------- salida ----------
step() { printf '\n\033[1;34m▶ %s\033[0m\n' "$*"; }
info() { printf '  %s\n' "$*"; }
pass() { printf '\033[1;32m✔ %s\033[0m\n' "$*"; }
fail() { printf '\033[1;31m✘ %s\033[0m\n' "$*" >&2; exit 1; }

# env_get KEY: lee un valor del .env sin hacer `source` (hay valores con espacios, como OPS_BOOTSTRAP_NAME).
env_get() {
  [[ -f "$ROOT/.env" ]] || fail "no existe .env (corre scripts/dev-env.sh)"
  grep -E "^$1=" "$ROOT/.env" | head -n1 | cut -d= -f2- || true
}

# ---------- sistema ----------
wait_ready() {
  local deadline=$((SECONDS + ${1:-120}))
  until curl -fsS "$ORCH/health/ready" >/dev/null 2>&1; do
    ((SECONDS < deadline)) || fail "el orquestador no quedó listo en $ORCH/health/ready"
    sleep 2
  done
}

TOKEN=""
login() {
  local user password body
  user="$(env_get OPS_BOOTSTRAP_USER)"
  password="$(env_get OPS_BOOTSTRAP_PASSWORD)"
  [[ -n "$user" && -n "$password" ]] || fail "faltan OPS_BOOTSTRAP_USER/OPS_BOOTSTRAP_PASSWORD en .env"
  body="$(jq -n --arg u "$user" --arg p "$password" '{username: $u, password: $p}')"
  TOKEN="$(curl -fsS -X POST "$ORCH/auth/login" -H 'Content-Type: application/json' -d "$body" | jq -r .token)"
  [[ -n "$TOKEN" && "$TOKEN" != null ]] || fail "login fallido"
}

# ---------- caos del simulador ----------
CHAOS_BACKUP=""
chaos_set() { curl -fsS -X POST "$SIM/chaos" -H 'Content-Type: application/json' -d "$1" >/dev/null || fail "POST /chaos rechazó: $1"; }

# chaos_save guarda el caos actual y lo restaura al salir del script, pase lo que pase.
chaos_save() {
  CHAOS_BACKUP="$(curl -fsS "$SIM/chaos")"
  trap 'chaos_set "$CHAOS_BACKUP" && info "caos restaurado"' EXIT
}

# chaos_calm apaga todas las fallas al azar, para que cada escenario muestre solo la falla que fuerza.
chaos_calm() {
  chaos_set '{
    "quoter": {"errorRate": 0, "invalidRate": 0, "minLatencyMs": 50, "maxLatencyMs": 300},
    "psp": {"approveRate": 1, "invalidRate": 0, "duplicateRate": 0, "earlyWebhookRate": 0,
            "minWebhookDelayMs": 200, "maxWebhookDelayMs": 800},
    "issuer": {"invalidRate": 0, "technicalErrorRate": 0, "businessErrorRate": 0, "timeoutButIssuedRate": 0,
               "minLatencyMs": 50, "maxLatencyMs": 300, "down": false, "outage": null}
  }'
}

# ---------- órdenes ----------
# random_plate: placa con formato válido (AAA12B o AAA123), distinta en cada orden.
random_plate() {
  local letters=ABCDEFGHIJKLMNOPQRSTUVWXYZ
  printf '%s%s%s%02d%d' "${letters:RANDOM%26:1}" "${letters:RANDOM%26:1}" "${letters:RANDOM%26:1}" $((RANDOM % 100)) $((RANDOM % 10))
}

# create_order KEY CORRELATION [PLATE]: imprime "<status HTTP> <JSON de la orden>".
create_order() {
  local key="$1" corr="$2" plate="${3:-$(random_plate)}" out code body
  body="$(jq -cn --arg p "$plate" '{plate: $p, vehicleYear: 2020, cityCode: "11001", documentId: "1020304050"}')"
  out="$(mktemp)"
  code="$(curl -sS -o "$out" -w '%{http_code}' -X POST "$ORCH/orders" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: $key" -H "X-Correlation-ID: $corr" -d "$body")"
  printf '%s %s\n' "$code" "$(cat "$out")"
  rm -f "$out"
}

order_get() { curl -fsS "$ORCH/orders/$1"; }

# wait_status ID ESTADO SEGUNDOS: espera el estado y falla si la orden termina en otro estado final.
wait_status() {
  local id="$1" want="$2" deadline=$((SECONDS + $3)) status
  while :; do
    status="$(order_get "$id" | jq -r .status)"
    [[ "$status" == "$want" ]] && return 0
    case "$status" in ISSUED | FAILED | REFUND_REQUIRED) fail "la orden $id terminó en $status (se esperaba $want)" ;; esac
    ((SECONDS < deadline)) || fail "la orden $id sigue en $status después de $3 s (se esperaba $want)"
    sleep 2
  done
}

# timeline ID: línea de tiempo de la orden (requiere login).
timeline() {
  curl -fsS "$ORCH/orders/$1/events" -H "Authorization: Bearer $TOKEN" |
    jq -r '.[] | "  \(.occurredAt[11:19])  \(.from // "—") → \(.to)  (\(.reason), \(.actor))"'
}

breaker_state() { curl -fsS "$ORCH/ops/breakers" -H "Authorization: Bearer $TOKEN" | jq -r '.[0].state'; }

# issuer_log_count ID PATRÓN: líneas del log del emisor simulado para ese externalRef cuyo mensaje empieza
# con PATRÓN. Necesita docker compose (corre en la máquina donde está el sistema).
issuer_log_count() {
  docker compose -f "$ROOT/docker-compose.yml" logs --no-log-prefix simulator 2>/dev/null |
    jq -R --arg ref "$1" --arg p "$2" 'fromjson? | select(.externalRef == $ref and (.msg | startswith($p)))' |
    jq -s length
}

# expect_single_policy ID: la regla crítica, una sola póliza por orden. El emisor simulado guarda una
# póliza por externalRef, así que contar sus pólizas siempre da 1; la prueba real son sus logs: /issue
# emitió una sola vez y ninguna llamada repetida tuvo que apoyarse en esa protección del legado.
expect_single_policy() {
  local id="$1" ours theirs issued repeated
  ours="$(order_get "$id" | jq -r .policyNumber)"
  theirs="$(curl -fsS "$SIM/policies?externalRef=$id" | jq -r '.policies[0].policyNumber // "ninguna"')"
  [[ "$ours" == "$theirs" ]] || fail "la orden $id tiene la póliza $ours y el emisor tiene $theirs"
  issued="$(issuer_log_count "$id" "policy issued")"
  repeated="$(issuer_log_count "$id" "policy already issued")"
  [[ "$issued" == 1 ]] || fail "el emisor emitió $issued veces para la orden $id (se esperaba 1)"
  [[ "$repeated" == 0 ]] || fail "se llamó a /issue $repeated veces más para la orden $id ya emitida"
  pass "una sola póliza ($ours): el emisor emitió una vez y nadie volvió a llamar a /issue"
}

# start ESCENARIO: prepara un escenario (sistema listo, login, caos guardado y en calma).
start() {
  printf '\n\033[1m=== %s ===\033[0m\n' "$1"
  wait_ready
  login
  chaos_save
  chaos_calm
}
