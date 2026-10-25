#!/usr/bin/env bash
# Escenario 2 — el emisor legado emite la póliza pero responde a los 8 s, después del timeout de la ACL
# (5 s). Un timeout no prueba que no se emitió: la ACL consulta por externalRef antes de responder y
# la orden queda ISSUED con una sola póliza, sin volver a llamar a /issue.
source "$(dirname "$0")/lib.sh"
start "2. Timeout pero emitió"

chaos_set '{"issuer": {"timeoutButIssuedRate": 1, "timeoutMs": 8000}}'
info "caos: el emisor emite y responde a los 8 s (la ACL corta a los 5 s)"

corr="e2e-timeout-$RANDOM"
step "POST /orders"
read -r code order <<<"$(create_order "e2e-timeout-$(date +%s)-$RANDOM" "$corr")"
[[ "$code" == 201 ]] || fail "se esperaba 201 y llegó $code: $(brief "$order")"
id="$(jq -r .id <<<"$order")"
info "orden $id"

step "Esperar ISSUED"
wait_status "$id" ISSUED 90
pass "orden ISSUED con póliza $(order_get "$id" | jq -r .policyNumber)"
expect_single_policy "$id"

step "Lo que hizo la ACL (logs de issuing-service con el correlation-id $corr)"
docker compose -f "$ROOT/docker-compose.yml" logs --no-log-prefix issuing-service 2>/dev/null | grep -F "$corr" | jq -r '"  " + .Message' || true

step "Línea de tiempo"
timeline "$id"
