#!/usr/bin/env bash
# Escenario 4 — el pago se aprobó pero el emisor rechaza la emisión por una regla de negocio (E-102/E-104,
# escondidos en un 200 que la ACL traduce a 422). La orden queda en REFUND_REQUIRED para que operaciones
# decida el reverso: no hay reembolso automático. Al final un operador reintenta y la acción queda auditada.
source "$(dirname "$0")/lib.sh"
start "4. Pago aprobado + emisión rechazada → REFUND_REQUIRED"

chaos_set '{"issuer": {"businessErrorRate": 1}}'
info "caos: el emisor rechaza toda emisión con un error de negocio"

step "POST /orders"
read -r code order <<<"$(create_order "e2e-reembolso-$(date +%s)-$RANDOM" "e2e-reembolso-$RANDOM")"
[[ "$code" == 201 ]] || fail "se esperaba 201 y llegó $code: $order"
id="$(jq -r .id <<<"$order")"
info "orden $id"

step "Esperar REFUND_REQUIRED"
wait_status "$id" REFUND_REQUIRED 60
o="$(order_get "$id")"
[[ "$(jq -r .paymentId <<<"$o")" != null ]] || fail "la orden no tiene pago registrado"
[[ "$(jq -r .failureReason <<<"$o")" == ISSUE_REJECTED ]] || fail "motivo inesperado: $(jq -r .failureReason <<<"$o")"
[[ "$(issuer_log_count "$id" "policy issued")" == 0 ]] || fail "el emisor sí emitió una póliza para la orden $id"
pass "REFUND_REQUIRED con motivo ISSUE_REJECTED, pago $(jq -r .paymentId <<<"$o") y ninguna póliza emitida"

step "Línea de tiempo"
timeline "$id"

step "Operador: reintentar la emisión cuando el emisor ya acepta"
chaos_set '{"issuer": {"businessErrorRate": 0}}'
curl -fsS -X POST "$ORCH/orders/$id/actions/retry-issuance" -H "Authorization: Bearer $TOKEN" >/dev/null
wait_status "$id" ISSUED 60
expect_single_policy "$id"
timeline "$id" | tail -n 3
pass "la acción quedó en la línea de tiempo con el usuario del operador"
