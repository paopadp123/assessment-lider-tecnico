#!/usr/bin/env bash
# Pruebas en vivo de los hallazgos del repositorio next-realworld-example-app.
# Requisito: node y npm en el PATH, y el clon del fork en $REPO.
# Instala marked en una carpeta temporal propia: no ejecuta código del repositorio revisado.
set -u
REPO=${REPO:-../PruebaSimon-forks/next-realworld-example-app}
TMP=$(mktemp -d)

echo "== Prueba 1: el backend y los estilos están fijos a dominios que ya no funcionan (lib/utils/constant.ts:1, pages/_document.tsx:145)"
curl -s -o /dev/null -m 15 -w '   API fija en el código: %{http_code} -> ' https://conduit.productionready.io/api/tags
curl -s -o /dev/null -m 15 -L -w '%{url_effective} -> HTTP %{http_code}\n' https://conduit.productionready.io/api/tags
curl -s -o /dev/null -m 15 -w '   Hoja de estilos principal: HTTP %{http_code}\n' https://demo.productionready.io/main.css

echo
echo "== Prueba 2: dependencias con vulnerabilidades conocidas (package.json:10-17)"
(cd "$REPO" && npm audit --package-lock-only --json 2>/dev/null) \
  | node -e "let s='';process.stdin.on('data',d=>s+=d).on('end',()=>{const m=JSON.parse(s).metadata.vulnerabilities;console.log('   '+JSON.stringify(m))})"

echo
echo "== Prueba 3: la protección contra XSS depende de una opción que ya no existe (pages/article/[pid].tsx:30)"
cat > "$TMP/xss.mjs" <<'EOF'
import { createRequire } from "module";
const require = createRequire(process.argv[2] + "/");
const pkg = require("marked/package.json");
const m = require("marked");
const marked = m.marked ?? m;
const html = marked("<img src=x onerror=alert(document.domain)>", { sanitize: true }).trim();
console.log(`   marked ${pkg.version} con sanitize: true -> ${html}`);
EOF
for V in 1.1.1 latest; do
  mkdir -p "$TMP/$V" && (cd "$TMP/$V" && npm init -y >/dev/null && npm install "marked@$V" --silent >/dev/null 2>&1)
  node "$TMP/xss.mjs" "$TMP/$V" 2>/dev/null
done

echo
echo "== Prueba 4 (manual, en el navegador): publicar un artículo válido rompe el editor (pages/editor/new.tsx:44-45)"
echo "   1. Cambiar SERVER_BASE_URL por una API RealWorld local (p. ej. la de C# en http://localhost:8086/api)."
echo "   2. NODE_OPTIONS=--openssl-legacy-provider npx next dev -p 3005 (Next 9.5 es anterior a OpenSSL 3; se corrió con esa opción en Node 24)."
echo "   3. Iniciar sesión, ir a /editor/new y publicar un artículo válido."
echo "   Resultado: la API responde 201 y crea el artículo; la consola muestra"
echo "   'TypeError: Cannot convert undefined or null to object at ListErrors'."
echo "   4. Revisar con git diff lib/utils/constant.ts que solo cambió esa línea y revertirla con git restore lib/utils/constant.ts."

rm -rf "$TMP"
