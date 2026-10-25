#!/usr/bin/env bash
# Escenario 1 — compra feliz: cotiza, paga, recibe el webhook firmado y emite. El PSP manda cada webhook
# dos veces y a veces antes de su 202: el orquestador los deduplica y los aplica en orden.
source "$(dirname "$0")/lib.sh"
start "1. Compra feliz (con webhooks duplicados y adelantados)"

chaos_set '{"psp": {"duplicateRate": 1, "earlyWebhookRate": 0.5}}'
info "caos: cada webhook llega dos veces; la mitad, antes de la respuesta 202 del PSP"

key="e2e-feliz-$(date +%s)-$RANDOM"
corr="e2e-feliz-$RANDOM"
plate="$(random_plate)"

step "POST /orders (Idempotency-Key: $key)"
read -r code order <<<"$(create_order "$key" "$corr" "$plate")"
[[ "$code" == 201 ]] || fail "se esperaba 201 y llegó $code: $(brief "$order")"
id="$(jq -r .id <<<"$order")"
info "orden $id en $(jq -r .status <<<"$order"), prima $(jq -r '.quote.premium' <<<"$order") $(jq -r '.quote.currency' <<<"$order")"

step "Repetir el mismo POST con la misma clave"
read -r code again <<<"$(create_order "$key" "$corr" "$plate")"
[[ "$code" == 200 && "$(jq -r .id <<<"$again")" == "$id" ]] || fail "la repetición devolvió $code con otra orden: $(brief "$again")"
pass "200 con la misma orden: no se creó otra ni se volvió a cotizar"

step "Esperar ISSUED"
wait_status "$id" ISSUED 60
pass "orden ISSUED con póliza $(order_get "$id" | jq -r .policyNumber)"
expect_single_policy "$id"
paid="$(events "$id" | jq '[.[] | select(.to == "PAID")] | length')"
[[ "$paid" == 1 ]] || fail "el pago se aplicó $paid veces (se esperaba 1)"
pass "los webhooks duplicados no tuvieron efecto: un solo PAYMENT_PENDING → PAID"

step "Línea de tiempo (correlation-id $corr)"
timeline "$id"
