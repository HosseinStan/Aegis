FROM golang:1.26.10-alpine3.24 AS build
WORKDIR /src/apps/aegis-api
COPY apps/aegis-api/go.mod apps/aegis-api/go.sum ./
RUN go mod download
COPY apps/aegis-api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/aegis-api ./cmd/server

FROM alpine:3.24.1
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 aegis \
    && adduser -S -D -H -u 10001 -G aegis aegis \
    && mkdir -p /app/data/backups \
    && chown -R 10001:10001 /app
WORKDIR /app
COPY --from=build /out/aegis-api /app/aegis-api
COPY packages/shared-contracts/openapi/aegis-api.yaml /app/openapi.yaml
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/app/aegis-api"]

