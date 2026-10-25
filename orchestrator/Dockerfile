# Etapa 1: compila un binario estático (sin cgo) con el toolchain de Go.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/orchestrator ./cmd/orchestrator

# Etapa 2: solo el binario sobre Alpine, con un usuario sin privilegios. Alpine (y no distroless) porque
# trae wget para el healthcheck del contenedor, igual que el simulador.
FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/orchestrator /usr/local/bin/orchestrator
USER app
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=20 CMD wget -qO- http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["orchestrator"]
