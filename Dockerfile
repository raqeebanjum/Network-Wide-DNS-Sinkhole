# syntax=docker/dockerfile:1

FROM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend /src/web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags embedded \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/sinkhole ./cmd/sinkhole

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 sinkhole \
    && adduser -S -D -H -u 10001 -G sinkhole sinkhole \
    && mkdir -p /data \
    && chown -R sinkhole:sinkhole /data
COPY --from=backend /out/sinkhole /usr/local/bin/sinkhole

ENV SINKHOLE_DNS_LISTEN=:53 \
    SINKHOLE_HTTP_LISTEN=:8080 \
    SINKHOLE_DATA_DIR=/data \
    SINKHOLE_UPSTREAMS=1.1.1.1:53,1.0.0.1:53 \
    SINKHOLE_DNS_TIMEOUT=5s \
    SINKHOLE_DNS_MAX_INFLIGHT=256 \
    SINKHOLE_QUERY_RETENTION=720h \
    SINKHOLE_BLOCKLIST_REFRESH=24h \
    SINKHOLE_LOG_LEVEL=info \
    SINKHOLE_LOG_FORMAT=json

LABEL org.opencontainers.image.title="Network-Wide DNS Sinkhole" \
      org.opencontainers.image.description="Self-hosted DNS filtering with query logs and blocklist management" \
      org.opencontainers.image.source="https://github.com/raqeebanjum/Network-Wide-DNS-Sinkhole"

USER sinkhole:sinkhole
VOLUME ["/data"]
EXPOSE 53/tcp 53/udp 8080/tcp
STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/sinkhole"]
