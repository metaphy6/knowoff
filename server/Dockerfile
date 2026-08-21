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
RUN apk add --no-cache ca-certificates libwebp
WORKDIR /app
COPY --from=builder /app/knowoffd /knowoffd
COPY --from=builder /app/migrations /migrations
EXPOSE 8080 9090 9091
ENTRYPOINT ["/knowoffd"]
