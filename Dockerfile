# Stage 1: Builder
FROM docker.m.daocloud.io/library/golang:1.25-alpine AS builder

# Set Go proxy for China
ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server

# Stage 2: Runner
FROM docker.m.daocloud.io/library/alpine:latest

WORKDIR /app

# Copy only the binary from builder
COPY --from=builder /build/main .

# Copy config file
COPY config.yaml .

EXPOSE 8080

CMD ["./main"]