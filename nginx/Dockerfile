# Knowoff local reverse proxy — nginx in front of the Flutter web client
# and the Go game server, terminating TLS for *.knowoff.local (see
# README.md and entrypoint.sh for certificate handling).
FROM nginx:1.27-alpine

# ── OCI image labels ────────────────────────────────────────────────────
# Bump org.opencontainers.image.version with:
#   make containers.label.version SERVICE=nginx VERSION=X.Y.Z
ARG IMAGE_VERSION=0.1.0
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="knowoff-nginx" \
      org.opencontainers.image.description="Reverse proxy + TLS termination for the Knowoff local dev stack (*.knowoff.local)" \
      org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.vendor="Knowoff" \
      org.opencontainers.image.authors="Knowoff" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.url="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.documentation="https://github.com/metaphy6/knowoff/blob/main/nginx/README.md" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.ref.name="knowoff-nginx" \
      org.opencontainers.image.base.name="nginx:1.27-alpine"

RUN apk add --no-cache openssl

COPY nginx.conf /etc/nginx/nginx.conf
COPY conf.d/ /etc/nginx/conf.d/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80 443
ENTRYPOINT ["/entrypoint.sh"]
