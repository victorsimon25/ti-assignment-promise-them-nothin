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

WORKDIR /app

# Copy build artifact
COPY --from=builder /app/rate-limiter .

EXPOSE 8080

CMD ["./rate-limiter"]
