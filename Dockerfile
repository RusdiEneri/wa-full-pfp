# Stage 1: Build binary Go
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git, ca-certificates, and tzdata
RUN apk add --no-cache git ca-certificates tzdata

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and frontend
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server .

# Stage 2: Minimal runtime container
FROM alpine:latest

# Install runtime SSL certificates & timezone data
RUN apk add --no-cache ca-certificates tzdata

# Setup non-root user for Hugging Face Spaces (UID 1000)
RUN adduser -D -u 1000 user
USER user

ENV HOME=/home/user \
    PATH=/home/user/.local/bin:$PATH \
    PORT=7860

WORKDIR /home/user/app

# Copy binary and frontend assets
COPY --chown=user:user --from=builder /app/server .
COPY --chown=user:user --from=builder /app/frontend ./frontend

# Hugging Face default port
EXPOSE 7860

CMD ["./server"]
