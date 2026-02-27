# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git and root certs
RUN apk add --no-cache git ca-certificates tzdata

# Cache mod downloads
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o qradar-collector ./cmd/collector/

# Stage 2: Minimal runtime image
FROM alpine:latest

# Need ca-certificates for outbound HTTPS
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Non-root user
RUN adduser -D -g '' collectoruser
USER collectoruser

COPY --from=builder /app/qradar-collector .

# Default command
CMD ["./qradar-collector"]
