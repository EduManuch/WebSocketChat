# Сборка
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY . .
RUN go clean --modcache
RUN go mod download
ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go build -v -o ws cmd/server/main.go

# Запуск
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y librdkafka-dev && rm -rf /var/lib/apt/lists/*
RUN groupadd -r webgroup && useradd -r webuser -G webgroup -u 1000
WORKDIR /app
COPY --from=builder --chown=webuser:webgroup /app/ws /app/ws
COPY --chown=webuser:webgroup ./certs ./certs
COPY --chown=webuser:webgroup ./web ./web
COPY --chown=webuser:webgroup ./.env ./.env
USER webuser
CMD ["./ws"]
