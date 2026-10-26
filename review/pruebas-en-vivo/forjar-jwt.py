"""Fabrica un JWT HS256 con el secreto publicado en common/utils.go del repositorio golang-gin.

Uso: python forjar-jwt.py <id>         -> token para el usuario <id>
     python forjar-jwt.py --texto <x>  -> token con el id como texto (para probar el panic del middleware)
Solo usa la biblioteca estándar de Python.
"""
import base64, hashlib, hmac, json, sys, time

SECRETO = b"A String Very Very Very Strong!!@##$!@#$"  # common/utils.go:41, público en GitHub


def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


if sys.argv[1] == "--texto":
    user_id = sys.argv[2]
else:
    user_id = int(sys.argv[1])

header = b64(json.dumps({"alg": "HS256", "typ": "JWT"}).encode())
payload = b64(json.dumps({"id": user_id, "exp": int(time.time()) + 3600}).encode())
firma = b64(hmac.new(SECRETO, f"{header}.{payload}".encode(), hashlib.sha256).digest())
print(f"{header}.{payload}.{firma}")
