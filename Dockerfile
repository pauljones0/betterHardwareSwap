FROM golang:1.25-alpine AS builder
WORKDIR /app

# Copy go mod and install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -v -o server ./cmd/server

# Create a minimal runtime image
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata

# Create a non-root user for security
RUN adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=builder /app/server .

# Ensure the binary is executable and owned by appuser
RUN chown appuser:appuser /app/server

USER appuser

# Run the web service on container startup.
CMD ["./server"]
