# Этап 1: Сборка бинарного файла
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/tools-service ./cmd/server/main.go

# Этап 2: Минимальный production-образ
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /workspace

RUN chown -R appuser:appgroup /workspace

COPY --from=builder /bin/tools-service /usr/local/bin/tools-service

USER appuser

EXPOSE 8082

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8082/health || exit 1

ENTRYPOINT ["tools-service"]
