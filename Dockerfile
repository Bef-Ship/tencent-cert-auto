# Build images / arch can be overridden via build-args / env:
#   docker build --build-arg TARGETARCH=arm64 .
#   TARGETARCH=arm64 docker compose build
#   docker buildx build --platform linux/arm64 .

ARG GO_IMAGE=golang:1.26-alpine
ARG RUNTIME_IMAGE=alpine:3.19

FROM ${GO_IMAGE} AS builder

# BuildKit injects TARGETOS/TARGETARCH for multi-platform builds.
# Override examples: TARGETARCH=arm64 | amd64 | arm
ARG TARGETOS=linux
ARG TARGETARCH=amd64
# Only needed for 32-bit ARM (e.g. GOARM=7 with TARGETARCH=arm)
ARG GOARM=

WORKDIR /src
RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${GOARM} \
    go build -trimpath -ldflags="-s -w" -o /out/tencent-cert-auto ./cmd/tencent-cert-auto

# Runtime — no ports exposed, minimal attack surface
FROM ${RUNTIME_IMAGE}

# docker-cli: optional, for local mode reload_cmd like `docker exec nginx ...`
RUN apk add --no-cache ca-certificates tzdata docker-cli \
    && adduser -D -H -u 10001 appuser

WORKDIR /app
COPY --from=builder /out/tencent-cert-auto /app/tencent-cert-auto

RUN mkdir -p /app/cert-data && chown -R appuser:appuser /app

USER appuser

VOLUME ["/app/cert-data"]

ENTRYPOINT ["/app/tencent-cert-auto"]
