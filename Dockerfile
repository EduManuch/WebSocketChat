# Сборка
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64
RUN go build -v -o ws cmd/server/main.go

# Запуск
FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y librdkafka-dev && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/ws /app/ws
COPY ./certs ./certs
COPY ./web ./web
COPY ./.env ./.env
ENTRYPOINT ["./ws"]
