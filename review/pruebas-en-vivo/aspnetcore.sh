#!/usr/bin/env bash
# Pruebas en vivo de los hallazgos del repositorio aspnetcore.
# Requisito: la app corriendo en http://localhost:8086 (ver la guía) y python en el PATH.
set -u
API=${API:-http://localhost:8086/api}
SUF=$RANDOM
GRANDE=$(mktemp)

echo "== Preparación: se registra a Alice y se crean tres artículos"
ALICE=$(curl -s -X POST "$API/users" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"username\":\"alice$SUF\",\"email\":\"alice$SUF@demo.com\",\"password\":\"alicepass123\"}}")
T=$(echo "$ALICE" | python -c "import sys,json;print(json.load(sys.stdin)['user']['token'])")
for i in 1 2 3; do
  curl -s -o /dev/null -X POST "$API/articles" -H "Authorization: Token $T" -H 'Content-Type: application/json' \
    -d "{\"article\":{\"title\":\"Nota $SUF $i\",\"description\":\"d\",\"body\":\"b\"}}"
done

echo
echo "== Prueba 1: paginación sin límite (Features/Articles/List.cs:74-75)"
echo "   limit=1 devuelve 1 artículo; limit=-1 debería ser 422, pero SQLite lo toma como 'sin límite':"
for L in 1 -1 1000000; do
  curl -s -w '\n%{http_code}' "$API/articles?limit=$L" \
    | python -c "import sys,json;*b,c=sys.stdin.read().splitlines();d=json.loads(''.join(b));print('   limit=$L -> HTTP %s, %d artículos de %d en total' % (c, len(d['articles']), d['articlesCount']))"
done

echo
echo "== Prueba 2: el token dura 5 minutos y no hay renovación (Infrastructure/Security/JwtIssuerOptions.cs:71)"
echo "$T" | python -c "import sys,json,base64;p=sys.stdin.read().strip().split('.')[1];p+='='*(-len(p)%4);c=json.loads(base64.urlsafe_b64decode(p));print('   exp - nbf =', c['exp']-c['nbf'], 'segundos')"

echo
echo "== Prueba 3: login sin límite de intentos (Features/Users/UsersController.cs:22-26)"
echo "   30 intentos con contraseña equivocada; lo esperado en producción es un 429 en algún momento:"
CODES=$(for i in $(seq 1 30); do
  curl -s -o /dev/null -w '%{http_code} ' -X POST "$API/users/login" -H 'Content-Type: application/json' \
    -d "{\"user\":{\"email\":\"alice$SUF@demo.com\",\"password\":\"intento$i\"}}"
done)
echo "   $CODES"
echo "   Tiempo de respuesta con un correo que existe vs. uno que no existe (enumeración por tiempo):"
curl -s -o /dev/null -w '   existe:    %{time_total} s\n' -X POST "$API/users/login" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"email\":\"alice$SUF@demo.com\",\"password\":\"otra-clave\"}}"
curl -s -o /dev/null -w '   no existe: %{time_total} s\n' -X POST "$API/users/login" -H 'Content-Type: application/json' \
  -d "{\"user\":{\"email\":\"nadie$SUF@demo.com\",\"password\":\"otra-clave\"}}"

echo
echo "== Prueba 4: slug que pierde las tildes o queda vacío (Infrastructure/Slug.cs:16)"
for TITLE in "Póliza de vehículo $SUF" "¿¡!?" "¿¡!?"; do
  # printf + --data-binary conserva el UTF-8 (en Git Bash de Windows, -d lo envía en ANSI)
  printf '{"article":{"title":"%s","description":"d","body":"b"}}' "$TITLE" \
    | curl -s -X POST "$API/articles" -H "Authorization: Token $T" -H 'Content-Type: application/json' --data-binary @- \
    | python -c "import sys,json;a=json.load(sys.stdin).get('article',{});print('   título %r -> slug %r' % (a.get('title'), a.get('slug')))"
done
echo "   El artículo con slug vacío no tiene URL propia: GET cae en el listado y DELETE no tiene ruta:"
curl -s -o /dev/null -w '   GET /articles/ -> HTTP %{http_code}\n' "$API/articles/"
curl -s -o /dev/null -w '   DELETE /articles/ -> HTTP %{http_code}\n' -X DELETE "$API/articles/" -H "Authorization: Token $T"

echo
echo "== Prueba 5: campos sin longitud máxima (Features/Articles/Create.cs:32-34)"
python -c "import json;print(json.dumps({'article':{'title':'Grande $SUF','description':'d','body':'x'*5_000_000}}))" > "$GRANDE"
curl -s -o /dev/null -w '   artículo con un cuerpo de 5 MB -> HTTP %{http_code}\n' -X POST "$API/articles" \
  -H "Authorization: Token $T" -H 'Content-Type: application/json' --data-binary @"$GRANDE"

echo
echo "== Prueba 6: CORS abierto a cualquier origen y Swagger público (Program.cs:79 y 86-89)"
curl -s -D - -o /dev/null "$API/user" -H 'Origin: https://sitio-malicioso.example' -H "Authorization: Token $T" \
  | grep -i 'access-control-allow-origin' | sed 's/^/   /'
curl -s -o /dev/null -w '   GET /swagger/v1/swagger.json -> HTTP %{http_code}\n' "${API%/api}/swagger/v1/swagger.json"

echo
echo "== Prueba 7: los errores de formato exponen tipos internos de .NET (Infrastructure/ValidatorActionFilter.cs:18-21)"
echo "   Un título con un byte que no es UTF-8 válido:"
# python escribe el byte 0xFF tal cual (printf y -d pueden recodificarlo según la consola)
python -c "import sys;sys.stdout.buffer.write(b'{\"article\":{\"title\":\"Mal\xff\",\"description\":\"d\",\"body\":\"b\"}}')" \
  | curl -s -X POST "$API/articles" -H "Authorization: Token $T" -H 'Content-Type: application/json' --data-binary @- \
  | sed 's/^/   /'
echo
