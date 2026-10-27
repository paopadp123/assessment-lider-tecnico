#!/usr/bin/env bash
# Reproduce el rojo→verde del PR #1 del fork de golang-gin en un contenedor Linux (sqlite3 necesita CGO).
# Uso: MSYS_NO_PATHCONV=1 docker run --rm -v "$PWD/review/pruebas-en-vivo/hu12-rojo-verde.sh:/r.sh:ro" golang:1.27 bash /r.sh
git config --global --add safe.directory '*'
git clone -q https://github.com/paopadp123/golang-gin-realworld-example-app.git /tmp/w && cd /tmp/w
for c in d2adb3c 0fa93ed; do
  git checkout -q $c
  echo "### $(git log --oneline -1)"
  echo "\$ go version && go test -count=1 ./..."
  go version
  go test -count=1 ./... 2>&1 | grep -E "^(=== RUN|--- FAIL|--- PASS: Test(LoadJWT|NewValidatorErrorWith|BindErrorStatus|UsersLoginWithMalformed|AuthMiddlewareRejectsTokenOfMissing|AuthMiddlewareOptionalTreatsMissing))|^(ok|FAIL|\?)\s|Error:  |expected: |actual  : |Messages: |should not panic" | grep -v "^=== RUN"
  echo "pruebas que fallan: $(go test -count=1 ./... 2>&1 | grep -cE '^--- FAIL')"
  echo
done
