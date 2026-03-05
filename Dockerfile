# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder

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

# Need ca-certificates for outbound HTTPS and sqlite for debugging
RUN apk --no-cache add ca-certificates tzdata sqlite

WORKDIR /app

COPY --from=builder /app/qradar-collector .

# Default command
CMD ["./qradar-collector"]
