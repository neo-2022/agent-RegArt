# Этап 1: Сборка бинарного файла
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/agent-service ./cmd/server/main.go

# Этап 2: Минимальный production-образ
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

RUN mkdir -p uploads/avatars prompts/admin && chown -R appuser:appgroup /app

COPY --from=builder /bin/agent-service /usr/local/bin/agent-service

USER appuser

EXPOSE 8083

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8083/health || exit 1

ENTRYPOINT ["agent-service"]
