# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build API server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o passbi-api cmd/api/main.go

# Build importer
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o passbi-import cmd/importer/main.go

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binaries from builder
COPY --from=builder /app/passbi-api .
COPY --from=builder /app/passbi-import .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Expose API port
EXPOSE 8080

# Run API server by default
CMD ["./passbi-api"]
