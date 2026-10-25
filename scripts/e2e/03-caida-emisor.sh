#!/usr/bin/env bash
# Escenario 3 — el emisor se cae: tras 5 fallas seguidas el circuit breaker se abre y las órdenes pagadas
# esperan en ISSUING (visibles, sin gastar intentos). Al volver el emisor, el breaker prueba de nuevo
# (half-open) y las órdenes terminan en ISSUED; las que agotaron sus intentos las recoge la reconciliación.
source "$(dirname "$0")/lib.sh"
start "3. Caída del emisor y recuperación"

chaos_set '{"issuer": {"down": true}}'
info "caos: emisor caído (503 en /issue y /policies)"

ids=()
step "Tres compras con el emisor caído"
for n in 1 2 3; do
  read -r code order <<<"$(create_order "e2e-caida-$(date +%s)-$n-$RANDOM" "e2e-caida-$n-$RANDOM")"
  [[ "$code" == 201 ]] || fail "se esperaba 201 y llegó $code: $order"
  ids+=("$(jq -r .id <<<"$order")")
  info "orden ${ids[-1]}"
done

step "Esperar que el breaker se abra"
deadline=$((SECONDS + 90))
until [[ "$(breaker_state)" == open ]]; do
  ((SECONDS < deadline)) || fail "el breaker no se abrió en 90 s"
  sleep 2
done
pass "breaker abierto (GET /ops/breakers)"
for id in "${ids[@]}"; do
  status="$(order_get "$id" | jq -r .status)"
  [[ "$status" == ISSUING ]] || fail "la orden $id está en $status con el emisor caído (se esperaba ISSUING)"
done
pass "las 3 órdenes esperan en ISSUING: pagadas, sin póliza y sin reembolso"
info "en http://localhost:15672 (colas orders.*) se ven los mensajes aplazados y los reintentos"

sleep "${OUTAGE_HOLD_S:-10}"
step "Levantar el emisor"
chaos_set '{"issuer": {"down": false}}'

step "Esperar ISSUED en las 3 órdenes"
for id in "${ids[@]}"; do
  wait_status "$id" ISSUED 180
  expect_single_policy "$id"
done
info "breaker: $(breaker_state)"

step "Línea de tiempo de la primera orden"
timeline "${ids[0]}"
