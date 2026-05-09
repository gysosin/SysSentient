# syntax=docker/dockerfile:1

FROM node:22-bookworm-slim AS web-builder
WORKDIR /src/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run typecheck \
    && npm audit --audit-level=moderate \
    && npm run build

FROM golang:1.25-bookworm AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/web/dist ./web/dist
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sys-daemon ./cmd/daemon

FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system syssentient \
    && useradd --system --gid syssentient --home-dir /var/lib/sys-sentient --shell /usr/sbin/nologin syssentient \
    && mkdir -p /var/lib/sys-sentient \
    && chown -R syssentient:syssentient /var/lib/sys-sentient

WORKDIR /app
COPY --from=go-builder /out/sys-daemon /app/sys-daemon
COPY --from=web-builder /src/web/dist /app/web/dist

ENV SYS_SENTIENT_DATABASE_PATH=/var/lib/sys-sentient/sys-sentient.db
EXPOSE 8080

USER syssentient
ENTRYPOINT ["/app/sys-daemon"]
