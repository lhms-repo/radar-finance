# Production image: one static Go binary with the React build embedded (section 8.3).

FROM node:24-bookworm-slim AS web
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY api/openapi.yaml /src/api/openapi.yaml
COPY web/ ./
RUN pnpm run build

FROM golang:1.26-bookworm AS go
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN go build -trimpath -tags embedweb -ldflags "-s -w -X main.version=${VERSION}" -o /out/radar ./cmd/radar
# Data folder owned by the nonroot user, so the named volume inherits the owner.
RUN mkdir -p /out/data /out/backups

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/radar /radar
COPY --from=go --chown=65532:65532 /out/data /data
COPY --from=go --chown=65532:65532 /out/backups /backups
ENV RADAR_ADDR=:8080 \
    RADAR_DB_PATH=/data/radar.db \
    RADAR_BACKUP_DIR=/backups \
    TZ=America/Sao_Paulo
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/radar", "healthcheck"]
ENTRYPOINT ["/radar"]
CMD ["serve"]
