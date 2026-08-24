# Build stage: static Go binary for production.
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache gcc musl-dev libwebp-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -o knowoffd ./cmd/knowoffd

# Production stage: alpine with libwebp runtime (required by chai2010/webp).
FROM alpine:latest

# ── OCI image labels ────────────────────────────────────────────────────
# Bump org.opencontainers.image.version with:
#   make containers.label.version SERVICE=server VERSION=X.Y.Z
ARG IMAGE_VERSION=0.1.0
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="knowoff-server" \
      org.opencontainers.image.description="Knowoff authoritative Go game server, Admin Console, and Contributor Portal" \
      org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.vendor="Knowoff" \
      org.opencontainers.image.authors="Knowoff" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.url="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.documentation="https://github.com/metaphy6/knowoff/blob/main/server/README.md" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.ref.name="knowoff-server" \
      org.opencontainers.image.base.name="alpine:latest"

RUN apk add --no-cache ca-certificates libwebp
WORKDIR /app
COPY --from=builder /app/knowoffd /knowoffd
COPY --from=builder /app/migrations /migrations
EXPOSE 8080 9090 9091
ENTRYPOINT ["/knowoffd"]
