# syntax=docker/dockerfile:1

FROM node:26-bookworm-slim AS web-builder
WORKDIR /src/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
# Keep this stage hermetic: typecheck + build only.
# `npm audit` reaches the advisory API at build time, which makes image builds
# non-reproducible and breaks previously-green tagged commits when a new CVE
# lands upstream. Audit and tests run in CI (.github/workflows/ci.yml) instead.
RUN npm run typecheck \
    && npm run build

FROM golang:1.27-bookworm AS go-builder
ARG VERSION=dev
ARG COMMIT=""
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/web/dist ./web/dist
# CGO_ENABLED=0 since the SQLite driver became pure Go. The binary is now
# static, so a plain GOOS/GOARCH matrix works and the runtime stage no longer
# needs glibc. Moving to a distroless or scratch base is tracked in
# docs/plans/04-goreleaser-packaging.md along with the multi-arch image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w \
      -X sys-sentient/internal/version.Version=${VERSION} \
      -X sys-sentient/internal/version.Commit=${COMMIT}" \
    -o /out/sys-sentient ./cmd/daemon

FROM debian:bookworm-slim AS runtime
ARG VERSION=dev
ARG COMMIT=""

LABEL org.opencontainers.image.title="SysSentient" \
      org.opencontainers.image.description="AI-assisted system monitor with alerting" \
      org.opencontainers.image.source="https://github.com/gysosin/SysSentient" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system syssentient \
    && useradd --system --gid syssentient --home-dir /var/lib/sys-sentient --shell /usr/sbin/nologin syssentient \
    && mkdir -p /var/lib/sys-sentient \
    && chown -R syssentient:syssentient /var/lib/sys-sentient

WORKDIR /app
COPY --from=go-builder /out/sys-sentient /app/sys-sentient
COPY --from=web-builder /src/web/dist /app/web/dist

ENV SYS_SENTIENT_DATABASE_PATH=/var/lib/sys-sentient/sys-sentient.db
VOLUME ["/var/lib/sys-sentient"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl -fsS "http://127.0.0.1:${SYS_SENTIENT_SERVER_PORT:-8080}/health" >/dev/null || exit 1

USER syssentient
ENTRYPOINT ["/app/sys-sentient"]
