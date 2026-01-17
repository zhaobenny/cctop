FROM golang:1.21-alpine AS builder

ARG VERSION=dev

# Install build dependencies for CGO (required for SQLite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum from parent directory
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build fully static binary with musl
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -linkmode external -extldflags '-static'" \
    -o cctop-server ./server

# Final stage - scratch image
FROM scratch

# Import user/group for non-root execution
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the static binary
COPY --from=builder /app/cctop-server /cctop-server

# Create data directory with correct ownership
COPY --chown=nobody:nogroup --from=builder /tmp /data

# Run as non-root
USER nobody:nogroup

# Default environment
ENV PORT=8080
ENV DB_PATH=/data/cctop.db

EXPOSE 8080

ENTRYPOINT ["/cctop-server"]
