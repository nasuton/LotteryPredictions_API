# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/lottery-api .

# ---- runtime stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -u 10001 -G app -H -s /sbin/nologin app

WORKDIR /app
COPY --from=build /out/lottery-api /app/lottery-api

USER app:app

ENV PORT=8080 \
    GIN_MODE=release \
    LOG_FORMAT=json \
    TZ=Asia/Tokyo

EXPOSE 8080

# /healthz は DB に触れない liveness。BASE_PATH を設定している場合はその配下になる
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${PORT}${BASE_PATH}/healthz" >/dev/null || exit 1

ENTRYPOINT ["/app/lottery-api"]
