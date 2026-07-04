# =============================================================================
# Dockerfile do engine WaCalls (Go) — calibrado para o fork lucianomaynard/WaCalls.
#
# O servidor Go serve o client React estático de `client/dist` (flag -static) e
# persiste as credenciais whatsmeow em SQLite (flag -db). Portanto o build:
#   1) builda o client React (vite)  -> client/dist
#   2) builda o binário Go           -> /usr/local/bin/wacalls
#   3) runtime roda: wacalls -addr :8080 -static client/dist -db /data/wacalls.db
#
# Cópia canônica. Uma cópia idêntica fica em engine/Dockerfile para o build local.
# TODO técnico: commite este Dockerfile no SEU fork para builds reproduzíveis.
# =============================================================================

# ---- 1. build do client React ----
FROM node:22-alpine AS client
WORKDIR /client
COPY client/package.json ./
RUN npm install
COPY client/ ./
RUN npm run build   # gera /client/dist

# ---- 2. build do servidor Go ----
FROM golang:1.26-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/wacalls ./cmd/server

# ---- 3. runtime ----
FROM alpine:3.20
# ffmpeg: converte as gravações WAV -> Opus antes de subir ao storage externo.
RUN apk add --no-cache ca-certificates ffmpeg
WORKDIR /app
COPY --from=build /out/wacalls /usr/local/bin/wacalls
COPY --from=client /client/dist /app/client/dist
EXPOSE 8080
VOLUME ["/data"]
# wacalls.db (credenciais whatsmeow) persiste em /data (volume engine_data).
ENTRYPOINT ["/usr/local/bin/wacalls", "-addr", ":8080", "-static", "/app/client/dist", "-db", "/data/wacalls.db"]
