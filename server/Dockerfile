# Build stage: static Go binary for production.
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o knowoffd ./cmd/knowoffd

# Production stage: distroless static image.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /app/knowoffd /knowoffd
COPY --from=builder /app/migrations /migrations
USER nonroot:nonroot
EXPOSE 8080 9090 9091
ENTRYPOINT ["/knowoffd"]
