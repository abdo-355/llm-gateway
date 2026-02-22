# Build stage
FROM golang:1.25-alpine AS builder

# Install git and ca-certificates for building
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gateway ./cmd/gateway

# Production stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests to providers
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/gateway .

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose ports
# 8080 - Main application port (public)
# 9090 - Metrics port (should be bound to localhost only)
EXPOSE 8080
EXPOSE 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${PORT:-8080}/health || exit 1

# Run the binary
ENTRYPOINT ["./gateway"]
