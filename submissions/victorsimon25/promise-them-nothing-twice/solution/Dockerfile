FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build executable
RUN CGO_ENABLED=0 GOOS=linux go build -o rate-limiter .

# Final minimal runtime image
FROM alpine:3.20

# Create a non-root group and user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy build artifact and set ownership to non-root user
COPY --from=builder --chown=appuser:appgroup /app/rate-limiter .

# Switch to non-root user
USER appuser

EXPOSE 8080

CMD ["./rate-limiter"]

