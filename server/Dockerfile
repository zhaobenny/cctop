FROM golang:1.21-alpine AS builder

# Install build dependencies for CGO (required for SQLite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go.mod and go.sum from parent directory
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the server binary with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o cctop-server ./server

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary
COPY --from=builder /app/cctop-server /app/cctop-server

# Create data directory for SQLite
RUN mkdir -p /data

# Default environment
ENV PORT=8080
ENV DB_PATH=/data/cctop.db

EXPOSE 8080

ENTRYPOINT ["/app/cctop-server"]
