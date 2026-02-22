# Этап 1: Сборка бинарного файла
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api-gateway ./cmd/main.go

# Этап 2: Минимальный production-образ
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /bin/api-gateway /usr/local/bin/api-gateway

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["api-gateway"]
