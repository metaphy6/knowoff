# Build stage: compile the Flutter web release bundle.
FROM debian:bookworm-slim AS builder

ARG FLUTTER_VERSION=3.47.1
RUN apt-get update && apt-get install -y --no-install-recommends \
      git curl ca-certificates unzip xz-utils \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch ${FLUTTER_VERSION} https://github.com/flutter/flutter.git /opt/flutter
ENV PATH="/opt/flutter/bin:/opt/flutter/bin/cache/dart-sdk/bin:${PATH}"
RUN git config --global --add safe.directory /opt/flutter \
    && flutter config --no-analytics --enable-web
WORKDIR /app
COPY . .
RUN flutter pub get && flutter build web --release

# Production stage: serve the static bundle from a minimal nginx image.
# (This is a plain static-file server, unrelated to the repo-root nginx/
# reverse proxy — that proxy's app.knowoff.local vhost fronts whichever
# of this image or client-web-dev is running.)
FROM nginx:1.27-alpine

ARG IMAGE_VERSION=0.1.0
ARG BUILD_DATE=unknown
ARG VCS_REF=unknown
LABEL org.opencontainers.image.title="knowoff-client-web" \
      org.opencontainers.image.description="Knowoff Flutter client — compiled web release bundle, served statically" \
      org.opencontainers.image.version="${IMAGE_VERSION}" \
      org.opencontainers.image.vendor="Knowoff" \
      org.opencontainers.image.authors="Knowoff" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.url="https://github.com/metaphy6/knowoff" \
      org.opencontainers.image.documentation="https://github.com/metaphy6/knowoff/blob/main/client/README.md" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.ref.name="knowoff-client-web" \
      org.opencontainers.image.base.name="nginx:1.27-alpine"

COPY --from=builder /app/build/web /usr/share/nginx/html
EXPOSE 80
